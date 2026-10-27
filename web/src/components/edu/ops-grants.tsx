import { useCallback, useEffect, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";
import { teacherError } from "@/components/edu/teacher-errors";
import { EFFECTIVE_LABEL, type OrgClass } from "@/components/edu/org-admin-logic";
import { OPS_HOURS_MAX, OPS_SCOPE_LABEL, opsGrantProblem, opsGrantText, opsRevokeProblem, type OpsGrantForm, type OpsGrantRow } from "@/components/edu/ops-grant-logic";

const box = "rounded-lg border border-stone-200 p-4 dark:border-stone-700";
const btn = "inline-flex h-8 items-center rounded-md bg-stone-900 px-3 text-sm text-white disabled:opacity-40 dark:bg-stone-100 dark:text-stone-900";
const ghost = "inline-flex h-8 items-center rounded-md border border-stone-300 px-2 text-sm disabled:opacity-40 dark:border-stone-600";
const input = "h-8 rounded-md border border-stone-300 bg-transparent px-2 text-sm dark:border-stone-600";

/** 运维临时授权（DEV-108）：运维默认看不到学校数据；管理员按目的、范围和期限授权，可随时撤销，每次查看都写入审计。 */
export function OpsGrantsPanel({ scope, classes, onMsg }: { scope: EduScope; classes: OrgClass[]; onMsg: (m: string) => void }) {
    const [rows, setRows] = useState<OpsGrantRow[]>([]);
    const [form, setForm] = useState<OpsGrantForm>({ operatorUserId: "", scope: "jobs", classId: "", includeContent: false, purpose: "", hours: 4 });
    const [reasons, setReasons] = useState<Record<string, string>>({});

    const load = useCallback(async () => {
        const r = await edu<OpsGrantRow[]>("get", "/ops-grants", scope);
        setRows(r.ok ? r.data : []);
    }, [scope]);
    useEffect(() => {
        void load();
    }, [load]);

    const grant = async () => {
        const bad = opsGrantProblem(form, "");
        if (bad) return onMsg(bad);
        const body = { ...form, operatorUserId: form.operatorUserId.trim(), purpose: form.purpose.trim(), classId: form.scope === "class" ? form.classId : "", includeContent: form.scope === "class" && form.includeContent };
        const r = await edu("post", "/ops-grants", scope, body);
        onMsg(r.ok ? "已授权；把授权编号告诉运维人员，每次查看都会写入访问记录" : teacherError(r));
        void load();
    };
    const revoke = async (id: string) => {
        const reason = reasons[id] || "";
        const bad = opsRevokeProblem(reason);
        if (bad) return onMsg(bad);
        const r = await edu("post", `/ops-grants/${id}/revoke`, scope, { reason: reason.trim() });
        onMsg(r.ok ? "已撤销，运维人员立即无法再查看" : teacherError(r));
        void load();
    };
    const className = (id?: string) => classes.find((k) => k.id === id)?.name;

    return (
        <section className={box} aria-labelledby="org-ops">
            <h2 id="org-ops" className="mb-2 font-medium">
                运维临时授权
            </h2>
            <p className="mb-2 text-sm text-stone-500">运维人员默认看不到本校任何数据。排查问题时由你写明目的，选择范围和期限（最长 {OPS_HOURS_MAX} 小时）；到期或撤销后立即失效，每次查看都记入访问记录。</p>
            <div className="flex flex-wrap gap-2">
                <input className={input} aria-label="运维人员用户编号" placeholder="运维人员用户编号" value={form.operatorUserId} onChange={(e) => setForm({ ...form, operatorUserId: e.target.value })} />
                <select className={input} aria-label="授权范围" value={form.scope} onChange={(e) => setForm({ ...form, scope: e.target.value, includeContent: false })}>
                    {Object.entries(OPS_SCOPE_LABEL).map(([k, v]) => (
                        <option key={k} value={k}>
                            {v}
                        </option>
                    ))}
                </select>
                {form.scope === "class" ? (
                    <>
                        <select className={input} aria-label="班级" value={form.classId} onChange={(e) => setForm({ ...form, classId: e.target.value })}>
                            <option value="">选择班级</option>
                            {classes.map((k) => (
                                <option key={k.id} value={k.id}>
                                    {k.name}
                                </option>
                            ))}
                        </select>
                        <label className="flex items-center gap-1 text-sm">
                            <input type="checkbox" checked={form.includeContent} onChange={(e) => setForm({ ...form, includeContent: e.target.checked })} />
                            包含作答内容
                        </label>
                    </>
                ) : null}
                <label className="flex items-center gap-1 text-sm">
                    有效
                    <input className={`${input} w-16`} type="number" min={1} max={OPS_HOURS_MAX} aria-label="有效小时数" value={form.hours} onChange={(e) => setForm({ ...form, hours: Number(e.target.value) })} />
                    小时
                </label>
                <input className={`${input} min-w-64 flex-1`} aria-label="授权目的" placeholder="授权目的，例如工单号和要排查的问题" value={form.purpose} onChange={(e) => setForm({ ...form, purpose: e.target.value })} />
                <button type="button" className={btn} onClick={() => void grant()}>
                    授权
                </button>
            </div>
            {rows.length === 0 ? (
                <p className="mt-3 text-sm text-stone-500">还没有授权记录。</p>
            ) : (
                <ul className="mt-3 space-y-2 text-sm">
                    {rows.map(({ grant: g, effective }) => (
                        <li key={g.id} className="rounded-md border border-stone-200 p-2 dark:border-stone-700">
                            <div>
                                <span className="font-medium">{EFFECTIVE_LABEL[effective] || effective}</span> · 授权编号 {g.id}
                            </div>
                            <div>{opsGrantText(g, className(g.classId))}</div>
                            {g.revokeReason ? <div className="text-stone-500">撤销理由：{g.revokeReason}</div> : null}
                            {effective === "active" ? (
                                <div className="mt-1 flex flex-wrap gap-2">
                                    <input className={input} aria-label={`撤销 ${g.id} 的理由`} placeholder="撤销理由" value={reasons[g.id] || ""} onChange={(e) => setReasons({ ...reasons, [g.id]: e.target.value })} />
                                    <button type="button" className={ghost} onClick={() => void revoke(g.id)}>
                                        撤销
                                    </button>
                                </div>
                            ) : null}
                        </li>
                    ))}
                </ul>
            )}
        </section>
    );
}
