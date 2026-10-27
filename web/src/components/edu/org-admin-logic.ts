/**
 * 机构管理（DEV-102）。概览只显示汇总数字，不显示学生身份；机构管理员查看自己不任课班级的
 * 个体学生记录时必须说明业务目的，服务端逐次写审计并列入访问记录。地域约束（DEV-105）
 * 与保留清理（DEV-106）尚未生效，界面如实显示。
 */
export type OrgClass = { id: string; name: string; workspaceId: string; courseId?: string; teachers: number; students: number; activeAssignments: number; youTeach: boolean };
export type OrgOverview = {
    tenantId: string;
    staff: { teacher: number; reviewer: number; org_admin: number };
    courses: number;
    classes: OrgClass[];
    students: { enrolled: number; guardianConsent: number; consentMissing: number };
    policy: { reviewMode: string; consentMode: string; editReviewInherit: string; updatedAt?: string };
    region: { state: string; blockedBy: string };
    retention: { state: string; reference: string; learningDays?: number; tempDays?: number; lastRunAt?: string };
};
export type RoleRow = { binding: { id: string; userId: string; role: string; workspaceId?: string; status: string; expiresAt?: string }; effective: string };
export type AccessRow = { id: string; action: string; actorId: string; subject: string; createdAt: string; detail?: string };

export const ROLE_LABEL: Record<string, string> = { teacher: "教师", reviewer: "审核员", org_admin: "教育管理员" };
export const REVIEW_MODE_LABEL: Record<string, string> = { self_allowed: "允许教师自审", second_reviewer: "必须由第二人审核" };
export const CONSENT_LABEL: Record<string, string> = { guardian_required: "需要监护人同意", not_applicable: "不适用（须写明理由）" };
export const INHERIT_LABEL: Record<string, string> = { none: "任何修改都重新审核", pronunciation_only: "只改读音时继承审核", presentation_only: "只改呈现时继承审核" };
export const EFFECTIVE_LABEL: Record<string, string> = { active: "有效", expired: "已到期", revoked: "已撤销" };
export const ACTION_LABEL: Record<string, string> = {
    "org.individual_access": "查看个体学生记录",
    "policy.set": "修改审核与同意策略",
    "budget.set": "修改预算",
    "class.teacher.add": "指派任课教师",
    "class.create": "创建班级",
    "class.roster.import": "导入名单",
    "class.roster.reissue": "重发认领码",
    "class.roster.revoke": "作废名单条目",
    "class.enrollment.withdraw": "移出学生",
    "assignment.withdraw": "撤回任务",
    "ledger.export": "导出账本",
    "role.bind": "授予角色",
    "role.renew": "续期角色",
    "role.revoke": "撤销角色",
    "export.content": "导出讲解",
    "export.report": "导出学习报告",
    "retention.set": "确认数据保留期",
    "retention.run": "执行数据保留清理",
    "datareq.create": "登记数据请求",
    "datareq.verify": "核验数据请求身份",
    "datareq.reject": "拒绝数据请求",
    "datareq.complete": "完成数据请求",
    "datareq.replay": "恢复后重放删除",
    "ops.grant": "运维临时授权",
    "ops.revoke": "撤销运维授权",
    "ops.read": "运维查看",
};

export const PURPOSE_MIN = 4;
export const PURPOSE_MAX = 200;

/** 业务目的的本地检查，与服务端一致；返回空串表示可以提交。 */
export function purposeProblem(p: string): string {
    const t = p.trim();
    const n = [...t].length;
    if (n < PURPOSE_MIN) return `请写明查看目的（至少 ${PURPOSE_MIN} 个字），例如“家长投诉核查”`;
    if (n > PURPOSE_MAX) return `查看目的不能超过 ${PURPOSE_MAX} 字`;
    if (/[<>\r\n]/.test(t)) return "查看目的不能包含尖括号或换行";
    return "";
}

export function consentLine(s: OrgOverview["students"]): string {
    if (s.enrolled === 0) return "还没有学生加入班级";
    return `在读学生 ${s.enrolled} 人，已登记监护人同意 ${s.guardianConsent} 人${s.consentMissing > 0 ? `，缺少同意 ${s.consentMissing} 人（缺少同意的学生不能开始学习）` : ""}`;
}

export function staffLine(s: OrgOverview["staff"]): string {
    return `教师 ${s.teacher} 人，审核员 ${s.reviewer} 人，教育管理员 ${s.org_admin} 人`;
}

export function regionLine(r: OrgOverview["region"]): string {
    return r.state === "not_configured" ? "供应商地域与出站约束尚未配置（待部署与供应商确认，见 DEV-105）；在此之前不要把真实学生数据接入外部模型。" : `供应商地域：${r.state}`;
}

