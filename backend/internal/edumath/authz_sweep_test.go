package edumath

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"gorm.io/gorm/schema"
	"sync"

	"yingce/backend/internal/model"
)

func TestAuthzSweep(t *testing.T) { runAuthzSweep(t, newHarness(t, startMathService(t))) }

func TestPGAuthzSweep(t *testing.T) {
	runAuthzSweep(t, newHarnessDB(t, startMathService(t), pgTestDB(t)))
}

// runAuthzSweep (DEV-021): every registered education route is called by an
// anonymous caller, by a teacher of another tenant and by a student of the
// class (on staff routes), with path ids of real resources of alice's tenant
// and a body naming them. Anonymous gets 401 everywhere; nobody else changes
// any row of alice's tenant (digest of every edu table before and after), and
// no response names alice's resources or carries the answer.
func runAuthzSweep(t *testing.T, h *harness) {
	tenant := h.setupTeacher()
	// carol is a real teacher and org admin of her own tenant, so the sweep reaches the resource checks.
	for _, r := range []string{"teacher", "org_admin"} {
		if c, _, raw, _ := h.call("u_carol", "POST", "/api/edu/roles", map[string]any{"tenantId": model.PersonalTenantID("u_carol"), "userId": "u_carol", "role": r}); c != 200 {
			t.Fatalf("bind carol %s: %d %s", r, c, raw)
		}
	}
	pubID, tl := publishG1(t, h)
	var pub model.EduPublication
	h.db.First(&pub, "id = ?", pubID)
	var lesson model.EduLessonRevision
	h.db.First(&lesson, "id = ?", pub.LessonRevisionID)
	var job model.EduMathJob
	h.db.First(&job, "id = ?", lesson.JobID)
	var prev model.EduProblemRevision
	h.db.Where("tenant_id = ?", tenant).First(&prev)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高二(5)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "授权扫描", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	inviteID := data(out)["invite"].(map[string]any)["id"].(string)
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": "form-sweep"})
	stu := []string{"X-Edu-Tenant", tenant}
	h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": "1", "clientKey": "bob-sweep-01"}, stu...)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/source-assets", map[string]any{"title": "教材例题", "kind": "textbook", "attribution": "人教A版", "allowStudent": true, "allowDerivative": true})
	srcID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/courses", map[string]any{"name": "高二数学", "knowledgePoints": []string{"线面角"}})
	courseID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/courses/"+courseID+"/alignments", map[string]any{"lessonRevisionId": lesson.ID, "knowledgePoints": []string{"线面角"}})
	alignID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/roster-import", map[string]any{"csv": "externalRef\nS9", "confirm": true})
	rosterID := data(out)["codes"].([]any)[0].(map[string]any)["entryId"].(string)
	var art model.EduArtifact
	h.db.Where("tenant_id = ? AND id = ?", tenant, lesson.PreviewArtifact).First(&art)
	c, out, raw := h.uploadImage("u_alice", pngWithText(64, 48, "sweep"), "sweep.png", "image/png", srcID)
	if c != 201 {
		t.Fatalf("upload sweep picture: %d %s", c, raw)
	}
	imgID := data(out)["id"].(string)

	ids := map[string]string{"classes": classID, "assignments": asID, "publications": pubID, "lesson-revisions": lesson.ID, "problems": prev.ProblemID,
		"problem-revisions": prev.ID, "jobs": job.ID, "artifacts": art.ID, "source-assets": srcID, "courses": courseID, "course-alignments": alignID,
		"roster-entries": rosterID, "invites": inviteID, "incidents": "einc_none", "variant-sets": "evs_none", "regrades": "erd_none", "problem-images": imgID}
	secret := []string{classID, asID, pubID, lesson.ID, prev.ID, job.ID, srcID, courseID, alignID, rosterID, inviteID, imgID, lesson.LessonHash}
	answer := []string{tl.Answer.Exact, tl.Answer.Plain}
	body := map[string]any{"publicationId": pubID, "classId": classID, "lessonRevisionId": lesson.ID, "problemRevisionId": prev.ID, "studentUserId": "u_bob",
		"userId": "u_carol", "tenantId": tenant, "role": "org_admin", "lessonHash": lesson.LessonHash, "decision": "approved", "comment": "x", "reason": "授权扫描",
		"confirm": true, "severity": "blocking", "note": "x", "sourceAssetId": srcID, "courseId": courseID, "knowledgePoints": []string{"线面角"},
		"status": "granted", "evidenceRef": "x", "reviewMode": "self_allowed", "consentMode": "not_applicable", "csv": "externalRef\nX1", "title": "x", "name": "x",
		"idempotencyKey": "sweep", "kind": "answer", "response": "1", "clientKey": "sweep-0001", "visibility": "class", "released": true, "version": 1, "imageId": imgID, "whole": true}

	digest := func() map[string]string {
		out := map[string]string{}
		for _, m := range model.EduModels() {
			s, err := schema.Parse(m, &sync.Map{}, h.db.NamingStrategy)
			if err != nil {
				t.Fatal(err)
			}
			if s.LookUpField("TenantID") == nil {
				continue
			}
			var rows []map[string]any
			h.db.Table(s.Table).Where("tenant_id = ?", tenant).Order("id").Find(&rows)
			b, _ := json.Marshal(rows)
			sum := sha256.Sum256(b)
			out[s.Table] = hex.EncodeToString(sum[:])
		}
		return out
	}
	// The ids are live: the owner can read them.
	for _, p := range []string{"/api/edu/classes/" + classID + "/roster", "/api/edu/courses/" + courseID, "/api/edu/source-assets/" + srcID, "/api/edu/lesson-revisions/" + lesson.ID} {
		if c, _, raw, _ := h.call("u_alice", "GET", p, nil); c != 200 {
			t.Fatalf("owner read %s: %d %s", p, c, raw)
		}
	}
	before := digest()

	fill := func(path string) string {
		segs := strings.Split(path, "/")
		last := ""
		for i, s := range segs {
			if strings.HasPrefix(s, ":") {
				v := ids[last]
				switch {
				case s == ":studentId" || s == ":userId":
					v = "u_bob"
				case s == ":inviteId":
					v = inviteID
				case s == ":eid":
					v = rosterID
				case v == "":
					v = "x_unknown"
				}
				segs[i] = v
			} else {
				last = s
			}
		}
		return strings.Join(segs, "/")
	}
	n := 0
	for _, rt := range h.r.Routes() {
		if !strings.HasPrefix(rt.Path, "/api/edu/") {
			continue
		}
		p := fill(rt.Path)
		n++
		// /files/:id is the signed short-lived link: no session, the signature decides (403 without it).
		if c, _, raw, _ := h.call("", rt.Method, p, body); c != 401 && !(rt.Path == "/api/edu/files/:id" && c == 403) {
			t.Errorf("anonymous %s %s: %d %s", rt.Method, rt.Path, c, raw)
		}
		check := func(who string, c int, raw []byte) {
			t.Helper()
			for _, a := range answer {
				if a != "" && strings.Contains(string(raw), a) {
					t.Errorf("%s %s %s leaked the answer: %d", who, rt.Method, rt.Path, c)
				}
			}
			if c < 300 {
				for _, s := range secret {
					if strings.Contains(string(raw), s) {
						t.Errorf("%s %s %s exposed %s: %d %s", who, rt.Method, rt.Path, s, c, raw)
					}
				}
			}
		}
		c, _, raw, _ := h.call("u_carol", rt.Method, p, body)
		check("other-tenant teacher", c, raw)
		c, _, raw, _ = h.call("u_carol", rt.Method, p, body, "X-Workspace-Id", model.PersonalWorkspaceID("u_alice"))
		check("other-tenant teacher naming the victim workspace", c, raw)
		c, _, raw, _ = h.call("u_carol", rt.Method, p, body, stu...)
		check("non-student with student header", c, raw)
		if !strings.HasPrefix(rt.Path, "/api/edu/student/") && rt.Path != "/api/edu/me" {
			c, _, raw, _ = h.call("u_bob", rt.Method, p, body, stu...)
			check("student on staff route", c, raw)
			c, _, raw, _ = h.call("u_bob", rt.Method, p, body)
			check("student without tenant header", c, raw)
		}
	}
	if n < 80 {
		t.Fatalf("route sweep looks incomplete: %d routes", n)
	}
	after := digest()
	for tbl, d := range before {
		if after[tbl] != d {
			t.Errorf("table %s of the victim tenant changed during the sweep", tbl)
		}
	}
	t.Logf("swept %d education routes", n)
}
