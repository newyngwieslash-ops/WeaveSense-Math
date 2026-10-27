import { useEffect, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";

import { RenderCheckPanel } from "./render-check-panel";
import type { RenderCheck } from "./render-check-logic";
import { canApprove, canReject, ITEM_LABEL, parseFindings, REVIEW_ITEMS, targetLabel, VERDICT_LABEL, WHOLE_TARGETS, type Checklist, type Finding, type ReviewItem, type Verdict } from "./review-checklist-logic";

/**
 * 教师端审核清单（DEV-076）：逐项选择通过、有问题或不适用；退回可定位到步骤、提示或整份讲解的某一部分。
 * 审核绑定当前讲解哈希；上一轮退回的定位意见在此列出，作者据此修改并生成新修订。
 */
const btn = "inline-flex h-9 items-center rounded-md bg-stone-900 px-3 text-sm text-white disabled:opacity-40 dark:bg-stone-100 dark:text-stone-900";
const ghost = "inline-flex h-9 items-center rounded-md border border-stone-300 px-3 text-sm disabled:opacity-40 dark:border-stone-600";
const sel = "rounded-md border border-stone-300 bg-transparent px-1 py-1 text-sm dark:border-stone-600";

type ReviewRecord = { id: string; decision: string; comment: string; findings?: string; createdAt: string; lessonHash: string };
type Lesson = { id: string; lessonHash: string; state: string; blockingErrors: string };

export function ReviewChecklist({ scope, lesson, onDone }: { scope: EduScope; lesson: Lesson; onDone: (decision: "approved" | "rejected") => void }) {
    const [beats, setBeats] = useState<{ id: string; title: string }[]>([]);
    const [hints, setHints] = useState<number[]>([]);
    const [reviews, setReviews] = useState<ReviewRecord[]>([]);
    const [renderCheck, setRenderCheck] = useState<RenderCheck | null | undefined>(undefined);
    const [checklist, setChecklist] = useState<Checklist>({});
    const [findings, setFindings] = useState<Finding[]>([]);
    const [draft, setDraft] = useState<Finding>({ target: "lesson", item: "method", note: "" });
    const [comment, setComment] = useState("");
    const [msg, setMsg] = useState("");

    useEffect(() => {
        void edu<{ beats: { id: string; title: string }[]; hints: { level: number }[] }>("get", `/lesson-revisions/${lesson.id}/editable`, scope).then((r) => {
            if (!r.ok) return;
            setBeats(r.data.beats || []);
            setHints((r.data.hints || []).map((h) => h.level));
        });
        void edu<{ reviews: ReviewRecord[]; renderCheck?: RenderCheck | null }>("get", `/lesson-revisions/${lesson.id}`, scope).then((r) => {
            if (!r.ok) return;
            setReviews(r.data.reviews || []);
            setRenderCheck(r.data.renderCheck ?? null);
        });
    }, [lesson.id, scope]);

    const submit = async (decision: "approved" | "rejected") => {
        const r = await edu<{ id: string }>("post", `/lesson-revisions/${lesson.id}/review`, scope, { lessonHash: lesson.lessonHash, decision, comment, checklist, findings });
        if (!r.ok) return setMsg(`${r.code}：${r.message}`);
        setMsg("");
        onDone(decision);
    };
    const pending = lesson.state === "ready_for_review";
    const past = reviews.filter((r) => r.decision === "rejected" && parseFindings(r.findings).length > 0);

    return (
        <div className="mt-2 space-y-2 text-sm">
            {renderCheck !== undefined ? <RenderCheckPanel check={renderCheck} /> : null}
            {past.map((r) => (
                <div key={r.id} className="rounded-md border border-amber-300 p-2 dark:border-amber-700">
                    <p className="font-medium">
                        上一轮退回（{new Date(r.createdAt).toLocaleString()}）{r.comment ? `：${r.comment}` : ""}
                    </p>
                    <ul className="ml-4 list-disc">
                        {parseFindings(r.findings).map((f, i) => (
                            <li key={i}>
                                {targetLabel(f.target, beats)}，{ITEM_LABEL[f.item] || f.item}：{f.note}
                            </li>
                        ))}
                    </ul>
                </div>
            ))}
            {pending ? (
                <>
                    <table className="w-full">
                        <tbody>
                            {REVIEW_ITEMS.map((k) => (
                                <tr key={k}>
                                    <td className="py-1 pr-2">{ITEM_LABEL[k]}</td>
                                    <td>
                                        {(["pass", "issue", "na"] as Verdict[]).map((v) => (
                                            <label key={v} className="mr-3">
                                                <input type="radio" name={`ck-${k}`} checked={checklist[k] === v} onChange={() => setChecklist({ ...checklist, [k]: v })} /> {VERDICT_LABEL[v]}
                                            </label>
                                        ))}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                    <div className="flex flex-wrap items-center gap-2">
                        <select aria-label="定位" className={sel} value={draft.target} onChange={(e) => setDraft({ ...draft, target: e.target.value })}>
                            {Object.entries(WHOLE_TARGETS).map(([v, l]) => (
                                <option key={v} value={v}>
                                    {l}
                                </option>
                            ))}
                            {beats.map((b, i) => (
                                <option key={b.id} value={`beat:${b.id}`}>
                                    第 {i + 1} 步 {b.title}
                                </option>
                            ))}
                            {hints.map((h) => (
                                <option key={h} value={`hint:${h}`}>
                                    提示 {h}
                                </option>
                            ))}
                        </select>
                        <select aria-label="清单项" className={sel} value={draft.item} onChange={(e) => setDraft({ ...draft, item: e.target.value as ReviewItem })}>
                            {REVIEW_ITEMS.map((k) => (
                                <option key={k} value={k}>
                                    {ITEM_LABEL[k]}
                                </option>
                            ))}
                        </select>
                        <input aria-label="问题说明" className={`${sel} min-w-48 flex-1`} maxLength={300} value={draft.note} onChange={(e) => setDraft({ ...draft, note: e.target.value })} placeholder="问题说明" />
                        <button
                            className={ghost}
                            disabled={!draft.note.trim() || findings.length >= 30}
                            onClick={() => {
                                setFindings([...findings, { ...draft, note: draft.note.trim() }]);
                                setDraft({ ...draft, note: "" });
                            }}
                        >
                            添加定位意见
                        </button>
                    </div>
                    {findings.length ? (
                        <ul className="ml-4 list-disc">
                            {findings.map((f, i) => (
                                <li key={i}>
                                    {targetLabel(f.target, beats)}，{ITEM_LABEL[f.item]}：{f.note}{" "}
                                    <button className="underline" onClick={() => setFindings(findings.filter((_, j) => j !== i))}>
                                        移除
                                    </button>
                                </li>
                            ))}
                        </ul>
                    ) : null}
                    <textarea className="w-full rounded-md border p-2 text-sm" rows={2} placeholder="审核意见" value={comment} onChange={(e) => setComment(e.target.value)} />
                    <div className="flex gap-2">
                        <button className={btn} disabled={!canApprove(checklist, findings, lesson.blockingErrors)} onClick={() => void submit("approved")}>
                            审核通过
                        </button>
                        <button className={ghost} disabled={!canReject(checklist, findings, comment)} onClick={() => void submit("rejected")}>
                            退回
                        </button>
                    </div>
                    <p className="text-xs text-stone-500">通过需逐项选择且没有“有问题”项或定位意见；退回需至少一条定位意见、一个“有问题”项或一段意见。</p>
                </>
            ) : null}
            {msg ? <p className="text-red-600">{msg}</p> : null}
        </div>
    );
}