export function retentionLine(r: OrgOverview["retention"]): string {
    if (r.state === "suggested_not_enforced") return "数据保留期为建议值，尚未执行自动清理（待机构确认，见 09 第 3.3 节）。";
    if (r.state === "confirmed") return `数据保留期已由机构确认：学习记录在学生离开班级 ${r.learningDays} 天后清理，临时文件保留 ${r.tempDays} 天；${r.lastRunAt ? `最近清理 ${new Date(r.lastRunAt).toLocaleString()}` : "尚未执行清理"}。`;
    return `数据保留：${r.state}`;
}

export function accessRowText(r: AccessRow): string {
    const label = ACTION_LABEL[r.action] || r.action;
    const purpose =
        r.action === "org.individual_access" && r.detail
            ? `；目的：${r.detail.split(" | ").slice(1).join(" | ")}`
            : (r.action.startsWith("retention.") || r.action.startsWith("datareq.") || r.action.startsWith("ops.") || r.action === "assignment.withdraw") && r.detail
              ? `；${retentionDetail(r.detail)}`
              : "";
    return `${label}（${r.actorId}，对象 ${r.subject}）${purpose}`;
}

/** DEV-106 retention: suggested periods, bounds and the impact of a cleanup run. */
export type RetentionDays = { tempDays: number; imageDays: number; learningDays: number; backupDays: number };
export type RetentionSummary = { enrollments: number; learningRows: Record<string, number>; rosterEntries: number; artifacts: number };
export type RetentionView = {
    policy: RetentionDays & { state: string; reason?: string; confirmedBy?: string; confirmedAt?: string; lastRunAt?: string; lastRunSummary?: string };
    confirmed: boolean;
    defaults: RetentionDays;
    bounds: Record<keyof RetentionDays, [number, number]>;
    preview: RetentionSummary;
    keptSeparately: string[];
};

export const RETENTION_LABEL: Record<keyof RetentionDays, string> = {
    tempDays: "临时文件（失败或取消任务的产物）",
    imageDays: "学生图片作答",
    learningDays: "学生离开班级后的学习记录",
    backupDays: "备份",
};

const RETENTION_TABLE: Record<string, string> = {
    edu_attempts: "作答",
    edu_hint_unlocks: "提示解锁",
    edu_self_tests: "自测",
    edu_review_items: "复习项",
    edu_help_requests: "求助",
    edu_predictions: "预测",
    edu_figure_interactions: "图形操作",
    edu_regrades: "重判",
    edu_assignment_extensions: "延期",
};

/** Mirrors the server bounds; returns "" when every period is inside them. */
export function retentionProblem(d: RetentionDays, bounds: RetentionView["bounds"], reason: string): string {
    for (const k of Object.keys(RETENTION_LABEL) as (keyof RetentionDays)[]) {
        const [lo, hi] = bounds[k];
        if (!Number.isInteger(d[k]) || d[k] < lo || d[k] > hi) return `${RETENTION_LABEL[k]}的保留期须为 ${lo} 到 ${hi} 天`;
    }
    const r = reason.trim();
    if ([...r].length < 4 || [...r].length > 200) return "确认保留期须写明 4 到 200 字的理由或依据";
    if (/[<>\n\r]/.test(r)) return "理由中不能包含尖括号或换行";
    return "";
}

export function retentionImpact(s: RetentionSummary): string {
    const rows = Object.entries(s.learningRows || {})
        .filter(([, n]) => n > 0)
        .map(([t, n]) => `${RETENTION_TABLE[t] || t} ${n} 条`);
    if (s.enrollments === 0 && s.artifacts === 0 && rows.length === 0) return "按当前保留期，现在没有需要清理的数据。";
    const parts = [`涉及已离开班级 ${s.enrollments} 人次`];
    if (rows.length) parts.push(`学习记录：${rows.join("，")}`);
    if (s.rosterEntries) parts.push(`名单认领 ${s.rosterEntries} 条`);
    if (s.artifacts) parts.push(`临时文件 ${s.artifacts} 个`);
    return `${parts.join("；")}。账本、用量与审计记录不在清理范围内。`;
}

function retentionDetail(d: string): string {
    const m = /^enrollments=(\d+) learning_rows=(\d+) roster_entries=(\d+) artifacts=(\d+)$/.exec(d);
    if (m) return `离开班级 ${m[1]} 人次，学习记录 ${m[2]} 条，名单认领 ${m[3]} 条，临时文件 ${m[4]} 个`;
    const p = /^temp=(\d+) image=(\d+) learning=(\d+) backup=(\d+) \| (.*)$/.exec(d);
    if (p) return `临时 ${p[1]} 天，图片 ${p[2]} 天，学习记录 ${p[3]} 天，备份 ${p[4]} 天；理由：${p[5]}`;
    return d;
}
