/** DEV-126 内容事故处置流程的纯逻辑：修订通知与事故复盘。 */
export type RevisionNotice = {
    notice: { id: string; incidentId: string; assignmentId: string; classId: string; fromPublicationId: string; fixPublicationId: string; state: "unread" | "acknowledged" | "migrated"; createdAt: string };
    assignmentTitle: string;
    assignmentState: string;
    severity: string;
    reason: string;
};

export type ReviewInput = { rootCause: string; detection: string; correctiveActions: string; followUps: string };

export const NOTICE_STATE_LABEL: Record<string, string> = { unread: "未读", acknowledged: "已确认", migrated: "已迁移" };

export function noticeLine(n: RevisionNotice): string {
    const st = n.assignmentState === "migrated" ? "任务已迁移" : n.assignmentState === "suspended" ? "任务已暂停" : "任务仍在使用旧版本";
    return `「${n.assignmentTitle || n.notice.assignmentId}」所用内容已更正（${n.reason}）；${st}。修正发布版 ${n.notice.fixPublicationId}`;
}

const len = (s: string) => [...s.trim()].length;
const contact = /[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}|1[3-9]\d{9}/;

/** 与服务端相同的长度规则；联系方式在提交前提示，学生姓名由服务端按班级名单检查。 */
export function reviewInputError(v: ReviewInput): string {
    if (len(v.rootCause) < 4 || len(v.rootCause) > 800) return "根因需 4 到 800 字";
    if (len(v.detection) < 2 || len(v.detection) > 400) return "发现途径需 2 到 400 字";
    if (len(v.correctiveActions) < 4 || len(v.correctiveActions) > 800) return "纠正措施需 4 到 800 字";
    if (len(v.followUps) > 800) return "后续事项不超过 800 字";
    if (contact.test(`${v.rootCause}\n${v.detection}\n${v.correctiveActions}\n${v.followUps}`)) return "复盘不得包含邮箱或手机号";
    return "";
}
