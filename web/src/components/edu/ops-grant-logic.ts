/** DEV-108 temporary operator access: labels, local checks and grant text. */
export type OpsGrant = {
    id: string;
    operatorUserId: string;
    scope: string;
    classId?: string;
    includeContent: boolean;
    purpose: string;
    expiresAt: string;
    status: string;
    grantedBy: string;
    revokedBy?: string;
    revokedAt?: string;
    revokeReason?: string;
    createdAt: string;
};
export type OpsGrantRow = { grant: OpsGrant; effective: string };
export type OpsGrantForm = { operatorUserId: string; scope: string; classId: string; includeContent: boolean; purpose: string; hours: number };

export const OPS_SCOPE_LABEL: Record<string, string> = { jobs: "生成任务状态（不含题目与输入）", class: "单个班级的作答记录" };
export const OPS_HOURS_MAX = 72;

/** Mirrors the server checks; returns "" when the grant can be submitted. */
export function opsGrantProblem(f: OpsGrantForm, self: string): string {
    if (!f.operatorUserId.trim()) return "请填写运维人员的用户编号";
    if (self && f.operatorUserId.trim() === self) return "不能给自己授权";
    if (f.scope !== "jobs" && f.scope !== "class") return "请选择授权范围";
    if (f.scope === "class" && !f.classId) return "请选择班级";
    if (f.scope === "jobs" && f.includeContent) return "任务状态范围不能包含作答内容";
    if (!Number.isInteger(f.hours) || f.hours < 1 || f.hours > OPS_HOURS_MAX) return `有效期为 1 到 ${OPS_HOURS_MAX} 小时`;
    const t = f.purpose.trim();
    const n = [...t].length;
    if (n < 4 || n > 200) return "授权目的需要 4 到 200 字，例如工单号和要排查的问题";
    if (/[<>\r\n]/.test(t)) return "授权目的不能包含尖括号或换行";
    return "";
}

/** One line telling the admin exactly what the operator will be able to see. */
export function opsGrantText(g: OpsGrant, className?: string): string {
    const what = g.scope === "jobs" ? "生成任务的状态与错误码" : `${className || g.classId} 的作答记录${g.includeContent ? "（含作答内容）" : "（不含作答内容）"}`;
    return `${g.operatorUserId} 可查看${what}，到 ${new Date(g.expiresAt).toLocaleString("zh-CN")} 为止。目的：${g.purpose}`;
}

export function opsRevokeProblem(reason: string): string {
    const n = [...reason.trim()].length;
    if (n < 4 || n > 200) return "撤销理由需要 4 到 200 字";
    if (/[<>\r\n]/.test(reason)) return "撤销理由不能包含尖括号或换行";
    return "";
}
