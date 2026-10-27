import { useCallback, useEffect, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";
import { teacherError } from "@/components/edu/teacher-errors";
import { DataRequestsPanel } from "@/components/edu/data-requests";
import { OpsGrantsPanel } from "@/components/edu/ops-grants";
import {
    accessRowText,
    CONSENT_LABEL,
    consentLine,
    EFFECTIVE_LABEL,
    INHERIT_LABEL,
    purposeProblem,
    PURPOSE_MAX,
    regionLine,
    RETENTION_LABEL,
    retentionImpact,
    retentionLine,
    retentionProblem,
    REVIEW_MODE_LABEL,
    ROLE_LABEL,
    staffLine,
    type AccessRow,
    type OrgOverview,
    type RetentionDays,
    type RetentionSummary,
    type RetentionView,
    type RoleRow,
} from "@/components/edu/org-admin-logic";

const box = "rounded-lg border border-stone-200 p-4 dark:border-stone-700";
const btn = "inline-flex h-8 items-center rounded-md bg-stone-900 px-3 text-sm text-white disabled:opacity-40 dark:bg-stone-100 dark:text-stone-900";
const ghost = "inline-flex h-8 items-center rounded-md border border-stone-300 px-2 text-sm disabled:opacity-40 dark:border-stone-600";
const input = "h-8 rounded-md border border-stone-300 bg-transparent px-2 text-sm dark:border-stone-600";

type RosterRow = { enrollmentId: string; studentUserId: string; status: string; consentOk: boolean; externalRef?: string; displayName?: string };

/** 机构管理（DEV-102）：汇总概览、人员与角色、审核与同意策略、地域与保留状态、个体记录访问（需说明目的）与访问记录。 */
export function OrgAdminPanel({ scope, onMsg }: { scope: EduScope; onMsg: (m: string) => void }) {
    const [ov, setOv] = useState<OrgOverview | null>(null);
    const [denied, setDenied] = useState(false);
    const [roles, setRoles] = useState<RoleRow[]>([]);
    const [log, setLog] = useState<AccessRow[]>([]);
    const [bind, setBind] = useState({ userId: "", role: "teacher" });
    const [revokeReason, setRevokeReason] = useState("");
    const [policy, setPolicy] = useState({ reviewMode: "self_allowed", consentMode: "guardian_required", editReviewInherit: "none", reason: "" });
    const [look, setLook] = useState({ classId: "", purpose: "" });
    const [roster, setRoster] = useState<RosterRow[] | null>(null);
    const [ret, setRet] = useState<RetentionView | null>(null);
    const [retDays, setRetDays] = useState<RetentionDays | null>(null);
    const [retReason, setRetReason] = useState("");
    const [retDry, setRetDry] = useState<RetentionSummary | null>(null);

    const load = useCallback(async () => {
        if (!scope.workspace) return;
        const r = await edu<OrgOverview>("get", "/org/overview", scope);
        if (!r.ok) {
            setDenied(r.status === 403);
            setOv(null);
            return;
        }
        setDenied(false);
        setOv(r.data);
        setPolicy((p) => ({ ...p, reviewMode: r.data.policy.reviewMode, consentMode: r.data.policy.consentMode, editReviewInherit: r.data.policy.editReviewInherit }));
        const [rr, lr, rt] = await Promise.all([edu<RoleRow[]>("get", `/roles?tenantId=${encodeURIComponent(r.data.tenantId)}`, scope), edu<AccessRow[]>("get", "/org/access-log", scope), edu<RetentionView>("get", "/retention", scope)]);
        setRoles(rr.ok ? rr.data : []);
        setLog(lr.ok ? lr.data : []);
        setRet(rt.ok ? rt.data : null);
        if (rt.ok) {
            const p = rt.data.policy;
            setRetDays({ tempDays: p.tempDays, imageDays: p.imageDays, learningDays: p.learningDays, backupDays: p.backupDays });
        }
    }, [scope]);
    useEffect(() => {
        void load();
    }, [load]);

    if (denied) return <p className={box}>只有机构所有者、管理员或教育管理员可以查看机构概览。</p>;
    if (!ov) return null;

    const bindRole = async () => {
        if (!bind.userId.trim()) return onMsg("请填写要授权的用户编号");
        const r = await edu("post", "/roles", scope, { tenantId: ov.tenantId, userId: bind.userId.trim(), role: bind.role });
        onMsg(r.ok ? `已授予${ROLE_LABEL[bind.role]}角色` : teacherError(r));
        void load();
    };
    const revoke = async (row: RoleRow) => {
        if (!revokeReason.trim()) return onMsg("撤销角色前请填写撤销理由");
        const r = await edu("post", "/roles/revoke", scope, { tenantId: ov.tenantId, userId: row.binding.userId, role: row.binding.role, reason: revokeReason.trim() });
        onMsg(r.ok ? "已撤销，相关权限立即失效" : teacherError(r));
        void load();
    };
    const confirmRetention = async () => {
        if (!ret || !retDays) return;
        const bad = retentionProblem(retDays, ret.bounds, retReason);
        if (bad) return onMsg(bad);
        const r = await edu("put", "/retention", scope, { ...retDays, reason: retReason.trim() });
        onMsg(r.ok ? "保留期已确认并写入审计；此后按期自动清理" : teacherError(r));
        setRetDry(null);
        void load();
    };
    const runRetention = async (confirm: boolean) => {
        const r = await edu<{ dryRun: boolean; summary: RetentionSummary }>("post", "/retention/run", scope, { confirm });
        if (!r.ok) return onMsg(teacherError(r));
        if (r.data.dryRun) {
            setRetDry(r.data.summary);
            return onMsg("已计算影响范围，确认无误后再执行清理");
        }
        setRetDry(null);
        onMsg(`清理完成：${retentionImpact(r.data.summary)}`);
        void load();
    };
    const savePolicy = async () => {
        if (policy.consentMode === "not_applicable" && policy.reason.trim().length < 4) return onMsg("把同意设为不适用时必须写明理由");
        const r = await edu("put", "/policy", scope, policy);
        onMsg(r.ok ? "策略已保存并写入审计" : teacherError(r));
        void load();
    };
    const viewRoster = async () => {
        setRoster(null);
        if (!look.classId) return onMsg("请先选择班级");
        const cls = ov.classes.find((k) => k.id === look.classId);
        if (!cls?.youTeach) {
            const p = purposeProblem(look.purpose);
            if (p) return onMsg(p);
        }
        const r = await edu<RosterRow[]>("get", `/classes/${look.classId}/roster`, cls?.youTeach ? scope : { ...scope, purpose: look.purpose.trim() });
        if (!r.ok) return onMsg(teacherError(r));
        setRoster(r.data);
        onMsg(cls?.youTeach ? "这是你任课的班级" : "本次查看已记入访问记录");
        void load();
    };

    return (
        <>
            <section className={box} aria-labelledby="org-overview">
                <h2 id="org-overview" className="mb-2 font-medium">
                    机构概览（只含汇总数字）
                </h2>
                <ul className="list-disc pl-5 text-sm">
                    <li>{staffLine(ov.staff)}</li>
                    <li>
                        课程 {ov.courses} 门，班级 {ov.classes.length} 个
                    </li>
                    <li>{consentLine(ov.students)}</li>
                    <li>{regionLine(ov.region)}</li>
                    <li>{retentionLine(ov.retention)}</li>
                </ul>
                {ov.classes.length === 0 ? (
                    <p className="mt-2 text-sm text-stone-500">还没有班级。下一步：教师在“课堂”页创建班级。</p>
                ) : (
                    <table className="mt-3 w-full text-left text-sm">
                        <caption className="sr-only">班级汇总</caption>
                        <thead>
                            <tr>
                                <th scope="col">班级</th>
                                <th scope="col">任课教师</th>
                                <th scope="col">在读学生</th>
                                <th scope="col">进行中任务</th>
                                <th scope="col">本人任课</th>
                            </tr>
                        </thead>
                        <tbody>
                            {ov.classes.map((k) => (
                                <tr key={k.id}>
                                    <td>{k.name}</td>
                                    <td>{k.teachers}</td>
                                    <td>{k.students}</td>
                                    <td>{k.activeAssignments}</td>
                                    <td>{k.youTeach ? "是" : "否"}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            </section>

            <section className={box} aria-labelledby="org-roles">
                <h2 id="org-roles" className="mb-2 font-medium">
                    人员与角色
                </h2>
                <div className="flex flex-wrap gap-2">
                    <input className={input} aria-label="用户编号" placeholder="用户编号" value={bind.userId} onChange={(e) => setBind({ ...bind, userId: e.target.value })} />
                    <select className={input} aria-label="角色" value={bind.role} onChange={(e) => setBind({ ...bind, role: e.target.value })}>
                        {Object.entries(ROLE_LABEL).map(([k, v]) => (
                            <option key={k} value={k}>
                                {v}
                            </option>
                        ))}
                    </select>
                    <button className={btn} onClick={() => void bindRole()}>
                        授予角色
                    </button>
                    <input className={`${input} flex-1`} aria-label="撤销理由" placeholder="撤销理由（撤销时必填）" value={revokeReason} onChange={(e) => setRevokeReason(e.target.value)} />
                </div>
                {roles.length === 0 ? (
                    <p className="mt-2 text-sm text-stone-500">还没有授予任何教育角色。</p>
                ) : (
                    <ul className="mt-2 text-sm">
                        {roles.map((r) => (
                            <li key={r.binding.id} className="flex items-center gap-2 py-1">
                                <span className="flex-1">
                                    {r.binding.userId}：{ROLE_LABEL[r.binding.role] || r.binding.role}
                                    {r.binding.workspaceId ? "（限本工作区）" : ""}，{EFFECTIVE_LABEL[r.effective] || r.effective}
                                    {r.binding.expiresAt ? `，到期 ${new Date(r.binding.expiresAt).toLocaleString()}` : ""}
                                </span>
                                {r.effective === "active" ? (
                                    <button className={ghost} aria-label={`撤销 ${r.binding.userId} 的${ROLE_LABEL[r.binding.role] || r.binding.role}角色`} onClick={() => void revoke(r)}>
                                        撤销
                                    </button>
                                ) : null}
                            </li>
                        ))}
                    </ul>
                )}
            </section>

            <section className={box} aria-labelledby="org-policy">
                <h2 id="org-policy" className="mb-2 font-medium">
                    审核与同意策略
                </h2>
                <div className="flex flex-wrap gap-2">
                    <select className={input} aria-label="审核方式" value={policy.reviewMode} onChange={(e) => setPolicy({ ...policy, reviewMode: e.target.value })}>
                        {Object.entries(REVIEW_MODE_LABEL).map(([k, v]) => (
                            <option key={k} value={k}>
                                {v}
                            </option>
                        ))}
                    </select>
                    <select className={input} aria-label="监护人同意" value={policy.consentMode} onChange={(e) => setPolicy({ ...policy, consentMode: e.target.value })}>
                        {Object.entries(CONSENT_LABEL).map(([k, v]) => (
                            <option key={k} value={k}>
                                {v}
                            </option>
                        ))}
                    </select>
                    <select className={input} aria-label="修改后审核继承" value={policy.editReviewInherit} onChange={(e) => setPolicy({ ...policy, editReviewInherit: e.target.value })}>
                        {Object.entries(INHERIT_LABEL).map(([k, v]) => (
                            <option key={k} value={k}>
                                {v}
                            </option>
                        ))}
                    </select>
                    <input className={`${input} flex-1`} aria-label="修改理由" placeholder="修改理由（写入审计）" value={policy.reason} onChange={(e) => setPolicy({ ...policy, reason: e.target.value })} />
                    <button className={btn} onClick={() => void savePolicy()}>
                        保存策略
                    </button>
                </div>
            </section>

            {ret && retDays ? (
                <section className={box} aria-labelledby="org-retention">
                    <h2 id="org-retention" className="mb-2 font-medium">
                        数据保留
                    </h2>
                    <p className="mb-2 text-sm text-stone-500">
                        {ret.confirmed ? `已由 ${ret.policy.confirmedBy || "机构管理员"} 确认${ret.policy.reason ? `（${ret.policy.reason}）` : ""}，按期自动清理。` : "下列天数是建议值，机构确认前不会清理任何数据。"}
                        账本、用量与审计记录单独保存，不在清理范围内；备份保留由部署方执行。
                    </p>
                    <div className="grid gap-2 sm:grid-cols-2">
                        {(Object.keys(RETENTION_LABEL) as (keyof RetentionDays)[]).map((k) => (
                            <label key={k} className="flex items-center gap-2 text-sm">
                                <span className="flex-1">{RETENTION_LABEL[k]}</span>
                                <input
                                    className={`${input} w-20`}
                                    type="number"
                                    min={ret.bounds[k][0]}
                                    max={ret.bounds[k][1]}
                                    aria-label={`${RETENTION_LABEL[k]}保留天数`}
                                    value={retDays[k]}
                                    onChange={(e) => setRetDays({ ...retDays, [k]: Number(e.target.value) })}
                                />
                                天
                            </label>
                        ))}
                    </div>
                    <div className="mt-2 flex flex-wrap gap-2">
                        <input className={`${input} flex-1`} aria-label="保留期确认理由" placeholder="确认理由或依据（写入审计）" value={retReason} onChange={(e) => setRetReason(e.target.value)} />
                        <button className={btn} onClick={() => void confirmRetention()}>
                            确认保留期
                        </button>
                    </div>
                    <p className="mt-2 text-sm">当前影响：{retentionImpact(retDry || ret.preview)}</p>
                    {ret.policy.lastRunAt ? <p className="text-sm text-stone-500">最近清理 {new Date(ret.policy.lastRunAt).toLocaleString()}</p> : null}
                    {ret.confirmed ? (
                        <div className="mt-2 flex gap-2">
                            <button className={ghost} onClick={() => void runRetention(false)}>
                                预览清理
                            </button>
                            <button className={ghost} disabled={!retDry} onClick={() => void runRetention(true)}>
                                执行清理
                            </button>
                        </div>
                    ) : null}
                </section>
            ) : null}

            <DataRequestsPanel scope={scope} onMsg={onMsg} />

            <OpsGrantsPanel scope={scope} classes={ov.classes} onMsg={onMsg} />

            <section className={box} aria-labelledby="org-individual">
                <h2 id="org-individual" className="mb-2 font-medium">
                    查看个体学生记录
                </h2>
                <p className="mb-2 text-sm text-stone-500">默认只看汇总。查看自己不任课班级的名单、作答或求助时，须说明业务目的；每次查看都记入下方访问记录。</p>
                <div className="flex flex-wrap gap-2">
                    <select className={input} aria-label="班级" value={look.classId} onChange={(e) => setLook({ ...look, classId: e.target.value })}>
                        <option value="">选择班级</option>
                        {ov.classes.map((k) => (
                            <option key={k.id} value={k.id}>
                                {k.name}
                            </option>
                        ))}
                    </select>
                    <input className={`${input} flex-1`} aria-label="查看目的" maxLength={PURPOSE_MAX} placeholder="查看目的，例如：家长投诉核查（工单号）" value={look.purpose} onChange={(e) => setLook({ ...look, purpose: e.target.value })} />
                    <button className={ghost} onClick={() => void viewRoster()}>
                        查看名单
                    </button>
                </div>
                {roster ? (
                    roster.length === 0 ? (
                        <p className="mt-2 text-sm text-stone-500">该班级还没有学生。</p>
                    ) : (
                        <ul className="mt-2 text-sm">
                            {roster.map((s) => (
                                <li key={s.enrollmentId}>
                                    {s.displayName || s.externalRef || s.studentUserId}：{s.status === "active" ? "在读" : "已移出"}，{s.consentOk ? "已登记同意" : "缺少同意"}
                                </li>
                            ))}
                        </ul>
                    )
                ) : null}
            </section>

            <section className={box} aria-labelledby="org-log">
                <h2 id="org-log" className="mb-2 font-medium">
                    访问与治理记录（最近 200 条）
                </h2>
                {log.length === 0 ? (
                    <p className="text-sm text-stone-500">暂无记录。</p>
                ) : (
                    <ul className="text-sm">
                        {log.map((r) => (
                            <li key={r.id}>
                                {new Date(r.createdAt).toLocaleString()} {accessRowText(r)}
                            </li>
                        ))}
                    </ul>
                )}
            </section>
        </>
    );
}
