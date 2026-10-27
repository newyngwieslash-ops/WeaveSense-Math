import { teacherError } from "./teacher-errors";

// DEV-070: teaching goal, style and length for a lesson build, the admission
// check before starting, and the recovery actions after a job did not succeed.
export type PlanBlocker = { code: string; message: string; action: string };
export type LessonPlan = {
    admitted: boolean;
    blockers: PlanBlocker[];
    teaching: { goals: string[]; styles: string[]; minMinutes: number; maxMinutes: number; defaultMinutes: number };
    estimate: { stages: { name: string; maxComputeMs: number; maxCostMicro: number }[]; maxComputeMs: number; maxCostMicro: number; unitMicro: number; priceVersion: string };
    queue: { queued: number; running: number };
};
export type Teaching = { goal: string; style: string; targetMinutes: number };
export type LessonDuration = { estimatedSeconds: number; basis: string; fit: string; targetMinutes?: number; targetFit?: string };

export const GOAL_LABEL: Record<string, string> = { "": "不指定", understand_method: "理解方法", practice: "巩固练习", fix_mistake: "纠正易错点" };
export const STYLE_LABEL: Record<string, string> = { concise: "简洁", standard: "标准", detailed: "详细" };

const ACTION_LABEL: Record<string, string> = {
    teacher_review: "先完成数学分析并通过验证",
    wait_or_contact_admin: "等待管理员恢复生成，或联系管理员",
    wait_for_resume: "等待管理员恢复生成",
    retry_later: "稍后再试",
    raise_budget: "请管理员提高预算",
    contact_admin: "联系管理员",
    reconcile: "请求对账（供应商结果未知，不会自动重复计费）",
    restart_new_key: "重新生成（新任务，按实际用量结算）",
    edit_problem: "回到第 1 步修改题目后重新分析",
};
export function actionLabel(a: string): string {
    return ACTION_LABEL[a] || a;
}

// Shown by code only (DEV-083): the server message is English and not for teachers.
export function blockerText(b: PlanBlocker): string {
    return teacherError({ code: b.code });
}

export function teachingBody(t: Teaching): Record<string, string | number> {
    const body: Record<string, string | number> = { style: t.style, targetMinutes: t.targetMinutes };
    if (t.goal) body.goal = t.goal;
    return body;
}

export function stageEstimateText(p: LessonPlan): string {
    const secs = Math.round(p.estimate.maxComputeMs / 1000);
    const cost = p.estimate.unitMicro ? `，费用上限 ${p.estimate.maxCostMicro} 微额度（价目 ${p.estimate.priceVersion}，按实际用量结算）` : `；当前价目 ${p.estimate.priceVersion} 未对计算计费`;
    return `共 ${p.estimate.stages.length} 个阶段，最多 ${secs} 秒计算${cost}`;
}

export function queueText(p: LessonPlan): string {
    return p.queue.queued || p.queue.running ? `本机构当前排队 ${p.queue.queued} 个、运行 ${p.queue.running} 个任务` : "当前无排队任务";
}

export function durationText(d: LessonDuration | null | undefined): string {
    if (!d) return "";
    const m = Math.floor(d.estimatedSeconds / 60);
    const s = d.estimatedSeconds % 60;
    const base = `按旁白字数估算约 ${m} 分 ${s} 秒（尚无音频，仅为估算）`;
    const fit = d.fit === "shorter_than_guidance" ? "，短于建议的 3 至 8 分钟" : d.fit === "longer_than_guidance" ? "，长于建议的 3 至 8 分钟" : "";
    const target = d.targetMinutes ? (d.targetFit === "shorter" ? `，比目标 ${d.targetMinutes} 分钟短` : d.targetFit === "longer" ? `，比目标 ${d.targetMinutes} 分钟长` : `，接近目标 ${d.targetMinutes} 分钟`) : "";
    return base + fit + target;
}
