import { describe, expect, it } from "bun:test";

import { ApiError } from "@/services/api/request";
import { classifyWorkflowError, workflowPhaseFromStatus, workflowSaveRetryable } from "@/pages/canvas/use-canvas-workflow-run";
import type { CanvasWorkflowRunSnapshot } from "@/services/api/canvas-workflow";

describe("T18 workflow run state machine", () => {
    it("maps server statuses to terminal phases and keeps transient stages polling", () => {
        expect(workflowPhaseFromStatus("succeeded")).toBe("succeeded");
        expect(workflowPhaseFromStatus("failed")).toBe("failed");
        expect(workflowPhaseFromStatus("cancelled")).toBe("cancelled");
        expect(workflowPhaseFromStatus("waiting_external")).toBe("waiting_external");
        expect(workflowPhaseFromStatus("running")).toBeNull();
        expect(workflowPhaseFromStatus("admitted")).toBeNull();
        expect(workflowPhaseFromStatus("saving_result")).toBeNull();
        expect(workflowPhaseFromStatus("saving_report")).toBeNull();
        expect(workflowPhaseFromStatus(undefined)).toBeNull();
    });

    it("allows save-only retry only for interrupted saves or partial results", () => {
        const snapshot = (patch: Partial<CanvasWorkflowRunSnapshot>): CanvasWorkflowRunSnapshot =>
            ({ run_id: "cw_1", canvas_id: "c1", status: "succeeded", revision: 2, ...patch }) as CanvasWorkflowRunSnapshot;
        expect(workflowSaveRetryable(snapshot({ status: "saving_report", checkpoint: { stage: "saving_report" } }))).toBe(true);
        expect(workflowSaveRetryable(snapshot({ status: "failed", checkpoint: { stage: "saving_result" } }))).toBe(true);
        expect(workflowSaveRetryable(snapshot({ result: { status: "partial" } }))).toBe(true);
        expect(workflowSaveRetryable(snapshot({ report_id: "rpt_1", result: { status: "succeeded" } }))).toBe(false);
        expect(workflowSaveRetryable(snapshot({ status: "failed", checkpoint: { stage: "failed" }, result: null }))).toBe(false);
    });

    it("classifies revision conflicts and approval gating instead of generic failure", () => {
        expect(classifyWorkflowError(new ApiError("冲突", { status: 409, reason: "canvas_revision_conflict" })).phase).toBe("conflict");
        expect(classifyWorkflowError(new ApiError("需审批", { status: 409, reason: "canvas_approval_required" })).phase).toBe("waiting_approval");
        expect(classifyWorkflowError(new ApiError("失败", { status: 500, reason: "canvas_workflow_failed" })).phase).toBe("failed");
        expect(classifyWorkflowError(new Error("网络中断")).phase).toBe("failed");
    });
});
