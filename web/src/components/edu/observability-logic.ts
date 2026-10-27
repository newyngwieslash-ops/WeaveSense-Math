/** DEV-121 运行观测：与服务端 observability/1 口径一致。只读元数据，不含输入、输出与作答。 */
export type AlertState = "firing" | "ok" | "insufficient_data";
export type ObsAlert = { id: string; state: AlertState; value: number; threshold: number; rule: string };
export type ObsKind = {
    kind: string;
    created: number;
    succeeded: number;
    failed: number;
    cancelled: number;
    open: number;
    reconciliationRequired: number;
    failureRate: number | null;
    p50Seconds: number | null;
    p95Seconds: number | null;
};
export type Observability = {
    definition: string;
    windowHours: number;
    generation: { byKind: ObsKind[]; errorCodes: { errorCode: string; count: number }[]; finished: number; failed: number };
    submission: { attempts: number; selfTests: number; helpRequests: number };
    reading: { collected: boolean };
    queue: { queued: number; running: number; oldestQueuedSeconds: number | null; stuck: { id: string; kind: string; status: string; stage: string; silentSeconds: number }[]; stuckTotal: number };
    cost: { reconciliationRequired: number; heldUnknown: number; unsettledReservations: number; openLedgerAnomalies: number };
    availability: { target: number; measured: number | null; reason: string };
    alerts: ObsAlert[];
    firing: number;
    notCollected: { dimension: string; reason: string }[];
};
export type TraceStep = { at: string; source: "platform" | "service" | "stage" | "ledger"; type: string; status?: string; detail?: Record<string, unknown> };
export type JobTrace = {
    traceId: string;
    job: { id: string; kind: string; status: string; stage: string; errorCode?: string; settlement: string };
    revisions: Record<string, string>;
    usage: { costMicro: number; computeMs: number; stages: number };
    steps: TraceStep[];
};

export const ALERT_LABEL: Record<string, string> = {
    stuck_jobs: "卡住的任务（超过 30 分钟无进展）",
    queue_age: "排队最久的任务超过 15 分钟",
    reconciliation_required: "结果未知、待对账的任务",
    unknown_cost: "供应商费用未知而冻结的预留",
    unsettled_reservations: "已结束但预留未结算（资源泄露）",
    ledger_anomalies: "未解决的账本异常",
    failure_rate: "失败率超过 20%（至少 5 个已结束任务）",
};
export const ALERT_STATE_LABEL: Record<AlertState, string> = { firing: "告警", ok: "正常", insufficient_data: "数据不足" };
export const SOURCE_LABEL: Record<TraceStep["source"], string> = { platform: "平台", service: "数学服务", stage: "阶段结算", ledger: "账本" };

const NOT_COLLECTED_LABEL: Record<string, string> = {
    availability: "可用性（99.5% 试点目标需外部探针）",
    reading: "阅读量（未采集浏览事件）",
    request_trace: "请求级 trace 编号（以任务编号串联）",
    resources: "主机资源",
    alert_delivery: "告警通知投递",
};

export function alertValueText(a: ObsAlert): string {
    if (a.id === "failure_rate") return `${(a.value * 100).toFixed(1)}%`;
    if (a.id === "queue_age") return durationText(a.value);
    return String(a.value);
}

export function durationText(seconds: number | null | undefined): string {
    if (seconds == null) return "无";
    if (seconds < 60) return `${Math.round(seconds)} 秒`;
    if (seconds < 3600) return `${Math.round(seconds / 60)} 分钟`;
    return `${(seconds / 3600).toFixed(1)} 小时`;
}

export function rateText(r: number | null): string {
    return r == null ? "不可计算" : `${(r * 100).toFixed(1)}%`;
}

export function availabilityText(o: Observability): string {
    const target = `${(o.availability.target * 100).toFixed(1)}%`;
    return o.availability.measured == null ? `目标 ${target}，未测量（需外部探针）` : `目标 ${target}，实测 ${(o.availability.measured * 100).toFixed(2)}%`;
}

export function notCollectedText(o: Observability): string {
    if (!o.notCollected?.length) return "";
    return `未采集：${o.notCollected.map((n) => NOT_COLLECTED_LABEL[n.dimension] || n.dimension).join("、")}。`;
}

/** 告警排序：告警在前，其次数据不足，最后正常。 */
export function sortedAlerts(alerts: ObsAlert[]): ObsAlert[] {
    const rank: Record<AlertState, number> = { firing: 0, insufficient_data: 1, ok: 2 };
    return [...alerts].sort((a, b) => rank[a.state] - rank[b.state] || a.id.localeCompare(b.id));
}

export function validJobId(id: string): boolean {
    return /^[A-Za-z0-9_-]{1,40}$/.test(id.trim());
}
