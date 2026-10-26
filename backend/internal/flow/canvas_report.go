package flow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"yingce/backend/internal/model"
)

type CanvasReportSaveResult struct {
	Status   string `json:"status"`
	ReportID string `json:"report_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

// CanvasReportSaver only persists an already completed, structured run output.
// The caller checkpoints that frozen payload before calling Save.
type CanvasReportSaver struct{ Tools Tools }

func (s CanvasReportSaver) Save(ctx context.Context, conv model.FlowAgentConversation, ss SubjectScope, userID, runID, nodeID string, payload map[string]any) (CanvasReportSaveResult, error) {
	fail := func(err error) (CanvasReportSaveResult, error) {
		return CanvasReportSaveResult{Status: "partial", Error: err.Error()}, err
	}
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(nodeID) == "" {
		return fail(fmt.Errorf("%w: run and report node required", ErrToolArgs))
	}
	// Serialize once to normalize JSON arrays/numbers without mutating the caller's
	// frozen checkpoint. No source extraction, model invocation or topic fallback.
	raw, err := json.Marshal(payload)
	if err != nil {
		return fail(err)
	}
	var args map[string]any
	if err = json.Unmarshal(raw, &args); err != nil {
		return fail(err)
	}
	key := sha256.Sum256([]byte(runID + "\x00" + nodeID))
	tools := s.Tools
	tools.RunID = runID
	result, err := tools.Invoke(ctx, conv, ss, userID, "report:"+hex.EncodeToString(key[:]), "intel_save_report", args, "")
	if err != nil {
		return fail(err)
	}
	if result.Status != "succeeded" {
		return fail(fmt.Errorf("report save %s: %s", result.Status, result.Error))
	}
	var saved struct {
		ReportID string `json:"report_id"`
	}
	if json.Unmarshal(result.Output, &saved) != nil || !strings.HasPrefix(saved.ReportID, "rpt_") {
		return fail(fmt.Errorf("report save missing flow report_id"))
	}
	return CanvasReportSaveResult{Status: "succeeded", ReportID: saved.ReportID}, nil
}
