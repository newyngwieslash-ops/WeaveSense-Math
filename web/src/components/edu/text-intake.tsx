import { useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";
import { teacherError } from "@/components/edu/teacher-errors";
import { draftLines, INTAKE_MAX, intakeNextStep, intakeReason, intakeTextProblem, type IntakeDraft } from "@/components/edu/text-intake-logic";

const ghost = "inline-flex h-9 items-center rounded-md border border-stone-300 px-3 text-sm disabled:opacity-40 dark:border-stone-600";

/** 粘贴题目文字 → 结构化草稿（DEV-031）。草稿只填入下方表单，不保存、不扣费，仍需老师核对并确认题意。 */
export function TextIntake({ scope, disabled, onDraft, onManual }: { scope: EduScope; disabled?: boolean; onDraft: (d: IntakeDraft) => void; onManual: () => void }) {
    const [text, setText] = useState("");
    const [busy, setBusy] = useState(false);
    const [draft, setDraft] = useState<IntakeDraft | null>(null);
    const [err, setErr] = useState<{ line: string; reason: string; next: string; manual: boolean } | null>(null);

    const run = async () => {
        const local = intakeTextProblem(text);
        setDraft(null);
        if (local) return setErr({ line: local, reason: "", next: "", manual: false });
        setErr(null);
        setBusy(true);
        const r = await edu<IntakeDraft>("post", "/problem-drafts/normalize", scope, { text: text.trim() });
        setBusy(false);
        if (!r.ok) return setErr({ line: teacherError(r), reason: intakeReason(r.detail), next: intakeNextStep(r.code), manual: r.code === "UNSUPPORTED_FAMILY" });
        setDraft(r.data);
        onDraft(r.data);
    };

    return (
        <details className="mt-2" open>
            <summary className="cursor-pointer text-sm font-medium">从题目文字识别（可选）</summary>
            <textarea
                aria-label="题目文字"
                className="mt-2 w-full rounded-md border p-2 text-sm"
                rows={3}
                maxLength={INTAKE_MAX}
                placeholder="粘贴题目文字，例如：正方体 ABCD-A1B1C1D1 的棱长为 2，求直线 A1C 与底面 ABCD 所成角的正弦值。"
                value={text}
                disabled={disabled}
                onChange={(e) => setText(e.target.value)}
            />
            <div className="mt-1 flex items-center gap-2">
                <button className={ghost} disabled={disabled || busy} onClick={() => void run()}>
                    {busy ? "识别中…" : "识别为结构化题目"}
                </button>
                <span className="text-xs text-stone-500">只识别已支持题型的固定表述；暂不支持图片识别。识别结果不保存、不扣费。</span>
            </div>
            {draft ? (
                <div role="status" className="mt-2 rounded-md bg-stone-50 p-2 text-sm dark:bg-stone-900">
                    <p className="font-medium">识别结果是草稿，已填入下方表单。请逐项核对，保存后再确认题意。</p>
                    <ul className="mt-1 list-disc pl-5">
                        {draftLines(draft).map((l) => (
                            <li key={l}>{l}</li>
                        ))}
                    </ul>
                </div>
            ) : null}
            {err ? (
                <div role="alert" className="mt-2 rounded-md bg-amber-50 p-2 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-100">
                    <p>{err.line}</p>
                    {err.reason ? <p>原因：{err.reason}</p> : null}
                    {err.next ? <p>{err.next}</p> : null}
                    {err.manual ? (
                        <button className={`${ghost} mt-1`} onClick={onManual}>
                            改用人工编写讲解
                        </button>
                    ) : null}
                </div>
            ) : null}
        </details>
    );
}
