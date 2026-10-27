/**
 * DEV-112：迁移任务、批量重评分、撤回发布（阻断内容事故）都先向服务端请求预览，
 * 显示具体影响后由有权教师填写原因再确认。权限与校验以服务端为准，这里只做同样的本地检查。
 */
export type MigratePreview = {
    fromAssignmentId: string;
    toPublicationId: string;
    affectedStudents: number;
    attemptsKept: number;
    incidentId: string;
    variantSetDropped: boolean;
    confirmed: boolean;
    toAssignmentId?: string;
};
export type RegradeChange = { attemptId: string; studentUserId: string; oldResult: string; newResult: string; teacherJudged: boolean };
export type RegradePreview = {
    refPublicationId: string;
    examined: number;
    changed: RegradeChange[] | null;
    unchanged: number;
    skipped: string[] | null;
    affectedStudents: number;
    confirmed: boolean;
    written?: number;
};
export type PublicationImpact = {
    assignments: { id: string; title?: string; state?: string }[] | null;
    classes: string[] | null;
    students: number;
    attempts: number;
    variantSets: string[] | null;
    revokedArtifacts: string[] | null;
    childRevisions: string[] | null;
};

/** 原因与服务端一致：去掉首尾空白后 1 到 max 个字。 */
export function actionReasonProblem(reason: string, max: number): string {
    const n = [...reason.trim()].length;
    if (n === 0) return "请填写原因";
    if (n > max) return `原因不超过 ${max} 字`;
    return "";
}

export function migratePreviewText(p: MigratePreview): string {
    const parts = [`${p.affectedStudents} 名学生的 ${p.attemptsKept} 次作答留在原任务，不改分数`, "新任务沿用开始与截止时间和答案政策"];
    if (p.variantSetDropped) parts.push("原任务的变式题组不会带到新任务");
    if (p.incidentId) parts.push(`关联内容事故 ${p.incidentId}`);
    return parts.join("；") + "。迁移后原任务不再接受作答，同一任务只能迁移一次；需要改分请另行重评分。";
}

const RESULT_TEXT: Record<string, string> = {
    correct: "正确",
    correct_approx: "近似正确",
    incorrect: "错误",
    needs_exact: "需精确值",
    needs_teacher: "待老师判断",
    invalid: "无法判定",
};
export const resultText = (r: string) => RESULT_TEXT[r] || r || "未判定";

export function regradePreviewText(p: RegradePreview): string {
    const changed = p.changed ?? [];
    const skipped = p.skipped?.length ?? 0;
    const judged = changed.filter((c) => c.teacherJudged).length;
    const parts = [`检查 ${p.examined} 次作答`, `${changed.length} 次结果会改变（涉及 ${p.affectedStudents} 名学生）`, `${p.unchanged} 次不变`];
    if (skipped) parts.push(`${skipped} 次判分器无法判定，跳过且不改为错误`);
    if (judged) parts.push(`${judged} 次已有老师人工判定，人工判定保持不变`);
    return parts.join("；") + "。";
}

export function regradeChangeText(c: RegradeChange): string {
    return `${c.studentUserId}：${resultText(c.oldResult)} → ${resultText(c.newResult)}${c.teacherJudged ? "（老师已人工判定，保持不变）" : ""}`;
}

export function publicationImpactText(i: PublicationImpact): string {
    const n = (x: unknown[] | null) => (x ? x.length : 0);
    const parts = [`涉及 ${n(i.assignments)} 个任务（${n(i.classes)} 个班级、${i.students} 名在读学生），进行中的任务会暂停，${i.attempts} 次作答保留`];
    if (n(i.variantSets)) parts.push(`撤回 ${n(i.variantSets)} 个变式题组`);
    if (n(i.revokedArtifacts)) parts.push(`已发给学生的 ${n(i.revokedArtifacts)} 个产物链接立即失效`);
    if (n(i.childRevisions)) parts.push(`另有 ${n(i.childRevisions)} 个由此编辑出的修订，请检查是否沿用了同一错误`);
    return parts.join("；") + "。撤回不可恢复，须发布经人工审核的修正版本后关闭事故。";
}
