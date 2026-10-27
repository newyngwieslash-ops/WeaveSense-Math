/** DEV-107 data subject requests: labels, local checks and impact text. */
export type DataRequest = {
    id: string;
    studentUserId: string;
    kind: string;
    requester: string;
    reason: string;
    status: string;
    verificationRef?: string;
    outcome?: string;
    createdAt: string;
    resolvedAt?: string;
};

type Stop = { studentRoleRevoked: number; consentWithdrawn: number; enrollmentsWithdrawn: number };
export type DataRequestImpact = {
    kind: string;
    records?: Record<string, number>;
    stopNewProcessing?: Stop;
    onlineDeleted?: { learningRows: Record<string, number>; rosterEntries: number };
    backupExpiry?: string;
    recordsKept?: string;
    rejected?: string;
};

export const KIND_LABEL: Record<string, string> = { access: "查阅副本", withdraw: "撤回同意并停止处理", delete: "删除学习记录" };
export const REQUESTER_LABEL: Record<string, string> = { student: "学生本人", guardian: "监护人", institution: "学校" };
export const STATUS_LABEL: Record<string, string> = { received: "已登记，待核验身份", verified: "已核验，待处理", completed: "已完成", rejected: "已拒绝" };

/** Mirrors the server: 4 to 200 (reason) or 4 to 120 (verification reference) characters, no angle brackets or line breaks. */
export function textProblem(s: string, max: number, what: string): string {
    const t = [...s.trim()];
    if (t.length < 4 || t.length > max) return `${what}须为 4 到 ${max} 字`;
    if (/[<>\r\n]/.test(s)) return `${what}不能包含尖括号或换行`;
    return "";
}

const sum = (m?: Record<string, number>) => Object.values(m || {}).reduce((a, b) => a + b, 0);

/** Plain language impact of a request, for the preview and the completed outcome. */
export function impactText(o: DataRequestImpact): string {
    if (o.rejected) return `已拒绝：${o.rejected}`;
    const parts: string[] = [];
    if (o.kind === "access") parts.push(`副本包含学习记录 ${sum(o.records)} 条，教师批注与答案不在副本内`);
    if (o.stopNewProcessing) {
        const s = o.stopNewProcessing;
        parts.push(`停止新的处理：撤销学生身份 ${s.studentRoleRevoked} 个，撤回同意 ${s.consentWithdrawn} 份，移出班级 ${s.enrollmentsWithdrawn} 个`);
    }
    if (o.onlineDeleted) parts.push(`在线删除学习记录 ${sum(o.onlineDeleted.learningRows)} 条，名单认领 ${o.onlineDeleted.rosterEntries} 条`);
    if (o.kind === "delete") parts.push("无单独缓存，媒体链接最长 5 分钟且每次复核权限");
    if (o.backupExpiry) parts.push(`备份中的副本在 ${o.backupExpiry} 前随备份到期`);
    if (o.kind === "delete") parts.push("账本、用量与审计依法单独保留，只含编号与计数");
    if (o.recordsKept) parts.push("已有学习记录保留到保留期或删除请求");
    return `${parts.join("；")}。`;
}

export function parseOutcome(r: DataRequest): DataRequestImpact | null {
    if (!r.outcome) return null;
    try {
        return JSON.parse(r.outcome) as DataRequestImpact;
    } catch {
        return null;
    }
}

export function jsonFilename(name: string): string {
    const base = name
        .replace(/[^A-Za-z0-9_.-]/g, "_")
        .replace(/^\.+/, "")
        .replace(/\.json$/, "")
        .slice(0, 110);
    return `${base || "data-request"}.json`;
}

/** The access package is handed over once as a JSON file; it is not stored on the platform. */
export function saveJson(name: string, value: unknown) {
    const url = URL.createObjectURL(new Blob([JSON.stringify(value, null, 2)], { type: "application/json;charset=utf-8" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = jsonFilename(name);
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
}
