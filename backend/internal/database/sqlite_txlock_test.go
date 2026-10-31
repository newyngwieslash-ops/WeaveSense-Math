package database

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSQLiteDSNAddsImmediateTxLock(t *testing.T) {
	cases := map[string]string{
		"/d/a.db?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL": "/d/a.db?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL&_txlock=immediate",
		"/d/a.db":                                 "/d/a.db?_txlock=immediate",
		"file:/d/a.db?_txlock=exclusive":          "file:/d/a.db?_txlock=exclusive",
		"file:/d/a.db?mode=ro&_busy_timeout=5000": "file:/d/a.db?mode=ro&_busy_timeout=5000",
	}
	for in, want := range cases {
		if got := SQLiteDSN(in); got != want {
			t.Errorf("SQLiteDSN(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenSQLiteDefaultDSNKeepsParamsAndUsesImmediate(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{Driver: "sqlite", DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	var journal string
	var busy, fk int
	db.Raw("PRAGMA journal_mode").Scan(&journal)
	db.Raw("PRAGMA busy_timeout").Scan(&busy)
	db.Raw("PRAGMA foreign_keys").Scan(&fk)
	if strings.ToLower(journal) != "wal" || busy != 5000 || fk != 1 {
		t.Fatalf("journal=%s busy=%d fk=%d, want wal 5000 1", journal, busy, fk)
	}
	// BEGIN IMMEDIATE takes the write reservation at BEGIN: a second
	// connection cannot start a write transaction while the first is open.
	other, err := gorm.Open(sqlite.Open(filepath.Join(dir, "open_ai_canvas.db")+"?_busy_timeout=0&_txlock=immediate"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	otherDB, _ := other.DB()
	defer otherDB.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if err := other.Exec("CREATE TABLE probe (id integer)").Error; err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("write from another connection during an open transaction: err=%v, want database is locked", err)
	}
	tx.Rollback()
}

// Regression: with deferred transactions, read-then-write transactions on
// separate connections fail with an immediate "database is locked" (lock
// upgrade is not retried by busy_timeout). With Open's DSN they queue.
func TestOpenSQLiteConcurrentReadThenWriteTransactionsDoNotFailLocked(t *testing.T) {
	db, err := Open(Config{Driver: "sqlite", DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := ConfigurePool(db); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := db.Exec("CREATE TABLE counter (id integer primary key, n integer not null)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO counter (id, n) VALUES (1, 0)").Error; err != nil {
		t.Fatal(err)
	}
	const workers, rounds = 8, 25
	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				errs <- db.Transaction(func(tx *gorm.DB) error {
					var n int
					if err := tx.Raw("SELECT n FROM counter WHERE id = 1").Scan(&n).Error; err != nil {
						return err
					}
					return tx.Exec("UPDATE counter SET n = ? WHERE id = 1", n+1).Error
				})
			}
		}()
	}
	wg.Wait()
	close(errs)
	var failed []string
	for err := range errs {
		if err != nil {
			failed = append(failed, err.Error())
		}
	}
	if len(failed) > 0 {
		t.Fatalf("%d of %d transactions failed, first: %s", len(failed), workers*rounds, failed[0])
	}
	var n int
	db.Raw("SELECT n FROM counter WHERE id = 1").Scan(&n)
	if n != workers*rounds {
		t.Fatalf("counter = %d, want %d (lost update)", n, workers*rounds)
	}
}
