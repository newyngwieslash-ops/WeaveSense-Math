import { useCallback, useEffect, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";

import { blockingLabel, dropReasonLabel, gradingScopeLabel, parseBlocking, replaceParamsError, selfTestPreview, VARIANT_STATE_LABEL, variantApprovable, variantShortfall, type VariantItem, type VariantSet } from "./edu-admin-logic";
import { SetSelfTest } from "./set-self-test";
import { teacherError, jobErrorText } from "./teacher-errors";

/**
 * 教师端：为已生成的讲解修订生成“举一反三”变式题组，逐题独立验证后审核（AC-22）。
 * 审核绑定题组哈希；未审核通过的题组不能布置。答案只在教师视图中显示。
 */
const btn = "inline-flex h-8 items-center rounded-md bg-stone-900 px-3 text-sm text-white disabled:opacity-40 dark:bg-stone-100 dark:text-stone-900";
const ghost = "inline-flex h-8 items-center rounded-md border border-stone-300 px-2 text-sm disabled:opacity-40 dark:border-stone-600";
const TERMINAL = new Set(["succeeded", "failed", "cancelled"]);

type SourceRow = { id: string; title: string; state: string; allowStudent: boolean; allowDerivative: boolean; expiresAt?: string | null };

/** 教师用自有数字替换一题（DEV-078）：需要选择记录了学生使用与改编权利的来源，结果是待审核的新题组。 */
function ReplaceItemForm({ scope, set, item, onStarted }: { scope: EduScope; set: VariantSet; item: VariantItem; onStarted: (j: JobRow) => void }) {
    const names = Object.keys(item.params);
    const [params, setParams] = useState<Record<string, string>>({});
    const [sources, setSources] = useState<SourceRow[]>([]);
    const [source, setSource] = useState("");
    const [note, setNote] = useState("");
    const [err, setErr] = useState("");
    useEffect(() => {
        void edu<SourceRow[]>("get", "/source-assets", scope).then((r) => {
            if (!r.ok) return;
            const now = Date.now();
            setSources(r.data.filter((s) => s.state === "active" && s.allowStudent && s.allowDerivative && (!s.expiresAt || Date.parse(s.expiresAt) > now)));
        });
    }, [scope]);
    const submit = async () => {
        const e = replaceParamsError(names, params);
        if (e) return setErr(e);
        if (!source) return setErr("请选择记录了学生使用与改编权利的来源");
        const r = await edu<JobRow>("post", `/variant-sets/${set.id}/replace`, scope, {
            itemId: item.id,
            params: Object.fromEntries(names.map((n) => [n, (params[n] || "").trim()])),
            sourceAssetId: source,
            note,
            idempotencyKey: crypto.randomUUID(),
        });
        if (!r.ok) return setErr(r.code === "RIGHTS_RESTRICTED" ? "来源权利不允许学生使用或改编（已过期、已撤回或未授予）" : teacherError(r));
        setErr("");
        onStarted(r.data);
    };
    const id = `rp-${set.id}-${item.id}`;
    return (
        <fieldset className="mt-1 rounded-md border border-dashed p-2" aria-describedby={`${id}-hint`}>
            <legend className="px-1 text-xs font-medium">用自有题替换第 {item.id} 题</legend>
            <p id={`${id}-hint`} className="text-xs text-stone-500">
                同一题族只改数字；题面按题族模板生成，每题重新独立验证。替换生成新题组，需重新审核后才能布置，原题组不变。
            </p>
            <div className="mt-1 flex flex-wrap items-center gap-2">
                {names.map((n) => (
                    <label key={n} htmlFor={`${id}-${n}`} className="text-xs">
                        {n} <input id={`${id}-${n}`} className="h-7 w-20 rounded-md border px-1" value={params[n] || ""} placeholder={item.params[n]} onChange={(e) => setParams({ ...params, [n]: e.target.value })} />
                    </label>
                ))}
                <label htmlFor={`${id}-src`} className="text-xs">
                    来源与权利{" "}
                    <select id={`${id}-src`} className="h-7 rounded-md border px-1" value={source} onChange={(e) => setSource(e.target.value)}>
                        <option value="">请选择</option>
                        {sources.map((s) => (
                            <option key={s.id} value={s.id}>
                                {s.title}
                            </option>
                        ))}
                    </select>
                </label>
                <label htmlFor={`${id}-note`} className="text-xs">
                    说明 <input id={`${id}-note`} className="h-7 w-40 rounded-md border px-1" maxLength={200} value={note} onChange={(e) => setNote(e.target.value)} />
                </label>
                <button type="button" className={ghost} onClick={() => void submit()}>
                    提交替换
                </button>
            </div>
            {sources.length === 0 ? <p className="mt-1 text-xs text-amber-700">本工作区没有允许学生使用与改编的有效来源，请先在来源管理中登记。</p> : null}
            {err ? (
                <p role="alert" className="mt-1 text-xs text-amber-700">
                    {err}
                </p>
            ) : null}
        </fieldset>
    );
}

type JobRow = { id: string; status: string; stage: string; error?: { code?: string; message?: string } | null; variantSetId?: string };

export function VariantSetPanel({ scope, lessonId, onPick, picked }: { scope: EduScope; lessonId: string; onPick: (id: string) => void; picked: string }) {
    const [count, setCount] = useState(3);
    const [sets, setSets] = useState<VariantSet[]>([]);
    const [open, setOpen] = useState<VariantSet | null>(null);
    const [job, setJob] = useState<JobRow | null>(null);
    const [comment, setComment] = useState("");
    const [studentView, setStudentView] = useState(false);
    const [msg, setMsg] = useState("");
    const [replacing, setReplacing] = useState("");

    const reload = useCallback(() => {
        if (!lessonId) return;
        void edu<VariantSet[]>("get", `/lesson-revisions/${lessonId}/variant-sets`, scope).then((r) => r.ok && setSets(r.data));
    }, [lessonId, scope]);
    useEffect(reload, [reload]);

    useEffect(() => {
        if (!job || TERMINAL.has(job.status)) return;
        const t = window.setTimeout(() => {
            void edu<JobRow>("get", `/jobs/${job.id}`, scope).then((r) => {
                if (!r.ok) return;
                setJob(r.data);
                if (TERMINAL.has(r.data.status)) reload();
            });
        }, 1500);
        return () => window.clearTimeout(t);
    }, [job, scope, reload]);

    const generate = async () => {
        const r = await edu<JobRow>("post", `/lesson-revisions/${lessonId}/variant-sets`, scope, { count, idempotencyKey: crypto.randomUUID() });
        if (!r.ok) return setMsg(teacherError(r));
        setMsg("");
        setJob(r.data);
    };
    const show = async (id: string) => {
        const r = await edu<VariantSet>("get", `/variant-sets/${id}`, scope);
        if (r.ok) setOpen(r.data);
        else setMsg(teacherError(r));
    };
    const review = async (decision: "approved" | "rejected") => {
        if (!open) return;
        const r = await edu("post", `/variant-sets/${open.id}/review`, scope, { setHash: open.setHash, decision, comment });
        if (!r.ok) return setMsg(r.code === "POLICY_REVOKED" ? "按机构审核策略，需要其他老师审核" : teacherError(r));
        setComment("");
        await show(open.id);
        reload();
    };

    return (
        <div className="text-sm">
            <div className="flex flex-wrap items-center gap-2">
                <label>
                    题数{" "}
                    <select className="h-8 rounded-md border px-2" value={count} onChange={(e) => setCount(Number(e.target.value))}>
                        {[1, 2, 3, 4, 5].map((n) => (
                            <option key={n} value={n}>
                                {n}
                            </option>
                        ))}
                    </select>
                </label>
                <button className={btn} disabled={!lessonId || (!!job && !TERMINAL.has(job.status))} onClick={() => void generate()}>
                    生成变式题组
                </button>
                {job ? (
                    <span>
                        任务 {job.id.slice(-8)}：{job.status}
                        {job.error?.code ? `（${jobErrorText(job.error)}）` : ""}
                    </span>
                ) : null}
            </div>
            <p className="mt-1 text-xs text-stone-500">变式只改数字，属于近迁移；每题独立求解与验证，答案与原题或彼此重复的会被剔除，不足请求数不补凑。</p>
            {msg ? (
                <p role="status" aria-live="polite" className="mt-1 text-amber-700">
                    {msg}
                </p>
            ) : null}
            <ul className="mt-2 space-y-1">
                {sets.map((s) => (
                    <li key={s.id} className="flex flex-wrap items-center gap-2">
                        <span>
                            题组 {s.id.slice(-6)}：{s.itemCount} 题，{VARIANT_STATE_LABEL[s.state] || s.state}
                        </span>
                        <button className={ghost} onClick={() => void show(s.id)}>
                            查看与审核
                        </button>
                        {s.state === "approved" ? (
                            <button className={ghost} disabled={picked === s.id} onClick={() => onPick(s.id)}>
                                {picked === s.id ? "已选为自测题组" : "用作自测题组"}
                            </button>
                        ) : null}
                    </li>
                ))}
            </ul>
            {open ? (
                <div className="mt-3 rounded-md border p-3" data-testid="variant-review">
                    <p>
                        题组哈希 {open.setHash.slice(0, 18)}；{VARIANT_STATE_LABEL[open.state] || open.state}；审核策略 {open.reviewMode || "未知"}
                    </p>
                    {variantShortfall(open) ? <p className="text-amber-700">{variantShortfall(open)}</p> : null}
                    {open.supersedesSetId ? <p className="text-xs text-stone-500">由题组 {open.supersedesSetId.slice(-6)} 替换一题生成；原题组保持不变。</p> : null}
                    {open.blockingErrors ? <p className="text-red-700">阻断错误：{parseBlocking(open.blockingErrors).map(blockingLabel).join("；")}，不可通过</p> : null}
                    <ol className="mt-2 list-decimal space-y-2 pl-5">
                        {open.items.map((it) => (
                            <li key={it.id}>
                                <p className="whitespace-pre-wrap">{it.prompt}</p>
                                <p className="text-xs text-stone-500">
                                    参数{" "}
                                    {Object.entries(it.params)
                                        .map(([k, v]) => `${k}=${v}`)
                                        .join("，")}
                                    ；数学状态 {it.math_state}；证据 {(it.evidence_levels || []).join("、")}；{it.transfer === "near" ? "近迁移（仅改数字）" : it.transfer}
                                </p>
                                {it.source === "teacher_own" ? <p className="text-xs">教师自有题（已记录来源权利）</p> : null}
                                <p className="text-xs text-stone-500">{gradingScopeLabel(open.grading?.[it.id])}</p>
                                {open.answerKeys?.[it.id] ? <p className="text-xs">参考答案（仅教师可见）：{open.answerKeys[it.id].plain}</p> : null}
                                {!open.blockingErrors ? (
                                    replacing === it.id ? (
                                        <ReplaceItemForm
                                            scope={scope}
                                            set={open}
                                            item={it}
                                            onStarted={(j) => {
                                                setReplacing("");
                                                setJob(j);
                                            }}
                                        />
                                    ) : (
                                        <button type="button" className={`${ghost} mt-1`} onClick={() => setReplacing(it.id)}>
                                            用自有题替换
                                        </button>
                                    )
                                ) : null}
                            </li>
                        ))}
                    </ol>
                    <details className="mt-2" open={studentView} onToggle={(e) => setStudentView((e.target as HTMLDetailsElement).open)}>
                        <summary className="cursor-pointer text-xs font-medium">自测视角预览（学生看到的样子，不含参考答案，提交已禁用）</summary>
                        {studentView ? (
                            <div className="mt-2 rounded-md border border-dashed p-2" data-testid="self-test-preview">
                                <SetSelfTest selfTest={selfTestPreview(open)} attempts={[]} busy onSubmit={() => undefined} />
                            </div>
                        ) : null}
                    </details>
                    {open.dropped?.length ? <p className="mt-2 text-xs text-stone-500">已剔除：{open.dropped.map((d) => dropReasonLabel(d.reason)).join("；")}</p> : null}
                    {open.reviews?.length ? (
                        <p className="mt-1 text-xs">
                            审核记录：
                            {open.reviews.map((r) => `${r.decision === "approved" ? "通过" : "退回"}${r.selfReview ? "（本人审核）" : ""}${r.comment ? "：" + r.comment : ""}`).join("；")}
                        </p>
                    ) : null}
                    {open.state === "ready_for_review" ? (
                        <>
                            <textarea className="mt-2 w-full rounded-md border p-2" rows={2} placeholder="审核意见" value={comment} onChange={(e) => setComment(e.target.value)} />
                            <div className="mt-1 flex gap-2">
                                <button className={btn} disabled={!variantApprovable(open)} onClick={() => void review("approved")}>
                                    审核通过
                                </button>
                                <button className={ghost} onClick={() => void review("rejected")}>
                                    退回
                                </button>
                            </div>
                        </>
                    ) : null}
                </div>
            ) : null}
        </div>
    );
}
