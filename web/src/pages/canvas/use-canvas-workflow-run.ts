import { useCallback, useEffect, useRef, useState } from "react";

import { decideAgentApproval } from "@/services/api/agent";
import {
    cancelCanvasWorkflowRun,
    getCanvasWorkflowRun,
    requestCanvasWorkflowApproval,
    retryCanvasWorkflowSave,
    runApprovedCanvasWorkflow,
    type CanvasWorkflowApprovalInput,
    type CanvasWorkflowRunSnapshot,
} from "@/services/api/canvas-workflow";
import { ApiError } from "@/services/api/request";

export type CanvasWorkflowPhase =
    | "idle"
    | "requesting"
    | "waiting_approval"
    | "starting"
    | "running"
    | "waiting_external"
    | "succeeded"
    | "failed"
    | "cancelled"
    | "conflict";

export type CanvasWorkflowRunState = {
    phase: CanvasWorkflowPhase;
    agentRunId?: string;
    approvalId?: string;
    runId?: string;
    status?: string;
    revision?: number;
    reportId?: string;
    error?: string;
    startedAt?: number;
    finishedAt?: number;
    saveRetryable?: boolean;
    snapshot?: CanvasWorkflowRunSnapshot;
};

export const CANVAS_WORKFLOW_TERMINAL_STATUSES = new Set(["succeeded", "failed", "cancelled"]);

/** 服务端运行状态到前端阶段的映射；null 表示仍在运行（继续轮询）。 */
export function workflowPhaseFromStatus(status: string | undefined): CanvasWorkflowPhase | null {
    switch (status) {
        case "succeeded":
            return "succeeded";
        case "failed":
            return "failed";
        case "cancelled":
            return "cancelled";
        case "waiting_external":
            return "waiting_external";
        default:
            return null;
    }
}

/** 终态或保存中断（saving_result/saving_report）时允许仅重试保存。 */
export function workflowSaveRetryable(snapshot: CanvasWorkflowRunSnapshot): boolean {
    const stage = snapshot.checkpoint?.stage;
    if (stage === "saving_result" || stage === "saving_report") return true;
    return snapshot.status === "succeeded" && !!snapshot.report_id === false && snapshot.result?.status === "partial";
}

/** 错误分类：revision 冲突需要提示重新申请，其余按失败展示真实 reason。 */
export function classifyWorkflowError(err: unknown): { phase: CanvasWorkflowPhase; message: string } {
    if (err instanceof ApiError) {
        if (err.reason === "canvas_revision_conflict") {
            return { phase: "conflict", message: "画布已修改，审批对应的版本已过期；请刷新后重新申请运行。" };
        }
        if (err.reason === "canvas_approval_required") {
            return { phase: "waiting_approval", message: "该运行需要先完成审批。" };
        }
        return { phase: "failed", message: err.message || "画布工作流请求失败" };
    }
    return { phase: "failed", message: err instanceof Error ? err.message : "画布工作流请求失败" };
}

const POLL_INTERVAL_MS = 2000;

/**
 * 画布工作流运行状态机：申请审批 → 批准（真实决定接口）→ 触发/恢复执行 →
 * GET 轮询直到终态；取消与仅保存重试都走真实接口，卸载时中止轮询。
 */
