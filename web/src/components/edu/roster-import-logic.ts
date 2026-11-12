/**
 * 教师名单导入与学生认领码的界面逻辑（DEV-016 前端，E93）。
 * 是否接受某一行、能否确认导入，最终仍由服务端判定。
 */

export const MAX_ROSTER_CSV_BYTES = 64 * 1024;
export const DEFAULT_CLAIM_TTL_HOURS = 14 * 24;
export const ROSTER_CSV_TEMPLATE = "externalRef,displayName\nS001,甲\nS002,乙\n";

export type RosterImportRow = {
    line: number;
    externalRef: string;
    displayName: string;
    status: string;
    error?: string;
};
export type IssuedClaimCode = { entryId: string; externalRef: string; code: string; expiresAt: string };
export type RosterImportPreview = {
    classId?: string;
    rows: RosterImportRow[];
    new: number;
    existing: number;
    invalid: number;
    confirmed?: boolean;
    batch?: string;
    codes?: IssuedClaimCode[];
};
export type RosterEntryRow = {
    id: string;
    externalRef: string;
    displayName?: string;
    state: string;
    expiresAt?: string;
    claimedBy?: string;
    claimedAt?: string;
    importBatch?: string;
};

export const ROSTER_ROW_STATUS_LABEL: Record<string, string> = {
    new: "新行",
    existing: "已存在",
    invalid: "无效",
};
export const ROSTER_ENTRY_STATE_LABEL: Record<string, string> = {
    pending: "待认领",
    claimed: "已认领",
    revoked: "已作废",
};

/** 与服务端 64 KiB 上限一致，按 UTF-8 字节计。 */
export function rosterCsvBytes(text: string): number {
    return new TextEncoder().encode(text).length;
}

export function rosterCsvTooLarge(text: string, max = MAX_ROSTER_CSV_BYTES): boolean {
    return rosterCsvBytes(text) > max;
}

export function rosterPreviewSummary(p: Pick<RosterImportPreview, "new" | "existing" | "invalid">): string {
    let s = `新 ${p.new} 行，已存在 ${p.existing} 行，无效 ${p.invalid} 行。`;
    if (p.invalid > 0) s += "无效行须改正后才能确认。";
    else if (p.new === 0) s += "没有可导入的新行。";
    return s;
}

/** 确认前的界面检查；服务端仍会再校验一次。 */
export function rosterConfirmProblem(preview: Pick<RosterImportPreview, "new" | "invalid"> | null, csv: string): string {
    if (!csv.trim()) return "请粘贴或选择 CSV 名单";
    if (rosterCsvTooLarge(csv)) return "CSV 不能超过 64 KiB";
    if (!preview) return "请先预览名单，核对后再确认";
    if (preview.invalid > 0) return "请先改正无效行再确认";
    if (preview.new === 0) return "没有可导入的新行";
    return "";
}

export function rosterCodesNotice(n: number): string {
    return `认领码已生成 ${n} 个，只显示这一次，服务端只保存哈希，请现在发给对应学生`;
}

export function formatClaimCodes(codes: IssuedClaimCode[]): string {
    return codes.map((c) => `${c.externalRef}\t${c.code}`).join("\n");
}

/** 邀请码为 eci_… 加 48 位密钥，名单认领码为 erc_… 加 48 位密钥。 */
export function joinCodeKind(raw: string): "invite" | "claim" | "unknown" {
    const v = raw.trim();
    const dot = v.indexOf(".");
    if (dot <= 0) return "unknown";
    const id = v.slice(0, dot);
    const secret = v.slice(dot + 1);
    if (secret.length !== 48) return "unknown";
    if (id.startsWith("eci_")) return "invite";
    if (id.startsWith("erc_")) return "claim";
    return "unknown";
}

export function joinSuccessText(kind: "invite" | "claim", consentRequired?: boolean): string {
    const base = kind === "claim" ? "已认领并加入班级" : "已加入班级";
    if (consentRequired) return `${base}。还需要老师或机构记录必要同意后，才能使用学习功能`;
    return base;
}

/** 学生端加入/认领错误说明；从不包含服务端原文。 */
export function joinErrorText(e: { status?: number; code?: string; offline?: boolean }): string {
    if (e.offline || e.code === "NETWORK") return "网络不可用，本次操作没有提交。下一步：检查网络后重试";
    if (e.status === 401 || e.code === "HTTP_401") return "登录已失效。下一步：重新登录后再试";
    if (e.code === "POLICY_REVOKED" || e.status === 403 || e.code === "HTTP_403") return "当前无法加入该班级。下一步：请联系老师";
    if (e.status === 404 || e.code === "HTTP_404" || e.code === "NOT_FOUND") return "邀请码或认领码无效或已过期。下一步：向老师确认后再试";
    const code = e.code || "UNKNOWN";
    return `加入没有完成。下一步：稍后重试；持续出现请把错误代码告诉老师（错误代码 ${code}）`;
}
