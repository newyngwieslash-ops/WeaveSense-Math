package flow

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"yingce/backend/internal/model"
)

func t22Event(eventID, scheduledAt string, complete bool) Event {
	payload := map[string]any{
		"monitor_id": "mon22", "canvas_id": "cv22", "cycle_complete": complete, "operation_id": "op22",
		"canvas_auto_run_grant": map[string]any{"purpose": "monitor_canvas_recurring"},
		"window":                map[string]any{"scheduled_at": scheduledAt, "from": "2026-10-01T00:00:00Z", "to": "2026-10-02T00:00:00Z", "timezone": "UTC"},
	}
	raw, _ := json.Marshal(payload)
	return Event{EventID: eventID, OperationID: "op22", TenantID: "tn_t22", WorkspaceID: "ws_t22", Type: "collection.completed", Payload: raw}
}

func TestT22MonitorCanvasIntakeDedupAndCycleIdempotency(t *testing.T) {
	db := testDB(t)
	if err := db.AutoMigrate(&model.FlowCommandOutbox{}, &model.FlowEventInbox{}); err != nil {
		t.Fatal(err)
	}
	first, err := ConsumeMonitorCanvasEvent(db, t22Event("evt-1", "2026-10-02T00:00:00Z", true))
	if err != nil || !first {
		t.Fatalf("first intake: %v %v", first, err)
	}
	// 重复投递同一事件：inbox 去重，不再产生命令。
	if again, err := ConsumeMonitorCanvasEvent(db, t22Event("evt-1", "2026-10-02T00:00:00Z", true)); err != nil || again {
		t.Fatalf("duplicate delivery re-enqueued: %v %v", again, err)
	}
	// 同一周期不同事件（重复完成信号）：命令幂等键一致，不产生第二条。
	if reenqueued, err := ConsumeMonitorCanvasEvent(db, t22Event("evt-1b", "2026-10-02T00:00:00Z", true)); err != nil || reenqueued {
		t.Fatalf("same cycle second command: %v %v", reenqueued, err)
	}
	// 下一周期：新命令。
	next, err := ConsumeMonitorCanvasEvent(db, t22Event("evt-2", "2026-10-03T00:00:00Z", true))
	if err != nil || !next {
		t.Fatalf("next cycle not enqueued: %v %v", next, err)
	}
	var count int64
	db.Model(&model.FlowCommandOutbox{}).Where("command = ?", MonitorCanvasRunCommand).Count(&count)
	if count != 2 {
		t.Fatalf("command count %d", count)
	}
	var cmd model.FlowCommandOutbox
	if err := db.First(&cmd, "command = ?", MonitorCanvasRunCommand).Error; err != nil {
		t.Fatal(err)
	}
	if cmd.Status != "pending" || cmd.TenantID != "tn_t22" || cmd.IdempotencyKey == "" || cmd.RequestHash == "" {
		t.Fatalf("command row incomplete: %+v", cmd)
	}
	// 未完成周期 / 无画布 / 无授权的事件不产生命令。
	for name, evt := range map[string]Event{
		"incomplete":   t22Event("evt-3", "2026-10-04T00:00:00Z", false),
		"other-type":   {EventID: "evt-4", Type: "document.revised", TenantID: "tn_t22", OperationID: "op22", Payload: json.RawMessage(`{}`)},
		"no-grant":     {EventID: "evt-5", TenantID: "tn_t22", OperationID: "op22", Type: "collection.completed", Payload: json.RawMessage(`{"monitor_id":"m","canvas_id":"c","cycle_complete":true,"operation_id":"o"}`)},
		"bad-schedule": {EventID: "evt-6", TenantID: "tn_t22", OperationID: "op22", Type: "collection.completed", Payload: json.RawMessage(`{"monitor_id":"m","canvas_id":"c","cycle_complete":true,"operation_id":"o","canvas_auto_run_grant":{},"window":{"scheduled_at":"not-a-time"}}`)},
	} {
		if ok, err := ConsumeMonitorCanvasEvent(db, evt); name == "bad-schedule" && err == nil {
			t.Fatalf("%s accepted invalid schedule", name)
		} else if name != "bad-schedule" && (err != nil || ok) {
			t.Fatalf("%s not ignored: %v %v", name, ok, err)
		}
	}
}

