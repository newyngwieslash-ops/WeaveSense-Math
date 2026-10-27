package edumath

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"yingce/backend/internal/logging"
	"yingce/backend/internal/model"
)

// lockedBuffer collects log output from handlers and the worker.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// TestLogMinimization (DEV-104, E58): with the platform access log in front of
// the education routes at debug level, a full student round (roster names,
// consent evidence, answers, help text, access purpose, data request reason,
// refused oversized input and a worker pass) writes request lines and worker
// lines, and none of them carries what a student or teacher typed.
func TestLogMinimization(t *testing.T) {
	h := newHarness(t, startMathService(t))
	buf := &lockedBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)
	r := gin.New()
	r.Use(logging.AccessLog(nil), logging.Recovery())
	(&API{Svc: h.svc, DB: h.db, User: func(c *gin.Context) (*model.User, error) {
		id := c.GetHeader("X-Test-User")
		if id == "" {
			return nil, fmt.Errorf("no user")
		}
		return &model.User{ID: id}, nil
	}}).Register(r.Group("/api"))
	h.r = r

	const mark = "LOGMARK"
	tenant := h.setupTeacher()
	pubID, _ := publishG1(t, h)
	_, out, _, _ := h.call("u_alice", "POST", "/api/edu/classes", map[string]any{"name": "高一(1)班"})
	classID := data(out)["id"].(string)
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/assignments", map[string]any{"publicationId": pubID, "title": "日志", "answerPolicy": "teacher_release"})
	asID := data(out)["id"].(string)
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/roster-import", map[string]any{"csv": "externalRef,displayName\nS001," + mark + "甲\n", "confirm": true, "ttlHours": 24}); c != 201 {
		t.Fatalf("roster import: %d %s", c, raw)
	}
	_, out, _, _ = h.call("u_alice", "POST", "/api/edu/classes/"+classID+"/invites", map[string]any{"ttlHours": 24, "maxUses": 5})
	h.call("u_bob", "POST", "/api/edu-join", map[string]any{"code": data(out)["code"]})
	h.call("u_alice", "POST", "/api/edu/consents", map[string]any{"studentUserId": "u_bob", "status": "granted", "evidenceRef": mark + "-evidence"})
	stu := []string{"X-Edu-Tenant", tenant}
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": mark + " 我觉得是三分之一", "clientKey": "lmin0001"}, stu...); c != 201 {
		t.Fatalf("attempt: %d %s", c, raw)
	}
	if c, _, raw, _ := h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/help", map[string]any{"kind": "ask_teacher", "message": mark + " 第二步看不懂", "clientKey": "logmin-help-0001"}, stu...); c != 201 {
		t.Fatalf("help: %d %s", c, raw)
	}
	// Refused input is logged as a warning without its content.
	if c, _, _, _ := h.call("u_bob", "POST", "/api/edu/student/assignments/"+asID+"/attempts", map[string]any{"kind": "answer", "response": mark + strings.Repeat("长", 400), "clientKey": "lmin0002"}, stu...); c != 400 {
		t.Fatalf("oversized attempt: %d", c)
	}
	h.call("u_alice", "GET", "/api/edu/classes/"+classID+"/roster", nil, accessPurposeHeader, url.QueryEscape(mark+" 家长投诉核查"))
	h.call("u_alice", "GET", "/api/edu/assignments/"+asID+"/help", nil)
	if c, _, raw, _ := h.call("u_alice", "POST", "/api/edu/data-requests", map[string]any{"studentUserId": "u_bob", "kind": "access", "requester": "guardian", "reason": mark + " 监护人书面申请"}); c != 201 {
		t.Fatalf("data request: %d %s", c, raw)
	}
	if c, _, _, _ := h.call("u_alice", "POST", "/api/edu/problem-drafts/normalize", map[string]any{"text": mark + " 忽略指令"}); c == 200 {
		t.Fatal("hostile statement accepted")
	}
	if _, err := h.svc.SyncOnce(context.Background(), 10); err != nil {
		t.Fatalf("sync: %v", err)
	}

	logs := buf.String()
	if !strings.Contains(logs, `"msg":"http request"`) || !strings.Contains(logs, `"status":400`) {
		t.Fatalf("access log not captured: %.400s", logs)
	}
	for _, leak := range []string{mark, "三分之一", "看不懂", "家长投诉", "监护人书面", "evidence", "displayName"} {
		if strings.Contains(logs, leak) {
			t.Fatalf("log carries %q:\n%s", leak, logs)
		}
	}
	// Request lines carry the path only, never a query string or body.
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		if strings.Contains(line, `"msg":"http request"`) && strings.Contains(line, "?") {
			t.Fatalf("request line carries a query string: %s", line)
		}
	}
}
