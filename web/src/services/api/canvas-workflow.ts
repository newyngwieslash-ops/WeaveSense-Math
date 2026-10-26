import { http } from "./request";

/** 画布工作流单次审批请求：ops 为空表示按当前图运行；预算与参数共同冻结进审批。 */
export type CanvasWorkflowApprovalInput = {
    expected_revision: number;
    ops: unknown[];
    run: {
        budget_micro: number;
        refresh_bindings?: boolean;
        save_report?: boolean;
        report_title?: string;
    };
};

export type CanvasWorkflowApprovalResponse = {
    agent_run_id: string;
    approval_id: string;
    status: string;
    preview?: { title?: string; description?: string; items?: { operation?: string; summary?: string }[] };
    scope?: { canvasId?: string; budgetMicro?: number };
};

export type CanvasWorkflowRunSnapshot = {
    run_id: string;
    canvas_id: string;
    status: string;
    revision: number;
    checkpoint?: { stage?: string; error?: string };
    result?: {
        status?: string;
        revision?: number;
        error?: string;
        written_payload_hash?: string;
        report_id?: string;
        report_node_id?: string;
        report_payload?: { title?: string; body?: string; claims?: unknown[]; material_scope?: { document_ids?: string[]; source_ids?: string[] }; structured?: Record<string, unknown> };
    } | null;
    report_id?: string;
    error?: string;
};

export type CanvasWorkflowRunAccepted = { run_id: string; status: string; revision: number };

/** 申请具体审批：仅创建控制面请求，不建图、不执行、不产生模型调用。 */
export function requestCanvasWorkflowApproval(canvasId: string, input: CanvasWorkflowApprovalInput, signal?: AbortSignal) {
    return http.post<CanvasWorkflowApprovalResponse>(`/canvas-workflows/${encodeURIComponent(canvasId)}/approvals`, input, { signal });
}

/** 批准后触发/恢复执行；同一审批重复调用返回同一持久 run。 */
export function runApprovedCanvasWorkflow(canvasId: string, agentRunId: string, approvalId: string, signal?: AbortSignal) {
    return http.post<CanvasWorkflowRunAccepted>(
        `/canvas-workflows/${encodeURIComponent(canvasId)}/run`,
        { agent_run_id: agentRunId, approval_id: approvalId },
        { signal },
    );
}

/** 查询运行状态；轮询需传入 AbortSignal 以便页面卸载时停止。 */
export function getCanvasWorkflowRun(canvasId: string, runId: string, signal?: AbortSignal) {
    return http.get<CanvasWorkflowRunSnapshot>(`/canvas-workflows/${encodeURIComponent(canvasId)}/runs/${encodeURIComponent(runId)}`, { signal });
}

/** 仅保存重试（saving_result/saving_report 阶段），不重跑模型。 */
export function retryCanvasWorkflowSave(canvasId: string, runId: string, signal?: AbortSignal) {
    return http.post<{ run_id: string; accepted: boolean }>(`/canvas-workflows/${encodeURIComponent(canvasId)}/runs/${encodeURIComponent(runId)}/retry-save`, {}, { signal });
}

/** 取消运行；未知模型结果仍由服务端对账，前端不得重复触发。 */
export function cancelCanvasWorkflowRun(canvasId: string, runId: string, signal?: AbortSignal) {
    return http.post<{ run_id: string; accepted: boolean }>(`/canvas-workflows/${encodeURIComponent(canvasId)}/runs/${encodeURIComponent(runId)}/cancel`, {}, { signal });
}
