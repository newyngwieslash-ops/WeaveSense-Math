package flow

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"yingce/backend/internal/model"
)

// MonitorCanvasRunCommand is the platform-owned outbox command that triggers a
// recurring canvas workflow run. It executes locally (managed runner) and is
// never forwarded to the engine.
const MonitorCanvasRunCommand = "canvas.run"

// MonitorCanvasEventPayload is the recurring-canvas slice of Loom's
// collection.completed event payload.
type MonitorCanvasEventPayload struct {
	MonitorID     string          `json:"monitor_id"`
	MonitorVersio int64           `json:"monitor_version"`
	CanvasID      string          `json:"canvas_id"`
	Grant         json.RawMessage `json:"canvas_auto_run_grant"`
	CycleComplete bool            `json:"cycle_complete"`
	Window        struct {
		ScheduledAt string `json:"scheduled_at"`
		From        string `json:"from"`
		To          string `json:"to"`
		Timezone    string `json:"timezone"`
	} `json:"window"`
	OperationID string `json:"operation_id"`
}

// ConsumeMonitorCanvasEvent is the T22 intake: an engine cycle completion with
// a bound canvas becomes at most one canvas.run command. The inbox row and the
// command commit in one transaction; the command idempotency key is the
// (tenant, monitor, scheduled cycle) identity, so duplicate deliveries and
// concurrent intakes cannot enqueue a cycle twice.
func ConsumeMonitorCanvasEvent(db *gorm.DB, e Event) (enqueued bool, err error) {
	if db == nil {
		return false, errors.New("flow: canvas auto run db not configured")
	}
	if e.Type != "collection.completed" || e.TenantID == "" || e.EventID == "" {
		return false, nil
	}
	var p MonitorCanvasEventPayload
	if json.Unmarshal(e.Payload, &p) != nil {
		return false, nil
	}
	if !p.CycleComplete || p.MonitorID == "" || p.CanvasID == "" || len(p.Grant) == 0 || p.OperationID == "" {
		return false, nil
	}
	scheduled := time.Now().UTC().Format(time.RFC3339Nano)
	if t, e := time.Parse(time.RFC3339, p.Window.ScheduledAt); e == nil {
		scheduled = t.UTC().Format(time.RFC3339Nano)
	} else if p.Window.ScheduledAt != "" {
		return false, errors.New("flow: monitor canvas cycle window invalid")
	}
	payload, err := json.Marshal(map[string]any{
		"canvas_id": p.CanvasID, "monitor_id": p.MonitorID, "monitor_version": p.MonitorVersio,
		"grant": p.Grant, "scheduled_at": scheduled, "window": p.Window, "operation_id": p.OperationID,
	})
	if err != nil {
		return false, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		row := model.FlowEventInbox{ConsumerID: "canvas-auto-run", EventID: e.EventID, TenantID: e.TenantID,
			OperationID: e.OperationID, Type: e.Type, Payload: string(e.Payload), ReceivedAt: time.Now().UTC()}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil
		}
		now := time.Now().UTC()
		cmd := model.FlowCommandOutbox{
			ID: NewID("cmd_"), OperationID: NewID("op_"), TenantID: e.TenantID, WorkspaceID: e.WorkspaceID,
			ActorType: "user", ActorID: "system", ActorRole: "admin",
			Command: MonitorCanvasRunCommand, Method: "POST", Path: "/local/canvas/run",
			IdempotencyKey: "canvas-run:" + hashJSON([]string{e.TenantID, p.MonitorID, scheduled}),
			RequestHash:    commandRequestHash(MonitorCanvasRunCommand, e.WorkspaceID, "POST", "/local/canvas/run", payload),
			Payload:        string(payload), Status: "pending", MaxAttempts: 5, NextAttemptAt: now,
		}
		if cmd.WorkspaceID == "" {
			cmd.WorkspaceID = e.TenantID
		}
		create := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "idempotency_key"}}, DoNothing: true}).Create(&cmd)
		if create.Error != nil {
			return create.Error
		}
		enqueued = create.RowsAffected == 1
		return nil
	})
	return enqueued, err
}

// ConsumeMonitorCanvasBudget is the single atomic T22 consumption: it reserves
// one run and its micro budget under the plan's MaxRuns/TotalBudgetMicro caps.
// It rolls back with the caller's transaction (pass tx), so admission and
// consumption commit or fail together.
func ConsumeMonitorCanvasBudget(tx *gorm.DB, tenantID, workspaceID, monitorID, canvasID, cycleKey string, runBudget, maxRuns, totalBudget int64, now time.Time) error {
	row := model.CanvasMonitorRunUsage{ID: NewID("cmu_"), TenantID: tenantID, MonitorID: monitorID, WorkspaceID: workspaceID, CanvasID: canvasID, CreatedAt: now, UpdatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return err
	}
	res := tx.Model(&model.CanvasMonitorRunUsage{}).
		Where("tenant_id = ? AND monitor_id = ? AND runs_used < ? AND micro_used + ? <= ?", tenantID, monitorID, maxRuns, runBudget, totalBudget).
		Updates(map[string]any{"runs_used": gorm.Expr("runs_used + 1"), "micro_used": gorm.Expr("micro_used + ?", runBudget), "last_cycle_key": cycleKey, "updated_at": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrBudgetInsufficient
	}
	return nil
}