func TestT22MonitorCanvasBudgetAtomicConsume(t *testing.T) {
	db := testDB(t)
	if err := db.AutoMigrate(&model.CanvasMonitorRunUsage{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := ConsumeMonitorCanvasBudget(db, "tn", "ws", "mon", "cv", "cycle", 30, 3, 100, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := ConsumeMonitorCanvasBudget(db, "tn", "ws", "mon", "cv", "cycle", 30, 3, 100, now); err == nil {
		t.Fatal("max_runs overrun accepted")
	}
	var usage model.CanvasMonitorRunUsage
	if err := db.First(&usage, "tenant_id = ? AND monitor_id = ?", "tn", "mon").Error; err != nil {
		t.Fatal(err)
	}
	if usage.RunsUsed != 3 || usage.MicroUsed != 90 {
		t.Fatalf("usage drifted: %+v", usage)
	}
	// 预算上限先于次数触发：每窗 40、总 100 只允许 2 次。
	for i := 0; i < 2; i++ {
		if err := ConsumeMonitorCanvasBudget(db, "tn", "ws", "mon2", "cv", "cycle", 40, 10, 100, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := ConsumeMonitorCanvasBudget(db, "tn", "ws", "mon2", "cv", "cycle", 40, 10, 100, now); err == nil {
		t.Fatal("total budget overrun accepted")
	}
}

func TestT22LocalCommandDispatchBranch(t *testing.T) {
	db := testDB(t)
	if err := db.AutoMigrate(&model.FlowCommandOutbox{}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	dispatcher := CommandDispatcher{DB: db, Local: map[string]func(context.Context, model.FlowCommandOutbox) (int, []byte, error){
		MonitorCanvasRunCommand: func(_ context.Context, row model.FlowCommandOutbox) (int, []byte, error) {
			calls++
			return 200, []byte(`{"code":0,"data":{"operation_id":"cw_x","run_id":"cw_x"}}`), nil
		},
	}}
	cmd := model.FlowCommandOutbox{ID: "cmd_t22", OperationID: "op", TenantID: "tn", WorkspaceID: "ws", ActorType: "user", ActorID: "u", Command: MonitorCanvasRunCommand, Method: "POST", Path: "/local/canvas/run", IdempotencyKey: "k", RequestHash: "h", Payload: "{}", Status: "pending", MaxAttempts: 5, NextAttemptAt: time.Now().UTC().Add(-time.Minute)}
	if err := db.Create(&cmd).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := dispatcher.DispatchOne(context.Background(), cmd.ID); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("local handler calls %d", calls)
	}
	if err := db.First(&cmd, "id = ?", cmd.ID).Error; err != nil {
		t.Fatal(err)
	}
	if cmd.Status != "dispatched" || cmd.EngineOperationID != "cw_x" {
		t.Fatalf("local command not settled: %+v", cmd)
	}
	// 重放同一命令不再执行本地处理器。
	if _, err := dispatcher.DispatchOne(context.Background(), cmd.ID); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("dispatched command re-executed: %d", calls)
	}
	// 并发领取只执行一次。
	var wg sync.WaitGroup
	cmd2 := cmd
	cmd2.ID, cmd2.IdempotencyKey, cmd2.Status = "cmd_t22b", "k2", "pending"
	if err := db.Create(&cmd2).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = dispatcher.DispatchOne(context.Background(), cmd2.ID) }()
	}
	wg.Wait()
	if calls != 2 {
		t.Fatalf("concurrent claims executed handler %d times", calls-1)
	}
}