export function useCanvasWorkflowRun(canvasId: string | undefined, onFinished?: (snapshot: CanvasWorkflowRunSnapshot) => void) {
    const [state, setState] = useState<CanvasWorkflowRunState>({ phase: "idle" });
    const abortRef = useRef<AbortController | null>(null);
    const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const finishedRef = useRef(false);

    const stopPolling = useCallback(() => {
        if (pollTimerRef.current) {
            clearTimeout(pollTimerRef.current);
            pollTimerRef.current = null;
        }
        abortRef.current?.abort();
        abortRef.current = null;
    }, []);

    useEffect(() => stopPolling, [stopPolling]);

    const pollRun = useCallback(
        (runId: string) => {
            const tick = async () => {
                const controller = new AbortController();
                abortRef.current = controller;
                try {
                    const snapshot = await getCanvasWorkflowRun(canvasId!, runId, controller.signal);
                    const phase = workflowPhaseFromStatus(snapshot.status);
                    if (phase === null) {
                        setState((prev) => ({ ...prev, phase: "running", status: snapshot.status, revision: snapshot.revision, runId }));
                        pollTimerRef.current = setTimeout(tick, POLL_INTERVAL_MS);
                        return;
                    }
                    setState((prev) => ({
                        ...prev,
                        phase,
                        status: snapshot.status,
                        revision: snapshot.revision,
                        reportId: snapshot.report_id || undefined,
                        error: snapshot.error || snapshot.result?.error || undefined,
                        saveRetryable: workflowSaveRetryable(snapshot),
                        snapshot,
                        finishedAt: Date.now(),
                    }));
                    if (!finishedRef.current) {
                        finishedRef.current = true;
                        onFinished?.(snapshot);
                    }
                } catch (err) {
                    if (controller.signal.aborted) return;
                    const classified = classifyWorkflowError(err);
                    setState((prev) => ({ ...prev, phase: classified.phase, error: classified.message, finishedAt: Date.now() }));
                }
            };
            tick();
        },
        [canvasId, onFinished],
    );

    /** 申请具体审批：只创建控制面请求；等待用户在审批卡片上决定。 */
    const requestApproval = useCallback(
        async (input: CanvasWorkflowApprovalInput) => {
            if (!canvasId) return;
            stopPolling();
            finishedRef.current = false;
            setState({ phase: "requesting" });
            try {
                const approval = await requestCanvasWorkflowApproval(canvasId, input);
                setState({
                    phase: "waiting_approval",
                    agentRunId: approval.agent_run_id,
                    approvalId: approval.approval_id,
                    startedAt: Date.now(),
                });
            } catch (err) {
                const classified = classifyWorkflowError(err);
                setState({ phase: classified.phase, error: classified.message });
            }
        },
        [canvasId, stopPolling],
    );

    /** 批准并进入执行：决定接口成功后服务端已开始执行；run 入口幂等返回持久 run。 */
    const approve = useCallback(
        async (reason?: string) => {
            const agentRunId = state.agentRunId;
            const approvalId = state.approvalId;
            if (!canvasId || !agentRunId || !approvalId) return;
            setState((prev) => ({ ...prev, phase: "starting" }));
            try {
                await decideAgentApproval(agentRunId, approvalId, "approve", reason);
                const accepted = await runApprovedCanvasWorkflow(canvasId, agentRunId, approvalId);
                setState((prev) => ({ ...prev, phase: "running", runId: accepted.run_id, status: accepted.status, revision: accepted.revision }));
                pollRun(accepted.run_id);
            } catch (err) {
                const classified = classifyWorkflowError(err);
                setState((prev) => ({ ...prev, phase: classified.phase, error: classified.message }));
            }
        },
        [canvasId, pollRun, state.agentRunId, state.approvalId],
    );

    const reject = useCallback(
        async (reason?: string) => {
            const agentRunId = state.agentRunId;
            const approvalId = state.approvalId;
            if (!agentRunId || !approvalId) return;
            try {
                await decideAgentApproval(agentRunId, approvalId, "reject", reason);
                setState((prev) => ({ ...prev, phase: "cancelled", error: "已拒绝本次工作流运行；未建图、未执行、未产生扣费。", finishedAt: Date.now() }));
            } catch (err) {
                const classified = classifyWorkflowError(err);
                setState((prev) => ({ ...prev, phase: classified.phase, error: classified.message }));
            }
        },
        [state.agentRunId, state.approvalId],
    );

    const cancel = useCallback(async () => {
        const runId = state.runId;
        if (!canvasId || !runId) return;
        try {
            await cancelCanvasWorkflowRun(canvasId, runId);
            pollRun(runId);
        } catch (err) {
            const classified = classifyWorkflowError(err);
            setState((prev) => ({ ...prev, phase: classified.phase, error: classified.message }));
        }
    }, [canvasId, pollRun, state.runId]);

    /** 仅保存重试：服务端不会重跑模型或重新计费。 */
    const retrySave = useCallback(async () => {
        const runId = state.runId;
        if (!canvasId || !runId) return;
        try {
            await retryCanvasWorkflowSave(canvasId, runId);
            setState((prev) => ({ ...prev, phase: "running" }));
            pollRun(runId);
        } catch (err) {
            const classified = classifyWorkflowError(err);
            setState((prev) => ({ ...prev, phase: classified.phase, error: classified.message }));
        }
    }, [canvasId, pollRun, state.runId]);

    const reset = useCallback(() => {
        stopPolling();
        finishedRef.current = false;
        setState({ phase: "idle" });
    }, [stopPolling]);

    return { state, requestApproval, approve, reject, cancel, retrySave, reset };
}
