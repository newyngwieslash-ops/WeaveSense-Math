package edumath

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

func TestRenderRollbackDrill(t *testing.T) {
	runRenderRollbackDrill(t, func(base string) *harness { return newHarness(t, base) })
}

func TestPGRenderRollbackDrill(t *testing.T) {
	db := pgTestDB(t)
	runRenderRollbackDrill(t, func(base string) *harness { return newHarnessDB(t, base, db) })
}

// mathVersion is one deployable code version of services/math-service, copied
// into its own directory so that two versions really exist side by side.
type mathVersion struct{ name, root string }

// copyMathService copies the service source tree; patch edits files relative to
// the copy and must change each file it names.
func copyMathService(t *testing.T, name string, patch map[string][2]string) mathVersion {
	t.Helper()
	src, _ := filepath.Abs("../../../services/math-service")
	dst := filepath.Join(t.TempDir(), name)
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	_ = os.RemoveAll(filepath.Join(dst, "mathsvc", "__pycache__"))
	for file, edit := range patch {
		p := filepath.Join(dst, file)
		b, err := os.ReadFile(p)
		if err != nil || !bytes.Contains(b, []byte(edit[0])) {
			t.Fatalf("patch %s: %q not found", file, edit[0])
		}
		if err := os.WriteFile(p, bytes.Replace(b, []byte(edit[0]), []byte(edit[1]), 1), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return mathVersion{name: name, root: dst}
}

// runMathVersion starts one version on a fixed address and data directory (the
// way a deployment replaces the service behind one internal URL and one
// persistent volume) and returns a stop function.
func runMathVersion(t *testing.T, v mathVersion, addr, dataDir string) func() {
	t.Helper()
	py := os.Getenv("MATH_PYTHON")
	if py == "" {
		py = "python3"
	}
	if err := exec.Command(py, "-c", "import sympy").Run(); err != nil {
		if os.Getenv("EDU_MATH_REQUIRE_SERVICE") == "1" {
			t.Fatalf("math service python unavailable: %v", err)
		}
		t.Skip("python3 with sympy not available; set MATH_PYTHON")
	}
	cmd := exec.Command(py, "-m", "mathsvc.server")
	cmd.Dir = v.root
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "MATH_SERVICE_KEY=" + testKey, "MATH_DATA_DIR=" + dataDir, "MATH_LISTEN=" + addr}
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("%s log:\n%s", v.name, logs.String())
		}
	}
	t.Cleanup(stop)
	for i := 0; i < 150; i++ {
		if r, err := http.Get("http://" + addr + "/healthz"); err == nil {
			r.Body.Close()
			return stop
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s did not start:\n%s", v.name, logs.String())
	return stop
}

// runRenderRollbackDrill (DEV-125, E66) is a real switch between two code
// versions of the math service. The previous valid version A renders with
// preview-svg/1.1.0. Candidate B is the same tree with a new renderer version
// and a presentation defect the render check does not catch (the hint list is
// hidden). The drill upgrades A to B on the same address and data volume,
// publishes a B lesson and moves a class onto it, finds the defect, pauses the
// B renderer (students blocked at once), switches the service back to A, moves
// the class back to the A version and checks: students read the A lesson again
// with hints visible, new lessons are rendered by A and can be published, B
// lessons stay paused, and no attempt, job, stage settlement or ledger row was
// lost. Timings are logged as drill evidence (go test -json keeps them in CI).
func runRenderRollbackDrill(t *testing.T, mk func(base string) *harness) {
	const good, bad = "preview-svg/1.1.0", "preview-svg/1.2.0-drill"
	vA := copyMathService(t, "math-A", nil)
	vB := copyMathService(t, "math-B", map[string][2]string{
		"mathsvc/render.py": {`RENDERER_VERSION = "` + good + `"`, `RENDERER_VERSION = "` + bad + `"`},
	})
	// The defect, in a second edit of the same file.
	{
		p := filepath.Join(vB.root, "mathsvc", "render.py")
		b, _ := os.ReadFile(p)
		if !bytes.Contains(b, []byte("<h2>分级提示</h2><ol>")) {
			t.Fatal("defect patch target not found")
		}
		_ = os.WriteFile(p, bytes.Replace(b, []byte("<h2>分级提示</h2><ol>"), []byte("<h2>分级提示</h2><ol hidden>"), 1), 0o644)
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	vol := t.TempDir()
	evidence := map[string]any{"previous": vA.name + " " + good, "candidate": vB.name + " " + bad}

	stopA := runMathVersion(t, vA, addr, vol)
	h := mk("http://" + addr)
	tenant := h.setupTeacher()
	p1, _ := publishG1(t, h)
	sol := solutionOf(h, p1)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高一(8)班"})
	classID := data(out)["id"].(string)
	c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": p1, "title": "回退演练", "answerPolicy": "teacher_release"})
	if c != 201 {
		t.Fatalf("assignment: %d %s", c, raw)
	}
	as1 := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-drill-bob"})
	stu := []string{"X-Edu-Tenant", tenant}
	sa := "/api/edu/student/assignments/"
	if c, _, raw, _ := h.call("u_bob", "POST", sa+as1+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "drill-attempt-01"}, stu...); c != 201 {
		t.Fatalf("attempt on A: %d %s", c, raw)
	}
	studentHTML := func(as string) (int, string) {
		t.Helper()
		c, out, _, _ := h.call("u_bob", "GET", sa+as, nil, stu...)
		if c != 200 {
			return c, ""
		}
		code, body, _ := fetch(h, "", data(out)["preview"].(map[string]any)["url"].(string))
		if code != 200 {
			t.Fatalf("student preview: %d", code)
		}
		return 200, body
	}
	if c, body := studentHTML(as1); c != 200 || !strings.Contains(body, "renderer "+good) || strings.Contains(body, "<ol hidden>") {
		t.Fatalf("A lesson before the upgrade: %d", c)
	}

	// Upgrade: replace A with B behind the same address and volume.
	t0 := time.Now()
	stopA()
	stopB := runMathVersion(t, vB, addr, vol)
	evidence["upgrade_switch_ms"] = time.Since(t0).Milliseconds()
	l2, p2 := buildAndPublish(t, h, sol, "drill-l2")
	if rendererOf(l2) != bad {
		t.Fatalf("B must render the new lesson, got %q", rendererOf(l2))
	}
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/assignments/"+as1+"/migrate", map[string]any{"publicationId": p2, "reason": "升级到新渲染版本", "confirm": true})
	if c != 201 {
		t.Fatalf("migrate to B: %d %s", c, raw)
	}
	as2 := migratedTo(h, out, as1)
	if c, body := studentHTML(as2); c != 200 || !strings.Contains(body, "renderer "+bad) || !strings.Contains(body, "<ol hidden>") {
		t.Fatalf("B lesson must carry the defect: %d", c)
	}
	if c, _, raw, _ := h.call("u_bob", "POST", sa+as2+"/attempts", map[string]any{"kind": "answer", "response": "2", "clientKey": "drill-attempt-02"}, stu...); c != 201 {
		t.Fatalf("attempt on B: %d %s", c, raw)
	}
	type snap struct{ attempts, jobs, stages, stageCost, ledger int64 }
	take := func() snap {
		var s snap
		h.db.Model(&model.EduAttempt{}).Where("tenant_id = ?", tenant).Count(&s.attempts)
		h.db.Model(&model.EduMathJob{}).Where("tenant_id = ?", tenant).Count(&s.jobs)
		h.db.Model(&model.EduStageRun{}).Where("tenant_id = ?", tenant).Count(&s.stages)
		h.db.Model(&model.EduStageRun{}).Where("tenant_id = ?", tenant).Select("COALESCE(SUM(cost_micro), 0)").Scan(&s.stageCost)
		h.db.Model(&model.CreditLedgerEntry{}).Count(&s.ledger)
		return s
	}
	before := take()

	// Incident: the defect is found. Pause the B renderer, then roll back.
	tIncident := time.Now()
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/pauses", map[string]any{"scope": "renderer", "targetId": bad, "reason": "渲染版本隐藏了分级提示", "confirm": true})
	if c != 201 {
		t.Fatalf("renderer pause: %d %s", c, raw)
	}
	if c, _ := studentHTML(as2); c != 423 {
		t.Fatalf("students must be blocked from B lessons at once, got %d", c)
	}
	evidence["pause_ms"] = time.Since(tIncident).Milliseconds()
	t1 := time.Now()
	stopB()
	runMathVersion(t, vA, addr, vol)
	evidence["rollback_switch_ms"] = time.Since(t1).Milliseconds()
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/assignments/"+as2+"/migrate", map[string]any{"publicationId": p1, "reason": "回退到上一有效渲染版本", "confirm": true})
	if c != 201 {
		t.Fatalf("migrate back to A: %d %s", c, raw)
	}
	as3 := migratedTo(h, out, as2)
	c, body := studentHTML(as3)
	if c != 200 || !strings.Contains(body, "renderer "+good) || strings.Contains(body, "<ol hidden>") || !strings.Contains(body, "提示 1") {
		t.Fatalf("after rollback students must read the A lesson with hints: %d", c)
	}
	evidence["incident_to_students_back_ms"] = time.Since(tIncident).Milliseconds()
	if c, _, raw, _ := h.call("u_bob", "POST", sa+as3+"/attempts", map[string]any{"kind": "answer", "response": "3", "clientKey": "drill-attempt-03"}, stu...); c != 201 {
		t.Fatalf("attempt after rollback: %d %s", c, raw)
	}
	after := take()
	if after.attempts != before.attempts+1 || after.jobs != before.jobs || after.stages != before.stages || after.stageCost != before.stageCost || after.ledger < before.ledger {
		t.Fatalf("the rollback must lose nothing: %+v -> %+v", before, after)
	}
	// New work after the rollback is rendered by A and is not caught by the B pause.
	l3, _ := buildAndPublish(t, h, sol, "drill-l3")
	if rendererOf(l3) != good {
		t.Fatalf("after rollback new lessons must be rendered by A, got %q", rendererOf(l3))
	}
	// B lessons stay paused until re-rendered and re-reviewed.
	if c, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": p2, "title": "x", "answerPolicy": "teacher_release"}); c != 423 || errCode(out) != "CONTENT_PAUSED" {
		t.Fatalf("a B publication must stay paused after the rollback: %d", c)
	}
	evidence["attempts_before_after"] = []int64{before.attempts, after.attempts}
	evidence["stage_runs_before_after"] = []int64{before.stages, after.stages}
	evidence["stage_cost_micro_before_after"] = []int64{before.stageCost, after.stageCost}
	b, _ := json.Marshal(evidence)
	t.Logf("render rollback drill OK: %s", b)
}

