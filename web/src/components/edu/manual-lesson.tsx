import { useState } from "react";

import { edu, newClientKey, type EduScope } from "@/services/api/edu";
import { teacherError } from "@/components/edu/teacher-errors";
import { buildManualLesson, emptyManualDraft, MANUAL_LIMITS, type ManualDraft } from "@/components/edu/manual-lesson-logic";

const ghost = "inline-flex h-8 items-center rounded-md border border-stone-300 px-2 text-sm disabled:opacity-40 dark:border-stone-600";
const btn = "inline-flex h-9 items-center rounded-md bg-stone-900 px-3 text-sm text-white disabled:opacity-40 dark:bg-stone-100 dark:text-stone-900";
const input = "w-full rounded-md border px-2 py-1 text-sm";

/** 人工编写讲解表单（DEV-039）：提交后返回任务号，生成的讲解修订照常进入人工审核。 */
export function ManualLessonForm({ scope, revisionId, onJob }: { scope: EduScope; revisionId: string; onJob: (jobId: string) => void }) {
    const [d, setD] = useState<ManualDraft>(emptyManualDraft);
    const [key] = useState(newClientKey);
    const [msg, setMsg] = useState("");
    const [busy, setBusy] = useState(false);
    const setBeat = (i: number, patch: Partial<ManualDraft["beats"][number]>) => setD({ ...d, beats: d.beats.map((b, j) => (j === i ? { ...b, ...patch } : b)) });
    const submit = async () => {
        const built = buildManualLesson(d);
        if (!built.ok) return setMsg(built.error);
        setBusy(true);
        const r = await edu<{ id: string }>("post", `/problem-revisions/${revisionId}/manual-lesson`, scope, { idempotencyKey: `${key}-${revisionId}`, lesson: built.lesson });
        setBusy(false);
        if (!r.ok) return setMsg(teacherError(r));
        setMsg("");
        onJob(r.data.id);
    };
    return (
        <div className="mt-2 space-y-2 text-sm">
            <p>这里写的内容不经过自动数学验证。生成的讲解会标明“未经自动数学验证”，学生作答由你人工批改，没有自测题，也不生成变式。</p>
            <input aria-label="讲解标题" className={input} maxLength={MANUAL_LIMITS.title} placeholder="讲解标题" value={d.title} onChange={(e) => setD({ ...d, title: e.target.value })} />
            <textarea aria-label="题面" className={input} rows={3} maxLength={MANUAL_LIMITS.problemText} placeholder="题面（学生可见）" value={d.problemText} onChange={(e) => setD({ ...d, problemText: e.target.value })} />
            {d.beats.map((b, i) => (
                <fieldset key={i} className="rounded-md border p-2">
                    <legend className="px-1">第 {i + 1} 步</legend>
                    <input aria-label={`第 ${i + 1} 步标题`} className={input} placeholder="步骤标题" value={b.title} onChange={(e) => setBeat(i, { title: e.target.value })} />
                    <textarea aria-label={`第 ${i + 1} 步板书`} className={`${input} mt-1`} rows={3} placeholder="板书，每行一句" value={b.lines} onChange={(e) => setBeat(i, { lines: e.target.value })} />
                    <label className="mt-1 flex items-center gap-1">
                        <input type="checkbox" checked={b.revealsAnswer} onChange={(e) => setBeat(i, { revealsAnswer: e.target.checked })} /> 这一步给出答案
                    </label>
                    {d.beats.length > 1 ? (
                        <button className={`${ghost} mt-1`} onClick={() => setD({ ...d, beats: d.beats.filter((_, j) => j !== i) })}>
                            删除这一步
                        </button>
                    ) : null}
                </fieldset>
            ))}
            <button className={ghost} disabled={d.beats.length >= MANUAL_LIMITS.beats} onClick={() => setD({ ...d, beats: [...d.beats, { title: "", lines: "", revealsAnswer: false }] })}>
                添加一步
            </button>
            <input aria-label="答案" className={input} maxLength={MANUAL_LIMITS.answer} placeholder="答案（学生在答案开放前看不到）" value={d.answer} onChange={(e) => setD({ ...d, answer: e.target.value })} />
            <textarea aria-label="分级提示" className={input} rows={3} placeholder="分级提示，每行一条，最多 5 条，不要写出答案" value={d.hints} onChange={(e) => setD({ ...d, hints: e.target.value })} />
            {msg ? (
                <p role="alert" className="rounded-md bg-amber-50 p-2 text-amber-900 dark:bg-amber-950 dark:text-amber-100">
                    {msg}
                </p>
            ) : null}
            <button className={btn} disabled={busy} onClick={() => void submit()}>
                生成人工讲解
            </button>
        </div>
    );
}
