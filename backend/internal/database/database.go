package database

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"yingce/backend/internal/logging"

	"github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Config struct {
	Driver  string
	DSN     string
	DataDir string
}

func Open(config Config) (*gorm.DB, error) {
	driver := strings.ToLower(strings.TrimSpace(config.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	switch driver {
	case "sqlite":
		dsn := strings.TrimSpace(config.DSN)
		if dsn == "" {
			if err := os.MkdirAll(config.DataDir, 0o755); err != nil {
				return nil, err
			}
			dsn = config.DataDir + "/open_ai_canvas.db?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on&_synchronous=NORMAL"
		}
		return gorm.Open(sqlite.Open(SQLiteDSN(dsn)), &gorm.Config{Logger: logging.GormLogger()})
	case "postgres", "postgresql":
		dsn := strings.TrimSpace(config.DSN)
		if dsn == "" {
			return nil, errors.New("PostgreSQL 模式必须配置 DATABASE_URL")
		}
		return gorm.Open(postgres.New(postgres.Config{
			DSN:          dsn,
			OptionOpenDB: []stdlib.OptionOpenDB{stdlib.OptionBeforeConnect(configurePostgresConnection)},
		}), &gorm.Config{Logger: logging.GormLogger()})
	default:
		return nil, fmt.Errorf("不支持的数据库驱动：%s", driver)
	}
}

// SQLiteDSN makes every transaction on a writable SQLite connection start with
// BEGIN IMMEDIATE (_txlock=immediate). With the driver default (deferred), a
// transaction that reads first and writes later must upgrade its SHARED lock
// to RESERVED; when another connection already holds RESERVED, SQLite returns
// SQLITE_BUSY ("database is locked") at once instead of waiting for
// _busy_timeout, because waiting could deadlock. Taking the write lock at
// BEGIN makes concurrent writers queue on busy_timeout instead.
//
// Cost: read-only transactions also take the write reservation for their
// duration (the driver ignores sql.TxOptions.ReadOnly). In WAL mode plain
// readers outside transactions are unaffected; only other writers wait.
//
// A DSN that already names _txlock is kept as is, and read-only opens
// (mode=ro) are left deferred because BEGIN IMMEDIATE fails on them.
func SQLiteDSN(dsn string) string {
	path, query, _ := strings.Cut(dsn, "?")
	params, err := url.ParseQuery(query)
	if err != nil {
		return dsn
	}
	if params.Has("_txlock") || params.Get("mode") == "ro" {
		return dsn
	}
	if query == "" {
		return path + "?_txlock=immediate"
	}
	return dsn + "&_txlock=immediate"
}

func ConfigurePool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	if db.Dialector.Name() == "postgres" {
		sqlDB.SetMaxOpenConns(30)
		sqlDB.SetMaxIdleConns(10)
		return nil
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(4)
	return nil
}
