import type { LessonDuration } from "./lesson-plan-logic";
import type { Provenance } from "./provenance-logic";

/**
 * 讲解结构化编辑的纯逻辑（FR-T05/T06，AC-17/18），便于单元测试。
 * 只生成字段级编辑；是否可编辑、影响范围和冲突都以服务端判定为准。
 */
/** 旁白锚点与绑定动作（DEV-071，edu.lesson/2）；只读，改旁白或板书后由数学服务重建。 */
export type CueAction = { kind: string; line?: number; refs?: string[] };
export type Cue = { anchor: string; text: string; est_ms?: number; actions: CueAction[] | null };
export type EditableBeat = { id: string; title: string; say: string; board: string[] | null; reveals_answer?: boolean; cues?: Cue[] | null };
export type EditableHint = { level: number; kind: string; text: string };
/** 图形里老师可改的文字（DEV-073）；几何、帧和预测 id 只随题目修订变化。 */
export type EditableFigure = { description: string; prediction?: { id: string; prompt: string; options: string[] } | null } | null;
export type Editable = {
    lessonRevisionId: string;
    lessonHash: string;
    schemaVersion?: string;
    state: string;
    parentLessonRevisionId?: string;
    beats: EditableBeat[];
    hints: EditableHint[];
    pronunciations: Record<string, string> | null;
    quality?: { edited_unverified?: string[]; math_text?: string; pedagogy?: string } | null;
    editReviewInherit: string;
    latestRevisionId: string;
    limits: { maxEdits: number; maxText: number; maxBoardLines: number; maxLine: number };
    teaching?: { goal: string; style: string; targetMinutes: number };
    duration?: LessonDuration;
    figure?: EditableFigure;
    provenance?: Provenance;
};
export type Draft = {
    beats: Record<string, { title: string; say: string; board: string }>;
    hints: Record<number, string>;
    pronunciations: { term: string; reading: string }[];
    /** 旧草稿没有这一项，按原修订处理。 */
    figure?: { description: string; prompt: string; options: string } | null;
};
export type LessonEdit =
    | { target: "beat"; id: string; field: "title" | "say"; value: string }
    | { target: "beat"; id: string; field: "board"; value: string[] }
    | { target: "hint"; level: number; field: "text"; value: string }
    | { target: "pronunciation"; term: string; reading: string }
    | { target: "figure"; field: "description" | "prediction_prompt"; value: string }
    | { target: "figure"; field: "prediction_options"; value: string[] };

export const EDIT_CLASS_LABEL: Record<string, string> = {
    reasoning: "推理（板书）",
    wording: "措辞",
    pronunciation: "读音",
    layout: "版式",
    figure_text: "图形文字",
};
export const INVALIDATES_LABEL: Record<string, string> = {
    review: "审核结论",
    student_projection: "学生版讲解",
    preview: "预览",
    media: "配音与视频",
    captions: "字幕",
};
export const INHERIT_LABEL: Record<string, string> = {
    none: "所有修改都需重新审核",
    pronunciation_only: "仅改读音时继承上一版审核结论",
};

const boardLines = (s: string) =>
    s
        .split("\n")
        .map((x) => x.trim())
        .filter(Boolean);

export function draftOf(e: Editable): Draft {
    const beats: Draft["beats"] = {};
    for (const b of e.beats) beats[b.id] = { title: b.title, say: b.say, board: (b.board || []).join("\n") };
    const hints: Draft["hints"] = {};
    for (const h of e.hints) hints[h.level] = h.text;
    const pronunciations = Object.entries(e.pronunciations || {}).map(([term, reading]) => ({ term, reading }));
    return { beats, hints, pronunciations, figure: figureDraft(e.figure) };
}

function figureDraft(f: EditableFigure | undefined): Draft["figure"] {
    if (!f) return null;
    return { description: f.description || "", prompt: f.prediction?.prompt || "", options: (f.prediction?.options || []).join("\n") };
}

/** diffEdits 只输出与原修订不同的字段；删除读音以空 reading 表示。 */
export function diffEdits(e: Editable, d: Draft): LessonEdit[] {
    const out: LessonEdit[] = [];
    for (const b of e.beats) {
        const x = d.beats[b.id];
        if (!x) continue;
        if (x.title.trim() !== b.title) out.push({ target: "beat", id: b.id, field: "title", value: x.title.trim() });
        if (x.say.trim() !== b.say) out.push({ target: "beat", id: b.id, field: "say", value: x.say.trim() });
        const lines = boardLines(x.board);
        if (lines.join("\n") !== (b.board || []).join("\n")) out.push({ target: "beat", id: b.id, field: "board", value: lines });
    }
    for (const h of e.hints) {
        const t = (d.hints[h.level] ?? h.text).trim();
        if (t !== h.text) out.push({ target: "hint", level: h.level, field: "text", value: t });
    }
    const before = e.pronunciations || {};
    const after: Record<string, string> = {};
    for (const p of d.pronunciations) if (p.term.trim()) after[p.term.trim()] = p.reading.trim();
    for (const [term, reading] of Object.entries(after)) if (before[term] !== reading) out.push({ target: "pronunciation", term, reading });
    for (const term of Object.keys(before)) if (!(term in after)) out.push({ target: "pronunciation", term, reading: "" });
    const f = e.figure;
    const df = d.figure ?? figureDraft(f);
    if (f && df) {
        if (df.description.trim() !== (f.description || "")) out.push({ target: "figure", field: "description", value: df.description.trim() });
        if (f.prediction) {
            if (df.prompt.trim() !== f.prediction.prompt) out.push({ target: "figure", field: "prediction_prompt", value: df.prompt.trim() });
            const opts = boardLines(df.options);
            if (opts.join("\n") !== f.prediction.options.join("\n")) out.push({ target: "figure", field: "prediction_options", value: opts });
        }
    }
    return out;
}

