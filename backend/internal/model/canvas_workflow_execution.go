package model

import "time"

// CanvasWorkflowExecution is an admission receipt and bounded checkpoint, not a
// queue or engine. Agent recovery must never deserialize this canvas state.
type CanvasWorkflowExecution struct {
	ID                   string `gorm:"primaryKey;size:64"`
	TenantID             string `gorm:"size:40;not null;uniqueIndex:ux_canvas_workflow_key,priority:1;index"`
	WorkspaceID          string `gorm:"size:40;not null;uniqueIndex:ux_canvas_workflow_key,priority:2"`
	CanvasID             string `gorm:"size:80;not null;uniqueIndex:ux_canvas_workflow_key,priority:3"`
	ExecutionKey         string `gorm:"size:160;not null;uniqueIndex:ux_canvas_workflow_key,priority:4"`
	UserID               string `gorm:"size:36;not null;index"`
	RequestHash          string `gorm:"size:64;not null"`
	RequestJSON          string `gorm:"type:text;not null" json:"-"`
	CanvasRevision       int64
	CanvasJSON           string `gorm:"type:text;not null" json:"-"`
	BudgetMicro          int64
	ReservationID        string `gorm:"size:40;not null"`
	CapabilitySnapshotID string `gorm:"size:64;not null"`
	Status               string `gorm:"size:32;not null"`
	Revision             int64  `gorm:"not null"`
	CheckpointJSON       string `gorm:"type:text;not null"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

func (CanvasWorkflowExecution) TableName() string { return "flow_canvas_workflow_executions" }

// CanvasMonitorRunUsage is T22's single atomic counter per recurring monitor
// plan: run count and micro budget are consumed by one conditional update in
// the same transaction as admission, so concurrent workers cannot overspend.
type CanvasMonitorRunUsage struct {
	ID          string `gorm:"primaryKey;size:64"`
	TenantID    string `gorm:"size:40;not null;uniqueIndex:ux_canvas_monitor_usage,priority:1"`
	MonitorID   string `gorm:"size:64;not null;uniqueIndex:ux_canvas_monitor_usage,priority:2"`
	WorkspaceID string `gorm:"size:40;not null"`
	CanvasID    string `gorm:"size:80;not null"`
	RunsUsed    int64  `gorm:"not null;default:0"`
	MicroUsed   int64  `gorm:"not null;default:0"`
	// LastCycleKey is the cycle (ExecutionKey suffix) whose consumption paid
	// for the most recent admission; the authority rejects admissions whose
	// execution key is not the just-consumed cycle.
	LastCycleKey string `gorm:"size:160;not null;default:''"`
	ExhaustedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (CanvasMonitorRunUsage) TableName() string { return "flow_canvas_monitor_run_usage" }
