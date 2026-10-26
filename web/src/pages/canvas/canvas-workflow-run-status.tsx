import { useEffect, useState } from "react";
import { AlertTriangle, CheckCircle2, CircleSlash, LoaderCircle, Play, RefreshCw, ShieldQuestion, XCircle } from "lucide-react";
import { Button, InputNumber, Modal, Switch, Tooltip } from "antd";

import { useCanvasWorkflowRun } from "./use-canvas-workflow-run";

const WORKFLOW_PHASE_LABELS: Record<string, string> = {
    requesting: "正在申请审批",
    waiting_approval: "等待审批",
    starting: "正在启动",
    running: "正在运行",
    waiting_external: "结果对账中",
    succeeded: "运行完成",
    failed: "运行失败",
    cancelled: "已取消",
    conflict: "版本已过期",
};

function useElapsedLabel(startedAt: number | undefined, active: boolean) {
    const [, forceTick] = useState(0);
    useEffect(() => {
        if (!active || !startedAt) return;
        const timer = window.setInterval(() => forceTick((value) => value + 1), 1000);
        return () => window.clearInterval(timer);
    }, [active, startedAt]);
    if (!startedAt) return "";
    const seconds = Math.max(0, Math.round((Date.now() - startedAt) / 1000));
    return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m${seconds % 60}s`;
}

/**
 * 顶栏画布工作流状态：申请具体审批 → 批准/拒绝（真实决定接口）→ 轮询运行状态，
 * 支持取消、仅保存重试与版本冲突标记；未保存画布不能申请。
 */
export function CanvasWorkflowRunStatus({
    workflow,
    revision,
    unsaved,
}: {
    workflow: ReturnType<typeof useCanvasWorkflowRun>;
    revision: number | undefined;
    unsaved?: boolean;
}) {
    const { state, requestApproval, approve, reject, cancel, retrySave, reset } = workflow;
    const [applyOpen, setApplyOpen] = useState(false);
    const [budget, setBudget] = useState<number>(200_000);
    const [saveReport, setSaveReport] = useState(false);
    const elapsed = useElapsedLabel(state.startedAt, state.phase === "running" || state.phase === "starting");

    if (state.phase === "idle") {
        return (
            <Tooltip title={unsaved ? "请先保存画布再申请运行" : revision ? "为当前画布申请一次受审批的工作流运行" : "画布尚未同步到云端"}>
                <Button
                    type="text"
                    size="small"
                    className="inline-flex items-center gap-1"
                    disabled={!revision || unsaved}
                    onClick={() => setApplyOpen(true)}
                    data-canvas-no-zoom
                >
                    <Play className="size-3.5" />
                    运行工作流
                </Button>
            </Tooltip>
        );
    }

    const label = WORKFLOW_PHASE_LABELS[state.phase] ?? state.status ?? "工作流";
    const busy = state.phase === "running" || state.phase === "starting" || state.phase === "requesting";

    return (
        <span className="inline-flex items-center gap-1.5" role="status" aria-label={`画布工作流：${label}`} data-canvas-no-zoom>
            {state.phase === "succeeded" ? <CheckCircle2 className="size-3.5 text-emerald-500" /> : null}
            {state.phase === "failed" ? <XCircle className="size-3.5 text-red-500" /> : null}
            {state.phase === "cancelled" ? <CircleSlash className="size-3.5 opacity-70" /> : null}
            {state.phase === "conflict" ? <AlertTriangle className="size-3.5 text-amber-500" /> : null}
            {state.phase === "waiting_approval" ? <ShieldQuestion className="size-3.5 text-sky-500" /> : null}
            {busy ? <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" /> : null}
            <span className="text-xs">
                {label}
                {elapsed ? ` · ${elapsed}` : ""}
            </span>
            {state.phase === "waiting_approval" ? (
                <>
                    <Button size="small" type="primary" onClick={() => void approve()}>
                        批准运行
                    </Button>
                    <Button size="small" onClick={() => void reject()}>
                        拒绝
                    </Button>
                </>
            ) : null}
            {state.phase === "running" ? (
                <Button size="small" onClick={() => void cancel()}>
                    取消
                </Button>
            ) : null}
            {state.saveRetryable && (state.phase === "failed" || state.phase === "succeeded") ? (
                <Button size="small" icon={<RefreshCw className="size-3" />} onClick={() => void retrySave()}>
                    仅重试保存
                </Button>
            ) : null}
            {state.phase === "conflict" || state.phase === "succeeded" || state.phase === "failed" || state.phase === "cancelled" ? (
                <Button size="small" type="text" onClick={reset}>
                    清除
                </Button>
            ) : null}
            {state.error ? <Tooltip title={state.error}><span className="max-w-40 truncate text-xs opacity-70">{state.error}</span></Tooltip> : null}
            <Modal
                title="申请运行画布工作流"
                open={applyOpen}
                okText="提交审批申请"
                cancelText="取消"
                okButtonProps={{ disabled: !budget || budget <= 0 }}
                onOk={() => {
                    setApplyOpen(false);
                    void requestApproval({ expected_revision: revision ?? 0, ops: [], run: { budget_micro: budget, save_report: saveReport } });
                }}
                onCancel={() => setApplyOpen(false)}
            >
                <p className="mb-3 text-xs opacity-70">
                    将以当前云端版本 revision {revision ?? 0} 冻结本次运行的图与预算，提交后需在上方批准才会执行；修改画布后需重新申请。
                </p>
                <label className="mb-3 flex items-center gap-2 text-sm">
                    预算上限（micro 积分）
                    <InputNumber min={1} max={1_000_000_000} value={budget} onChange={(value) => setBudget(Number(value) || 0)} className="w-40" />
                </label>
                <label className="flex items-center gap-2 text-sm">
                    保存为正式报告
                    <Switch size="small" checked={saveReport} onChange={setSaveReport} />
                </label>
            </Modal>
        </span>
    );
}
