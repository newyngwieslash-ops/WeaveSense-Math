import { useCallback, useEffect, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";
import { teacherError } from "@/components/edu/teacher-errors";
import { impactText, KIND_LABEL, parseOutcome, REQUESTER_LABEL, saveJson, STATUS_LABEL, textProblem, type DataRequest, type DataRequestImpact } from "@/components/edu/data-request-logic";

const box = "rounded-lg border border-stone-200 p-4 dark:border-stone-700";
const btn = "inline-flex h-8 items-center rounded-md bg-stone-900 px-3 text-sm text-white disabled:opacity-40 dark:bg-stone-100 dark:text-stone-900";
const ghost = "inline-flex h-8 items-center rounded-md border border-stone-300 px-2 text-sm disabled:opacity-40 dark:border-stone-600";
const input = "h-8 rounded-md border border-stone-300 bg-transparent px-2 text-sm dark:border-stone-600";

/** 数据请求（DEV-107）：登记，核验身份，预览影响，完成或拒绝；恢复备份后重放删除。 */
export function DataRequestsPanel({ scope, onMsg }: { scope: EduScope; onMsg: (m: string) => void }) {
    const [rows, setRows] = useState<DataRequest[]>([]);
    const [form, setForm] = useState({ studentUserId: "", kind: "access", requester: "guardian", reason: "" });
    const [refs, setRefs] = useState<Record<string, string>>({});
    const [preview, setPreview] = useState<Record<string, DataRequestImpact>>({});
    const [replay, setReplay] = useState<string>("");

    const load = useCallback(async () => {
        const r = await edu<DataRequest[]>("get", "/data-requests", scope);
        setRows(r.ok ? r.data : []);
    }, [scope]);
    useEffect(() => {
        void load();
    }, [load]);

    const create = async () => {
        if (!form.studentUserId.trim()) return onMsg("请填写学生用户编号");
        const bad = textProblem(form.reason, 200, "申请说明");
        if (bad) return onMsg(bad);
        const r = await edu("post", "/data-requests", scope, { ...form, studentUserId: form.studentUserId.trim(), reason: form.reason.trim() });
        onMsg(r.ok ? "已登记，请先核验申请人身份" : teacherError(r));
        void load();
    };
    const verify = async (id: string) => {
        const ref = refs[id] || "";
        const bad = textProblem(ref, 120, "核验记录编号");
        if (bad) return onMsg(bad);
        const r = await edu("post", `/data-requests/${id}/verify`, scope, { verificationRef: ref.trim() });
        onMsg(r.ok ? "身份核验已记录" : teacherError(r));
        void load();
    };
    const reject = async (id: string) => {
        const reason = refs[id] || "";
        const bad = textProblem(reason, 200, "拒绝理由");
        if (bad) return onMsg(bad);
        const r = await edu("post", `/data-requests/${id}/reject`, scope, { reason: reason.trim() });
        onMsg(r.ok ? "已拒绝并记录理由" : teacherError(r));
        void load();
    };
    const complete = async (row: DataRequest, confirm: boolean) => {
        const r = await edu<{ dryRun: boolean; impact?: DataRequestImpact; outcome?: DataRequestImpact; package?: unknown }>("post", `/data-requests/${row.id}/complete`, scope, { confirm });
        if (!r.ok) return onMsg(teacherError(r));
        if (r.data.dryRun && r.data.impact) {
            setPreview({ ...preview, [row.id]: r.data.impact });
            return onMsg("已计算影响，确认无误后再完成");
        }
        if (r.data.package) saveJson(`data-request-${row.id}`, r.data.package);
        onMsg(r.data.package ? "副本已下载；平台不保存副本内容" : "已完成并写入审计");
        void load();
    };
    const runReplay = async (confirm: boolean) => {
        const r = await edu<{ dryRun: boolean; markers: number; statesReverted: number; deleted: { learningRows: Record<string, number> } }>("post", "/data-requests/replay", scope, { confirm });
        if (!r.ok) return onMsg(teacherError(r));
        const n = Object.values(r.data.deleted.learningRows || {}).reduce((a, b) => a + b, 0);
        const line = `删除标记 ${r.data.markers} 个，恢复出来的身份或同意 ${r.data.statesReverted} 项，学习记录 ${n} 条`;
        setReplay(r.data.dryRun ? `预览：${line}` : "");
        onMsg(r.data.dryRun ? "已计算重放影响" : `已重放：${line}`);
    };

    return (
        <section className={box} aria-labelledby="org-datareq">
            <h2 id="org-datareq" className="mb-2 font-medium">
                数据请求
            </h2>
            <p className="mb-2 text-sm text-stone-500">登记学生或监护人的查阅、撤回或删除请求；先在线下核验申请人身份并记下核验单号，再预览影响并完成。每一步都写入审计。</p>
            <div className="flex flex-wrap gap-2">
                <input className={input} aria-label="学生用户编号" placeholder="学生用户编号" value={form.studentUserId} onChange={(e) => setForm({ ...form, studentUserId: e.target.value })} />
                <select className={input} aria-label="请求类型" value={form.kind} onChange={(e) => setForm({ ...form, kind: e.target.value })}>
                    {Object.entries(KIND_LABEL).map(([k, v]) => (
                        <option key={k} value={k}>
                            {v}
                        </option>
                    ))}
                </select>
                <select className={input} aria-label="申请人" value={form.requester} onChange={(e) => setForm({ ...form, requester: e.target.value })}>
                    {Object.entries(REQUESTER_LABEL).map(([k, v]) => (
                        <option key={k} value={k}>
                            {v}
                        </option>
                    ))}
                </select>
                <input className={`${input} flex-1`} aria-label="申请说明" placeholder="申请说明（范围与理由）" value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })} />
                <button className={btn} onClick={() => void create()}>
                    登记请求
                </button>
            </div>
            {rows.length === 0 ? (
                <p className="mt-2 text-sm text-stone-500">暂无数据请求。</p>
            ) : (
                <ul className="mt-2 space-y-2 text-sm">
                    {rows.map((r) => {
                        const done = parseOutcome(r);
                        const pv = preview[r.id];
                        return (
                            <li key={r.id}>
                                <div>
                                    {r.studentUserId}：{KIND_LABEL[r.kind] || r.kind}（{REQUESTER_LABEL[r.requester] || r.requester}），{STATUS_LABEL[r.status] || r.status}；{r.reason}
                                </div>
                                {done ? <div className="text-stone-500">{impactText(done)}</div> : null}
                                {r.status === "received" || r.status === "verified" ? (
                                    <div className="mt-1 flex flex-wrap gap-2">
                                        <input
                                            className={`${input} flex-1`}
                                            aria-label={`${r.studentUserId} 的核验单号或拒绝理由`}
                                            placeholder={r.status === "received" ? "核验单号（不要填写证件内容）或拒绝理由" : "拒绝理由"}
                                            value={refs[r.id] || ""}
                                            onChange={(e) => setRefs({ ...refs, [r.id]: e.target.value })}
                                        />
                                        {r.status === "received" ? (
                                            <button className={ghost} onClick={() => void verify(r.id)}>
                                                记录身份核验
                                            </button>
                                        ) : (
                                            <>
                                                <button className={ghost} onClick={() => void complete(r, false)}>
                                                    预览影响
                                                </button>
                                                <button className={btn} disabled={!pv} onClick={() => void complete(r, true)}>
                                                    完成请求
                                                </button>
                                            </>
                                        )}
                                        <button className={ghost} onClick={() => void reject(r.id)}>
                                            拒绝
                                        </button>
                                    </div>
                                ) : null}
                                {pv && r.status === "verified" ? <div className="mt-1">影响：{impactText(pv)}</div> : null}
                            </li>
                        );
                    })}
                </ul>
            )}
            <div className="mt-3 flex flex-wrap items-center gap-2 text-sm">
                <span className="flex-1 text-stone-500">从备份恢复后，重放已完成的删除请求，避免已删除的记录重新出现。</span>
                <button className={ghost} onClick={() => void runReplay(false)}>
                    预览重放
                </button>
                <button className={ghost} disabled={!replay} onClick={() => void runReplay(true)}>
                    执行重放
                </button>
            </div>
            {replay ? <p className="mt-1 text-sm">{replay}</p> : null}
        </section>
    );
}
