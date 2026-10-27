package edumath

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"yingce/backend/internal/database"
	"yingce/backend/internal/flow"
	"yingce/backend/internal/model"
)

// TestRestoreDrill (DEV-122, E65) on SQLite: the backup is a VACUUM INTO copy.
func TestRestoreDrill(t *testing.T) {
	base := startMathService(t)
	runRestoreDrill(t, newHarness(t, base), func(h *harness) *harness {
		p := filepath.Join(t.TempDir(), "backup.db")
		if err := h.db.Exec("VACUUM INTO ?", p).Error; err != nil {
			t.Fatal(err)
		}
		db, err := gorm.Open(sqlite.Open("file:"+p), &gorm.Config{Logger: logger.Discard})
		if err != nil {
			t.Fatal(err)
		}
		s, _ := db.DB()
		s.SetMaxOpenConns(1)
		t.Cleanup(func() { s.Close() })
		return newHarnessDB(t, base, db)
	})
}

// TestPGRestoreDrill on PostgreSQL: the backup is a file level copy of the live
// database (CREATE DATABASE ... TEMPLATE) taken while the pool is idle, and the
// restore is a fresh platform pointed at that copy, under FORCE RLS.
func TestPGRestoreDrill(t *testing.T) {
	base := startMathService(t)
	runRestoreDrill(t, newHarnessDB(t, base, pgTestDB(t)), func(h *harness) *harness {
		var live string
		h.db.Raw("SELECT current_database()").Scan(&live)
		s, _ := h.db.DB()
		s.SetMaxIdleConns(0)
		defer s.SetMaxIdleConns(2)
		admin, err := database.Open(database.Config{Driver: "postgres", DSN: os.Getenv("FLOW_TEST_PG_RLS_DSN")})
		if err != nil {
			t.Fatal(err)
		}
		as, _ := admin.DB()
		defer as.Close()
		snap := live + "_bk"
		start := time.Now()
		if err := admin.Exec("CREATE DATABASE " + snap + " TEMPLATE " + live).Error; err != nil {
			t.Fatal(err)
		}
		t.Logf("backup copy took %s", time.Since(start))
		t.Cleanup(func() {
			if a2, err := database.Open(database.Config{Driver: "postgres", DSN: os.Getenv("FLOW_TEST_PG_RLS_DSN")}); err == nil {
				a2.Exec("DROP DATABASE IF EXISTS " + snap + " WITH (FORCE)")
				if s2, e := a2.DB(); e == nil {
					s2.Close()
				}
			}
		})
		u, _ := url.Parse(os.Getenv("FLOW_TEST_PG_RLS_DSN"))
		u.Path = "/" + snap
		db, err := database.Open(database.Config{Driver: "postgres", DSN: u.String()})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if s2, e := db.DB(); e == nil {
				s2.Close()
			}
		})
		return newHarnessDB(t, base, db)
	})
}