func solutionOf(h *harness, pubID string) string {
	var p model.EduPublication
	h.db.First(&p, "id = ?", pubID)
	var l model.EduLessonRevision
	h.db.First(&l, "id = ?", p.LessonRevisionID)
	return l.SolutionRevisionID
}

func buildAndPublish(t *testing.T, h *harness, sol, key string) (model.EduLessonRevision, string) {
	t.Helper()
	c, out, raw, _ := h.call("u_alice", "POST", "/api/edu/solution-revisions/"+sol+"/lesson", map[string]any{"idempotencyKey": key})
	if c >= 300 {
		t.Fatalf("lesson %s: %d %s", key, c, raw)
	}
	j := h.waitJob("u_alice", data(out)["id"].(string))
	id, _ := j["lessonRevisionId"].(string)
	if id == "" {
		t.Fatalf("lesson %s did not produce a revision: %v", key, j)
	}
	var l model.EduLessonRevision
	h.db.First(&l, "id = ?", id)
	h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+id+"/review", map[string]any{"lessonHash": l.LessonHash, "decision": "approved", "comment": "自审"})
	c, out, raw, _ = h.call("u_alice", "POST", "/api/edu/lesson-revisions/"+id+"/publish", map[string]any{"lessonHash": l.LessonHash})
	if c != 200 {
		t.Fatalf("publish %s: %d %s", key, c, raw)
	}
	return l, data(out)["id"].(string)
}

func migratedTo(h *harness, out map[string]any, from string) string {
	if s, ok := data(out)["toAssignmentId"].(string); ok && s != "" {
		return s
	}
	var m model.EduAssignmentMigration
	h.db.First(&m, "from_assignment_id = ?", from)
	return m.ToAssignmentID
}