/** 本地校验：只做提示，服务端仍会再校验。 */
export function draftProblems(e: Editable, edits: LessonEdit[]): string[] {
    const p: string[] = [];
    if (edits.length > e.limits.maxEdits) p.push(`一次最多 ${e.limits.maxEdits} 处修改`);
    for (const x of edits) {
        if (x.target === "beat" && x.field === "board") {
            if (x.value.length < 1 || x.value.length > e.limits.maxBoardLines) p.push(`板书需 1 到 ${e.limits.maxBoardLines} 行`);
            if (x.value.some((l) => l.length > e.limits.maxLine)) p.push(`板书每行不超过 ${e.limits.maxLine} 字`);
        } else if (x.target === "figure" && x.field === "prediction_options") {
            if (x.value.length < 2 || x.value.length > 6) p.push("预测选项需 2 到 6 个");
            if (new Set(x.value).size !== x.value.length) p.push("预测选项不能重复");
            if (x.value.some((o) => o.length > 60)) p.push("每个预测选项不超过 60 字");
        } else if (x.target !== "pronunciation" && (!x.value || x.value.length > e.limits.maxText)) {
            p.push(`文字需 1 到 ${e.limits.maxText} 字`);
        }
    }
    return [...new Set(p)];
}

export const draftKey = (lessonId: string, hash: string) => `edu-edit-draft:${lessonId}:${hash}`;

/** 冲突响应里服务端回传的信息（AC-17）：最新修订与本人的修改。 */
export function conflictOf(detail: unknown): { reason: string; latestRevisionId: string; yourEdits: LessonEdit[] } | null {
    const d = detail as { conflict?: { reason?: string; latestRevisionId?: string }; yourEdits?: LessonEdit[] } | null;
    if (!d?.conflict) return null;
    return { reason: d.conflict.reason || "", latestRevisionId: d.conflict.latestRevisionId || "", yourEdits: d.yourEdits || [] };
}

export function estimateText(est: { maxComputeMs?: number; maxCostMicro?: number; unitMicro?: number; priceVersion?: string } | null | undefined): string {
    if (!est) return "";
    if (!est.unitMicro) return `最多 ${Math.round((est.maxComputeMs || 0) / 1000)} 秒计算；当前价目 ${est.priceVersion || ""} 未对计算计费`;
    return `最多 ${Math.round((est.maxComputeMs || 0) / 1000)} 秒计算，费用上限 ${est.maxCostMicro} 微额度（价目 ${est.priceVersion}，按实际用量结算）`;
}

/** 版本对比（DEV-073）：服务端逐字段比较同一题目的两版讲解，这里只负责文案。 */
export type LessonChange = { section: string; key: string; field: string; before: unknown; after: unknown };
const SECTION_LABEL: Record<string, string> = {
    problem: "题目",
    answer: "答案",
    beat: "步骤",
    hint: "提示",
    figure: "图形",
    pronunciation: "读音",
    teaching: "讲解设置",
    layout: "版式",
};
const FIELD_LABEL: Record<string, string> = {
    title: "标题",
    say: "讲解",
    board: "板书",
    text: "文字",
    plain: "答案",
    latex: "公式",
    description: "说明",
    prompt: "预测问题",
    options: "预测选项",
    reading: "读音",
    goal: "目标",
    style: "风格",
    targetMinutes: "目标时长",
    layout: "版式",
};
export function changeLabel(c: LessonChange): string {
    const where = c.section === "beat" ? `步骤 ${c.key}` : c.section === "hint" ? `提示 ${c.key}` : c.section === "pronunciation" ? `读音「${c.key}」` : SECTION_LABEL[c.section] || c.section;
    const kind = c.before == null ? "（新增）" : c.after == null ? "（删除）" : "";
    return `${where} ${FIELD_LABEL[c.field] || c.field}${kind}`;
}
export function changeValue(v: unknown): string {
    if (v == null) return "无";
    if (Array.isArray(v)) return v.map(String).join(" / ") || "空";
    if (typeof v === "object") return JSON.stringify(v);
    return String(v) || "空";
}

/**
 * DEV-071：把一个步骤的旁白锚点说成老师看得懂的一行。没有语音合成，锚点按文字顺序，
 * 时长只是按字数估计；旧版（edu.lesson/1）修订没有锚点，提示保存一次修改即可生成。
 */
export function cueSummary(b: EditableBeat): string {
    const cues = b.cues ?? [];
    if (!cues.length) return "旧版讲解没有旁白锚点，保存任意修改后自动生成";
    const parts = cues.map((c, k) => {
        const acts = (c.actions ?? []).map((a) => (a.kind === "show_board_line" ? `板书第${(a.line ?? 0) + 1}行` : a.kind === "focus_figure" ? `图中标出${(a.refs ?? []).join("、")}` : "未知动作"));
        return `第${k + 1}句${acts.length ? "：" + acts.join("，") : ""}`;
    });
    const sec = Math.round(cues.reduce((t, c) => t + (c.est_ms ?? 0), 0) / 1000);
    return `旁白分 ${cues.length} 句（按文字顺序，约 ${sec} 秒，字数估计）；${parts.join("；")}`;
}
