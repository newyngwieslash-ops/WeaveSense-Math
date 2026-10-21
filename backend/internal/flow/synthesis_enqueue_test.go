package flow

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

func TestSynthesisQuestionEnqueue(t *testing.T) {
	checkSynthesisQuestionEnqueue(t, testDB(t))
}

func TestSynthesisQuestionEnqueuePostgres(t *testing.T) {
	db := pgTestDB(t)
	if err := db.Create(&model.User{ID: "u_alice", Username: "u_alice"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveScope(db, "u_alice", ""); err != nil {
		t.Fatal(err)
	}
	checkSynthesisQuestionEnqueue(t, db)
}

func checkSynthesisQuestionEnqueue(t *testing.T, db *gorm.DB) {
	t.Helper()
	b := Billing{DB: db}
	fund(t, b, "u_alice", 5_000_000)
	tenant := model.PersonalTenantID("u_alice")
	gw := &scriptedGW{}
	count := func(table string) int64 {
		var n int64
		if err := db.Table(table).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	allocated := func(id string) int64 {
		if id == "" {
			return 0
		}
		var r model.FlowBudgetReservation
		if err := db.First(&r, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		return r.AllocatedMicro
	}
	for _, child := range []bool{false, true} {
		for _, question := range []string{"short", strings.Repeat("a", 80), strings.Repeat("a", 81), strings.Repeat("问", 80), strings.Repeat("问", 81), strings.Repeat("🧵", 81)} {
			name := "root/"
			if child {
				name = "child/"
			}
			name += fmt.Sprintf("%c/%d", []rune(question)[0], utf8.RuneCountInString(question))
			t.Run(name, func(t *testing.T) {
				req := SynthesisRequest{TenantID: tenant, Key: NewID("test_"), Purpose: "research", Question: question, BudgetMicro: 1000, MaxTokens: 512}
				if child {
					parent, err := b.Reserve(tenant, req.Key+":parent", "agent_run", "run", 10_000)
					if err != nil {
						t.Fatal(err)
					}
					req.ParentResID = parent.ID
				}
				_, task, err := StartSynthesis(gw, b, req)
				if err != nil {
					t.Fatalf("enqueue %d-character question: %v", utf8.RuneCountInString(question), err)
				}
				var reservation model.FlowBudgetReservation
				if err := db.First(&reservation, "id = ?", task.ReservationID).Error; err != nil {
					t.Fatal(err)
				}
				want := []rune(question)
				if len(want) > 80 {
					want = want[:80]
				}
				if reservation.RefID != string(want) || !utf8.ValidString(reservation.RefID) {
					t.Fatalf("budget reference has %d characters; want %d", utf8.RuneCountInString(reservation.RefID), len(want))
				}
				var request struct {
					User string `json:"user"`
				}
				var cx SynthesisContext
				if json.Unmarshal([]byte(task.Request), &request) != nil || json.Unmarshal([]byte(task.Context), &cx) != nil || cx.Question != question || !strings.Contains(request.User, question+"\n\nEvidence:") {
					t.Fatal("full question lost from model request/context")
				}
				before := credit(b, "u_alice")
				ledgerBefore := count("credit_ledger_entries")
				tasksBefore, reservesBefore := count("flow_model_tasks"), count("flow_budget_reservations")
				parentBefore := allocated(req.ParentResID)
				_, again, err := StartSynthesis(gw, b, req)
				if err != nil || again.ID != task.ID || again.ReservationID != task.ReservationID {
					t.Fatalf("same key did not reuse task: %v", err)
				}
				if ledgerBefore != count("credit_ledger_entries") || before != credit(b, "u_alice") || tasksBefore != count("flow_model_tasks") || reservesBefore != count("flow_budget_reservations") || parentBefore != allocated(req.ParentResID) {
					t.Fatal("replay changed ledger or balance")
				}
				// Identical summary prefixes must not collapse distinct idempotency keys.
				other := req
				other.Key += ":other"
				other.Question += "different tail"
				_, distinct, err := StartSynthesis(gw, b, other)
				if err != nil || distinct.ID == task.ID || distinct.ReservationID == task.ReservationID {
					t.Fatalf("distinct key was not independent: %v", err)
				}
				tasksBefore, reservesBefore = count("flow_model_tasks"), count("flow_budget_reservations")
				parentBefore = allocated(req.ParentResID)
				balance := credit(b, "u_alice")
				bad := req
				bad.Key += ":over-budget"
				bad.BudgetMicro = 10_000_000
				if _, _, err := StartSynthesis(gw, b, bad); err == nil {
					t.Fatal("insufficient budget accepted")
				}
				if tasksBefore != count("flow_model_tasks") || reservesBefore != count("flow_budget_reservations") || balance != credit(b, "u_alice") || parentBefore != allocated(req.ParentResID) {
					t.Fatal("failed enqueue left task, reservation or balance change")
				}
			})
		}
	}
	if gw.calls != 0 {
		t.Fatal("enqueue called model gateway")
	}
}
