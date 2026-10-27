/**
 * 教师管理与自测题组的纯逻辑，便于单元测试。
 * 所有判断只用于界面提示；是否允许审核、布置、延期都由服务端最终决定。
 */
export type VariantItem = {
    id: string;
    family_id: string;
    params: Record<string, string>;
    unit: string;
    prompt: string;
    answer_format: string;
    change_type?: string;
    transfer?: string;
    method?: string;
    math_state?: string;
    evidence_levels?: string[];
    /** generated（服务生成）或 teacher_own（教师自有数字替换，DEV-078）。 */
    source?: string;
};
export type GradingScope = { auto: boolean; covers: string; equivalence?: string; decimal?: string; steps: string; unit?: string };
export type VariantKey = { id: string; exact: string; plain: string; latex: string };
export type VariantSet = {
    id: string;
    lessonRevisionId: string;
    jobId: string;
    setHash: string;
    familyId: string;
    requested: number;
    itemCount: number;
    state: string;
    blockingErrors: string;
    createdBy: string;
    createdAt: string;
    reviewedAt?: string | null;
    items: VariantItem[];
    answerKeys?: Record<string, VariantKey>;
    dropped?: { id?: string; reason: string; params?: Record<string, string> }[];
    reviews?: { id: string; reviewerId: string; decision: string; comment: string; selfReview: boolean; createdAt: string }[];
    reviewMode?: string;
    supersedesSetId?: string;
    replacementId?: string;
    grading?: Record<string, GradingScope>;
};

export const VARIANT_STATE_LABEL: Record<string, string> = {
    ready_for_review: "待审核",
    approved: "已审核通过，可布置",
    rejected: "已退回，不可布置",
};
/** 剔除原因：answer_reused、not_verified:<数学状态>，或数学服务错误码。 */
export function dropReasonLabel(reason: string): string {
    if (reason === "answer_reused") return "答案与原题或其他变式重复";
    if (reason.startsWith("not_verified:")) return `未通过独立验证（${reason.slice(13)}）`;
    return `求解失败（${reason}）`;
}

/** 只有待审核、无阻断错误且至少一题的题组才可提交“通过”。 */
export function variantApprovable(s: Pick<VariantSet, "state" | "blockingErrors" | "itemCount">): boolean {
    return s.state === "ready_for_review" && !s.blockingErrors && s.itemCount > 0;
}

/** 少于请求数量时如实告知，不补凑题目。 */
export function variantShortfall(s: Pick<VariantSet, "requested" | "itemCount">): string {
    if (s.itemCount >= s.requested) return "";
    return `请求 ${s.requested} 题，独立验证后保留 ${s.itemCount} 题（不足部分不会补凑）`;
}

export type InviteRow = { id: string; status: string; expiresAt: string; maxUses: number; uses: number; createdAt: string };
export const INVITE_STATUS_LABEL: Record<string, string> = {
    active: "有效",
    revoked: "已撤销",
    exhausted_or_expired: "已用尽或已过期",
};

export type RosterRow = { enrollmentId: string; studentUserId: string; status: string; consentOk: boolean };
export type ExtensionRow = { id: string; studentUserId: string; dueAt: string; reason: string; createdAt: string };

/**
 * 把 datetime-local 的本地时间转为带时区的 ISO 字符串，并校验延期晚于原截止时间。
 * 返回错误文本或 ISO 值；服务端仍会再校验一次。
 */
export function extensionDueISO(local: string, assignmentDue?: string | null): { ok: true; iso: string } | { ok: false; error: string } {
    if (!local) return { ok: false, error: "请选择延期截止时间" };
    const d = new Date(local);
    if (Number.isNaN(d.getTime())) return { ok: false, error: "时间格式无效" };
    if (assignmentDue && d.getTime() <= new Date(assignmentDue).getTime()) return { ok: false, error: "延期截止时间必须晚于作业原截止时间" };
    return { ok: true, iso: d.toISOString() };
}

export type SetAttempt = { id: string; item: string; result: string; valid: boolean; response: string };