// runRestoreDrill (DEV-122, E65): withdrawals and deletions done after the
// backup point are journaled outside the database once committed; restoring the
// backup brings the withdrawn lesson and the deleted student's records back;
// the replay (dry run first, org admin only, backup time required) re-applies
// exactly the post backup entries, recreates the incident and request markers,
// is idempotent, skips older entries, and refuses to undo a later legitimate
// state (new consent) which it reports as superseded; afterwards the restored
// platform matches the live one on every replayed fact.
func runRestoreDrill(t *testing.T, h *harness, backup func(*harness) *harness) {
	journal := &RestoreJournal{Path: filepath.Join(t.TempDir(), "edu-restore.jsonl")}
	h.svc.Journal = journal
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高三(2)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "恢复演练", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	code := data(out)["code"]
	stu := []string{"X-Edu-Tenant", tenant}
	for i, u := range []string{"u_bob", "u_eve"} {
		h.call(u, "POST", "/api/edu-join", map[string]any{"code": code})
		h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": u, "status": "granted", "evidenceRef": "form-rd-" + u})
		if c, _, raw, _ := h.call(u, "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": fmt.Sprintf("rd-attempt-%04d", i)}, stu...); c != 201 {
			t.Fatalf("attempt %s: %d %s", u, c, raw)
		}
	}
	completeReq := func(u, kind string) string {
		t.Helper()
		c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/data-requests", map[string]any{"studentUserId": u, "kind": kind, "requester": "guardian", "reason": "监护人书面申请"})
		if c != 201 {
			t.Fatalf("create %s: %d %s", kind, c, raw)
		}
		id := data(out)["id"].(string)
		h.call("u_alice", "POST", "/api/edu/data-requests/"+id+"/verify", map[string]any{"verificationRef": "核验单 RD-01"})
		if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/data-requests/"+id+"/complete", map[string]any{"confirm": true}); c != 200 {
			t.Fatalf("complete %s: %d %s", kind, c, raw)
		}
		return id
	}
	// Eve withdrew two hours before the backup and has since given new consent.
	h.now.Store(time.Now().Add(-2 * time.Hour).Unix())
	eveReq := completeReq("u_eve", "withdraw")
	h.now.Store(0)
	// The institution later re-admitted her (seeded: no endpoint re-activates a
	// revoked student role) and recorded new consent.
	if err := flow.TenantTx(h.db, tenant, func(tx *gorm.DB) error {
		return tx.Model(&model.EduRoleBinding{}).Where("tenant_id = ? AND user_id = ? AND role = ?", tenant, "u_eve", "student").
			Updates(map[string]any{"status": "active", "updated_at": time.Now().UTC()}).Error
	}); err != nil {
		t.Fatal(err)
	}
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_eve", "status": "granted", "evidenceRef": "form-rd-eve-2"}); c != 200 {
		t.Fatalf("eve consent again: %d %s", c, raw)
	}

	backupAt := time.Now().UTC().Truncate(time.Second)
	time.Sleep(1100 * time.Millisecond)
	r := backup(h)
	r.svc.Journal = journal

	// After the backup: a blocking incident withdraws the lesson and Bob is deleted.
	inc := "/api/edu/publications/" + pubID + "/incidents"
	c, out, raw, _ := h.call("u_alice", "POST", inc, map[string]any{"severity": "blocking", "reason": "答案错误", "confirm": true})
	if c != 201 {
		t.Fatalf("incident: %d %s", c, raw)
	}
	incID := data(out)["incident"].(map[string]any)["id"].(string)
	bobReq := completeReq("u_bob", "delete")
	// A rolled back action is never journaled: an invalid incident is refused.
	h.call("u_alice", "POST", inc, map[string]any{"severity": "urgent", "reason": "x"})

	raw2, _ := os.ReadFile(journal.Path)
	lines := strings.Split(strings.TrimSpace(string(raw2)), "\n")
	if len(lines) != 3 {
		t.Fatalf("journal must hold eve withdraw, the withdrawal and bob delete: %q", raw2)
	}
	for _, bad := range []string{"答案错误", "监护人", "核验单", "form-rd"} {
		if strings.Contains(string(raw2), bad) {
			t.Fatalf("journal must hold ids only, found %q", bad)
		}
	}

	state := func(x *harness) (pub, asg, role string, bobAttempts, liveArtifacts, incN, reqN int64) {
		var p model.EduPublication
		x.db.First(&p, "id = ?", pubID)
		var a model.EduAssignment
		x.db.First(&a, "id = ?", asID)
		var rb model.EduRoleBinding
		x.db.First(&rb, "tenant_id = ? AND user_id = ? AND role = ?", tenant, "u_bob", "student")
		x.db.Model(&model.EduAttempt{}).Where("tenant_id = ? AND student_user_id = ?", tenant, "u_bob").Count(&bobAttempts)
		var l model.EduLessonRevision
		x.db.First(&l, "id = ?", p.LessonRevisionID)
		x.db.Model(&model.EduArtifact{}).Where("id IN ? AND revoked_at IS NULL", studentArtifactsOf(l, nil)).Count(&liveArtifacts)
		x.db.Model(&model.EduContentIncident{}).Where("id = ?", incID).Count(&incN)
		x.db.Model(&model.EduDataRequest{}).Where("id = ? AND status = ?", bobReq, "completed").Count(&reqN)
		return p.State, a.State, rb.Status, bobAttempts, liveArtifacts, incN, reqN
	}
	if p, a, rl, n, art, i, q := state(h); p != "withdrawn" || a != "suspended" || rl != "revoked" || n != 0 || art != 0 || i != 1 || q != 1 {
		t.Fatalf("live state: %s %s %s attempts=%d artifacts=%d inc=%d req=%d", p, a, rl, n, art, i, q)
	}
	// The restored backup has the lesson published and Bob's data back.
	if p, a, rl, n, art, i, q := state(r); p != "published" || a != "active" || rl != "active" || n != 1 || art == 0 || i != 0 || q != 0 {
		t.Fatalf("restored state before replay: %s %s %s attempts=%d artifacts=%d inc=%d req=%d", p, a, rl, n, art, i, q)
	}

	rp := "/api/edu/restore/replay"
	since := backupAt.Format(time.RFC3339)
	if c, _, _, _ := r.call("u_alice", "POST", rp, map[string]any{}); c != 400 {
		t.Fatalf("since is required, got %d", c)
	}
	for _, u := range []string{"u_bob", "u_carol"} {
		if c, _, _, _ := r.call(u, "POST", rp, map[string]any{"since": since, "confirm": true}); c != 403 && c != 404 {
			t.Fatalf("%s must not replay, got %d", u, c)
		}
	}
	r.svc.Journal = nil
	if c, out, _, _ := r.call("u_alice", "POST", rp, map[string]any{"since": since}); c != 409 || errCode(out) != "RESTORE_JOURNAL_MISSING" {
		t.Fatalf("missing journal must be explicit, got %d", c)
	}
	r.svc.Journal = journal
	// A torn last line (crash mid write) is skipped and counted.
	f, _ := os.OpenFile(journal.Path, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(`{"at":"2026-`)
	f.Close()

	replay := func(body map[string]any) map[string]any {
		t.Helper()
		c, out, raw, _ := r.call("u_alice", "POST", rp, body)
		if c != 200 {
			t.Fatalf("replay: %d %s", c, raw)
		}
		return data(out)
	}
	outcomes := func(d map[string]any) string {
		o := d["outcomes"].(map[string]any)
		return fmt.Sprintf("%v/%v/%v/%v/%v", o["would_reapply"], o["reapplied"], o["already_applied"], o["superseded"], o["absent"])
	}
	d := replay(map[string]any{"since": since})
	if d["dryRun"] != true || d["entries"].(float64) != 2 || d["olderSkipped"].(float64) != 1 || d["tornLines"].(float64) != 1 || outcomes(d) != "2/<nil>/<nil>/<nil>/<nil>" {
		t.Fatalf("dry run: %v", d)
	}
	if p, _, rl, n, _, _, _ := state(r); p != "published" || rl != "active" || n != 1 {
		t.Fatal("a dry run must not change anything")
	}
	d = replay(map[string]any{"since": since, "confirm": true})
	if d["dryRun"] != false || outcomes(d) != "<nil>/2/<nil>/<nil>/<nil>" {
		t.Fatalf("replay: %v", d)
	}
	if p, a, rl, n, art, i, q := state(r); p != "withdrawn" || a != "suspended" || rl != "revoked" || n != 0 || art != 0 || i != 1 || q != 1 {
		t.Fatalf("restored state after replay: %s %s %s attempts=%d artifacts=%d inc=%d req=%d", p, a, rl, n, art, i, q)
	}
	// Restored and live now agree on every replayed fact.
	lp, la, lr, ln, lart, li, lq := state(h)
	rp2, ra, rr, rn, rart, ri, rq := state(r)
	if lp != rp2 || la != ra || lr != rr || ln != rn || lart != rart || li != ri || lq != rq {
		t.Fatal("restored state differs from live after replay")
	}
	var eveAttempts int64
	r.db.Model(&model.EduAttempt{}).Where("tenant_id = ? AND student_user_id = ?", tenant, "u_eve").Count(&eveAttempts)
	if eveAttempts != 1 {
		t.Fatal("a classmate's records must stay")
	}
	d = replay(map[string]any{"since": since, "confirm": true})
	if outcomes(d) != "<nil>/<nil>/2/<nil>/<nil>" {
		t.Fatalf("replay must be idempotent: %v", d)
	}
	// Reaching back past Eve's withdrawal must not undo her later consent.
	d = replay(map[string]any{"since": backupAt.Add(-3 * time.Hour).Format(time.RFC3339)})
	items := d["items"].([]any)
	if outcomes(d) != "<nil>/<nil>/2/1/<nil>" || items[0].(map[string]any)["ref"] != eveReq || items[0].(map[string]any)["outcome"] != "superseded" {
		t.Fatalf("older entry with later consent: %v", d)
	}
	var granted int64
	r.db.Model(&model.EduConsentRecord{}).Where("tenant_id = ? AND student_user_id = ? AND status = ?", tenant, "u_eve", "granted").Count(&granted)
	if granted == 0 {
		t.Fatal("eve's later consent must stay granted")
	}
	var audits int64
	r.db.Model(&model.EduAuditEvent{}).Where("tenant_id = ? AND action = ?", tenant, "restore.replay").Count(&audits)
	if audits != 2 {
		t.Fatalf("each confirmed replay is audited, got %d", audits)
	}
}
