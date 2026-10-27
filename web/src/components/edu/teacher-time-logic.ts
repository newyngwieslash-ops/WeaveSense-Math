/**
 * 教师工时与教学指标（DEV-117）的纯逻辑：何时发送活跃心跳、当前所处阶段、时长与比例文案。
 * 服务端只累加两次心跳之间不超过空闲阈值的真实间隔；这里只保证页面不可见或老师一分钟内没有操作时不发心跳。
 */

export type TeacherPhase = "prep" | "review" | "edit" | "publish" | "after_class";

export const PHASES: TeacherPhase[] = ["prep", "review", "edit", "publish", "after_class"];

export const PHASE_LABEL: Record<TeacherPhase, string> = {
    prep: "备课",
    review: "审核",
    edit: "修改",
    publish: "发布与布置",
    after_class: "课后反馈",
};

/** 心跳间隔与“最近有操作”的判定窗口（毫秒）。 */
export const BEAT_EVERY_MS = 60_000;
export const ACTIVE_WINDOW_MS = 60_000;

export function shouldBeat(now: number, lastActive: number, visible: boolean): boolean {
    return visible && lastActive > 0 && now - lastActive <= ACTIVE_WINDOW_MS;
}

/** 备课页的阶段：未生成讲解为备课；待审为审核；被退回或在编辑为修改；已通过或已发布为发布与布置。 */
export function authorPhase(lessonState: string | undefined, editing: boolean, published: boolean): TeacherPhase {
    if (!lessonState) return "prep";
    if (published || lessonState === "approved") return "publish";
    if (editing || lessonState === "rejected") return "edit";
    if (lessonState === "ready_for_review") return "review";
    return "prep";
}

export function durationText(seconds: number): string {
    const m = Math.round(Math.max(0, seconds) / 60);
    if (m < 60) return `${m} 分钟`;
    return `${Math.floor(m / 60)} 小时 ${m % 60} 分钟`;
}

export type PhaseSum = { measuredSeconds: number; diaryMinutes: number; diaryEntries: number; correctedSeconds: number };
export type TeacherTimeView = {
    from: string;
    to: string;
    scope: "self" | "org";
    phases: Record<TeacherPhase, PhaseSum>;
    total: PhaseSum;
    daysWithActivity: string[];
    idleThresholdSeconds: number;
    definition: string;
    teachers?: number;
    smallSample?: boolean;
};

export function phaseLine(p: TeacherPhase, s: PhaseSum): string {
    const base = `${PHASE_LABEL[p]}：实测 ${durationText(s.measuredSeconds)}`;
    if (s.diaryEntries === 0) return base;
    return `${base}；时间日记 ${s.diaryMinutes} 分钟（${s.diaryEntries} 条）；校正后 ${durationText(s.correctedSeconds)}`;
}

export function timeFootnote(v: TeacherTimeView): string {
    return `统计 ${v.from} 至 ${v.to}（UTC+8），空闲超过 ${Math.round(v.idleThresholdSeconds / 60)} 分钟不计；口径 ${v.definition}。有时间日记的日期与阶段以日记为准，实测值同时列出。`;
}

export function diaryProblem(day: string, minutes: string, today: string): string {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(day)) return "请选择日期";
    if (day > today) return "不能填写未来的日期";
    if (!/^\d+$/.test(minutes.trim())) return "请填写分钟数（整数）";
    const n = Number(minutes);
    if (n < 0 || n > 960) return "分钟数应在 0 到 960 之间";
    return "";
}

export type TeachingMetrics = {
    denominator: number;
    attempted: number;
    outcomes: { independent: number; assisted: number; not_correct: number; pending: number; missing: number };
    firstTry: { correct: number; of: number };
    independentSelfTests: number;
    excludedOutsideRoster: number;
    smallSample: boolean;
    demo: { classes: number; records: number; included: boolean };
    version: { definition: string; publicationId: string; lessonRevisionId: string };
};

/** 比例一律带分母；分母为 0 时写“不可计算”。 */
export function ratio(n: number, d: number): string {
    if (d <= 0) return `${n}/0（不可计算）`;
    return `${n}/${d}（${Math.round((n / d) * 100)}%）`;
}

export function teachingMetricLines(m: TeachingMetrics): string[] {
    const d = m.denominator;
    const lines = [
        `在读学生 ${d} 人，已有效作答 ${m.attempted} 人`,
        `独立答对 ${ratio(m.outcomes.independent, d)}`,
        `看提示后答对 ${ratio(m.outcomes.assisted, d)}`,
        `尚未答对 ${ratio(m.outcomes.not_correct, d)}；待老师判定 ${ratio(m.outcomes.pending, d)}`,
        `缺测（未有效作答）${ratio(m.outcomes.missing, d)}`,
        `首次有效作答即独立答对 ${ratio(m.firstTry.correct, m.firstTry.of)}`,
        `独立自测已提交 ${m.independentSelfTests} 人`,
    ];
    if (m.excludedOutsideRoster > 0) lines.push(`已离班学生 ${m.excludedOutsideRoster} 人的作答不计入`);
    if (m.smallSample) lines.push("样本少于 5 人，比例仅供参考");
    lines.push(m.demo.classes > 0 ? `示例班级的 ${m.demo.records} 条虚构记录单独存放，未计入以上数字` : "没有示例班级数据，演示数据不会计入");
    lines.push(`口径 ${m.version.definition}；讲解修订 ${m.version.lessonRevisionId}`);
    return lines;
}