/** 题组自测进度：哪些题已提交、还剩几题。每题只提交一次。 */
export function setProgress(questions: { id: string }[], attempts: SetAttempt[]) {
    const done = new Map<string, SetAttempt>();
    for (const a of attempts) if (a.item !== "main" && a.valid && !done.has(a.item)) done.set(a.item, a);
    const remaining = questions.filter((q) => !done.has(q.id)).length;
    return { done, remaining, complete: questions.length > 0 && remaining === 0 };
}

/** 自测视角预览（DEV-075）：只取学生能看到的字段（题干、单位、作答格式），不带参数、数学状态与参考答案。 */
export function selfTestPreview(set: Pick<VariantSet, "items">) {
    return {
        state: "open",
        helpUsed: false,
        sawSolution: false,
        measurement: "set" as const,
        questions: set.items.map((it) => ({ id: it.id, prompt: it.prompt, unit: it.unit, answer_format: it.answer_format })),
    };
}

/** 自动评分范围的教师说明（DEV-078）：只说明服务端判分实际覆盖的内容。 */
export function gradingScopeLabel(g?: GradingScope): string {
    if (!g) return "自动评分范围未知，布置前请向管理员确认";
    if (!g.auto) return "不自动评分，需教师批改";
    const parts = ["自动评分只判最终答案"];
    if (g.equivalence === "exact_value") parts.push("按精确值等价判定（化简、分数与根式的等价写法均算对）");
    if (g.decimal === "needs_exact") parts.push("只写小数会提示需要精确值");
    if (g.unit === "checked_when_expected") parts.push("有单位的题检查单位");
    if (g.steps === "not_graded") parts.push("解题过程不自动评分");
    return parts.join("；");
}

/** 题组阻断错误的教师说明；teacher_item_dropped 表示教师自有题没有通过检查。 */
export function blockingLabel(code: string): string {
    const [kind, reason = ""] = code.split(/:(.*)/s);
    if (kind === "teacher_item_dropped") return `自有题未通过：${dropReasonLabel(reason)}`;
    if (kind === "item_dropped") return `原有题未通过：${dropReasonLabel(reason)}`;
    if (code === "item_count_short") return "题数少于替换前，不能审核";
    if (kind.endsWith("math_not_verified") || reason === "math_not_verified") return `${kind} 数学未验证`;
    return code;
}

export function parseBlocking(raw: string): string[] {
    if (!raw) return [];
    try {
        const v = JSON.parse(raw);
        return Array.isArray(v) ? v.map(String) : [raw];
    } catch {
        return [raw];
    }
}

const PARAM_VALUE = /^[0-9a-zA-Z_+\-*/^().√ ]{1,32}$/;
/** 替换参数只允许原题的参数名与简短数值表达式；最终由数学服务归一化与验证。 */
export function replaceParamsError(names: string[], params: Record<string, string>): string {
    for (const n of names) {
        const v = (params[n] || "").trim();
        if (!v) return `请填写参数 ${n}`;
        if (!PARAM_VALUE.test(v)) return `参数 ${n} 只能是简短的数值表达式，例如 3、2/3、sqrt(2)`;
    }
    return "";
}

/** DEV-112：撤回任务前服务端给出的影响范围。 */
export type AssignmentImpact = { students: number; studentsWithAttempts: number; attempts: number; openHelp: number; extensions: number };

export function assignmentImpactText(i: AssignmentImpact): string {
    const parts = [`将对班级 ${i.students} 名在读学生隐藏此任务`, `其中 ${i.studentsWithAttempts} 人已作答（共 ${i.attempts} 次作答，记录保留不删除）`];
    if (i.openHelp > 0) parts.push(`${i.openHelp} 条求助尚未处理`);
    if (i.extensions > 0) parts.push(`${i.extensions} 条个人延期随之失效`);
    return parts.join("；") + "。撤回后不能恢复，需要时重新布置。";
}

/** 撤回原因与服务端一致：4 到 200 字，不含尖括号和换行。 */
export function withdrawReasonProblem(reason: string): string {
    const r = reason.trim();
    if ([...r].length < 4) return "请填写撤回原因（至少 4 个字）";
    if ([...r].length > 200) return "撤回原因不超过 200 字";
    if (/[<>\r\n]/.test(r)) return "撤回原因不能包含尖括号或换行";
    return "";
}
