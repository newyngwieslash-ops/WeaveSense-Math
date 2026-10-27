import { formatMicro } from "@/components/edu/ledger-logic";

/** DEV-118 成本分层：与服务端 cost_breakdown/1 口径一致。 */
export type CostBy = "tenant" | "family" | "class" | "reuse";
export const COST_BY: CostBy[] = ["tenant", "family", "class", "reuse"];
export const COST_BY_LABEL: Record<CostBy, string> = { tenant: "机构", family: "题族", class: "班级", reuse: "复用" };

export type CostRow = {
    key: string;
    costMicro: number;
    machineMicro: number;
    failedMicro: number;
    supportMicro: number;
    jobs: number;
    retriedJobs: number;
    accepted: number;
    classDeliveries: number;
    perAcceptedMicro: number | null;
    perClassDeliveryMicro: number | null;
    computable: boolean;
};
export type CostBreakdown = {
    period: string;
    definition: string;
    by: CostBy;
    rows: CostRow[] | null;
    totalMicro: number;
    supportUnattributedMicro: number;
    excludedOpenJobs: number;
    notCollected: { dimension: string; reason: string }[];
};

const ROW_LABEL: Record<string, string> = {
    tenant: "本机构",
    no_accepted_lesson: "未形成接受课件",
    not_assigned: "未布置",
    single_class: "单班使用",
    reused: "多班复用",
    support_unattributed: "未归属人工支持",
    support_family_not_split: "按题族记录的人工支持（未分到班）",
    support_class_not_split: "按班级记录的人工支持（不分到发布）",
    unknown: "未知题族",
};
const DIM_LABEL: Record<string, string> = { method: "方法", model: "模型", template: "模板", terminal: "终端" };

export function costRowLabel(key: string): string {
    if (key.startsWith("class:")) return `班级 ${key.slice(6)}`;
    return ROW_LABEL[key] || key;
}

/** 分母为零时显示“不可计算”，不显示 0。 */
export function ratioText(v: number | null, denominator: number): string {
    return denominator > 0 && v !== null ? formatMicro(v) : "不可计算";
}

export function rowDetail(r: CostRow): string {
    const parts = [`机器 ${formatMicro(r.machineMicro)}`];
    if (r.failedMicro > 0) parts.push(`其中失败或取消 ${formatMicro(r.failedMicro)}`);
    if (r.retriedJobs > 0) parts.push(`重试 ${r.retriedJobs} 次`);
    if (r.supportMicro > 0) parts.push(`人工支持 ${formatMicro(r.supportMicro)}`);
    return parts.join("，");
}

export function notCollectedText(b: Pick<CostBreakdown, "notCollected">): string {
    if (!b.notCollected?.length) return "";
    return `未采集，无法分层：${b.notCollected.map((x) => DIM_LABEL[x.dimension] || x.dimension).join("、")}`;
}

/** 各行合计应等于总额；不等时页面提示而不是静默。 */
export function reconciles(b: CostBreakdown): boolean {
    return (b.rows || []).reduce((s, r) => s + r.costMicro, 0) === b.totalMicro;
}
