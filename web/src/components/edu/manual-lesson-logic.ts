/**
 * 人工编写讲解（DEV-039）。题目不在已实现题型内时（UNSUPPORTED_FAMILY，下一步 manual_authoring），
 * 老师自己写题面、步骤和答案。平台只检查格式，不做数学验证：讲解标记为“未经自动验证”，
 * 学生作答由老师人工批改，没有自测题，也不生成变式。限制与服务端一致。
 */
export const MANUAL_LIMITS = { title: 60, problemText: 600, beats: 12, beatTitle: 40, lines: 8, line: 200, answer: 80, hints: 5, hint: 200 } as const;

export const MANUAL_TEMPLATE_FAMILY = "OTHER-MANUAL";
export const MANUAL_UNVERIFIED = "manual_unverified";
export const MANUAL_NOTICE_TEACHER = "这是老师人工编写的讲解，未经自动数学验证。学生作答由老师人工批改，没有自测题，也不生成变式。";
export const MANUAL_NOTICE_STUDENT = "本讲解由老师人工编写，未经自动数学验证。你的作答由老师批改，本题没有自测。";

export type ManualBeatDraft = { title: string; lines: string; revealsAnswer: boolean };
export type ManualDraft = { title: string; problemText: string; beats: ManualBeatDraft[]; answer: string; hints: string };
export type ManualLesson = {
    title: string;
    problem_text: string;
    beats: { title: string; lines: string[]; reveals_answer: boolean }[];
    answer: { plain: string };
    hints: string[];
};

export const emptyManualDraft = (): ManualDraft => ({
    title: "",
    problemText: "",
    beats: [
        { title: "", lines: "", revealsAnswer: false },
        { title: "", lines: "", revealsAnswer: true },
    ],
    answer: "",
    hints: "",
});

const splitLines = (s: string) =>
    s
        .split("\n")
        .map((x) => x.trim())
        .filter(Boolean);

// eslint-disable-next-line no-control-regex
const BAD = /[<>\u0000-\u0008\u000b-\u001f\u007f]/;

function field(v: string, limit: number, what: string): string | null {
    const t = v.trim();
    if (!t) return `请填写${what}`;
    if ([...t].length > limit) return `${what}不能超过 ${limit} 个字`;
    if (BAD.test(t)) return `${what}不能包含尖括号或控制字符`;
    return null;
}

/** 把表单草稿转成接口内容；有问题时返回中文说明和下一步，不发请求。 */
export function buildManualLesson(d: ManualDraft): { ok: true; lesson: ManualLesson } | { ok: false; error: string } {
    const L = MANUAL_LIMITS;
    const errs: (string | null)[] = [field(d.title, L.title, "标题"), field(d.problemText, L.problemText, "题面"), field(d.answer, L.answer, "答案")];
    if (d.beats.length < 1 || d.beats.length > L.beats) errs.push(`讲解步骤需要 1 到 ${L.beats} 步`);
    const beats = d.beats.map((b, i) => {
        const lines = splitLines(b.lines);
        errs.push(field(b.title, L.beatTitle, `第 ${i + 1} 步标题`));
        if (lines.length < 1 || lines.length > L.lines) errs.push(`第 ${i + 1} 步需要 1 到 ${L.lines} 行板书`);
        lines.forEach((l, j) => errs.push(field(l, L.line, `第 ${i + 1} 步第 ${j + 1} 行`)));
        return { title: b.title.trim(), lines, reveals_answer: b.revealsAnswer };
    });
    if (!beats.some((b) => b.reveals_answer)) errs.push("请标出给出答案的那一步（学生在答案开放前看不到它）");
    const hints = splitLines(d.hints);
    if (hints.length > L.hints) errs.push(`提示最多 ${L.hints} 条`);
    hints.forEach((h, i) => errs.push(field(h, L.hint, `第 ${i + 1} 条提示`)));
    const first = errs.find((e): e is string => !!e);
    if (first) return { ok: false, error: `${first}。下一步：修改后再提交` };
    return { ok: true, lesson: { title: d.title.trim(), problem_text: d.problemText.trim(), beats, answer: { plain: d.answer.trim() }, hints } };
}

export const isManualLesson = (meta?: { verification?: string } | null) => meta?.verification === MANUAL_UNVERIFIED;
