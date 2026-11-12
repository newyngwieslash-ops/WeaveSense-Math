import { useCallback, useEffect, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";

import { archiveImpactText, archiveReasonProblem, assignmentImpactText, extensionDueISO, type ArchiveImpact, INVITE_STATUS_LABEL, withdrawReasonProblem, type AssignmentImpact, type ExtensionRow, type InviteRow, type RosterRow } from "./edu-admin-logic";
import {
    DEFAULT_CLAIM_TTL_HOURS,
    formatClaimCodes,
    ROSTER_CSV_TEMPLATE,
    ROSTER_ENTRY_STATE_LABEL,
    ROSTER_ROW_STATUS_LABEL,
    rosterCodesNotice,
    rosterConfirmProblem,
    rosterPreviewSummary,
    type IssuedClaimCode,
    type RosterEntryRow,
    type RosterImportPreview,
    type RosterImportRow,
} from "./roster-import-logic";
import { teacherError } from "./teacher-errors";

/**
 * 教师端班级管理：建班、邀请码（明文只在创建时显示一次）、撤销、名册、退出班级、同意记录，
 * 名单 CSV 导入与认领码分发（DEV-016，先预览再确认，认领码只显示一次），以及单个学生的作业延期和班级归档（DEV-106，先看影响、写原因再确认）。所有权限与校验由服务端完成。
 */
const btn = "inline-flex h-8 items-center rounded-md bg-stone-900 px-3 text-sm text-white disabled:opacity-40 dark:bg-stone-100 dark:text-stone-900";
const ghost = "inline-flex h-8 items-center rounded-md border border-stone-300 px-2 text-sm disabled:opacity-40 dark:border-stone-600";

export function ClassAdmin({ scope, classId, onCreated }: { scope: EduScope; classId: string; onCreated: (id: string) => void }) {
    const [name, setName] = useState("");
    const [invites, setInvites] = useState<InviteRow[]>([]);
    const [roster, setRoster] = useState<RosterRow[]>([]);
    const [code, setCode] = useState("");
    const [ttl, setTtl] = useState(168);
    const [maxUses, setMaxUses] = useState(60);
    const [evidence, setEvidence] = useState("");
    const [msg, setMsg] = useState("");
    const [archive, setArchive] = useState<ArchiveImpact | null>(null);
    const [archiveReason, setArchiveReason] = useState("");

    const reload = useCallback(() => {
        if (!classId) return;
        void edu<InviteRow[]>("get", `/classes/${classId}/invites`, scope).then((r) => r.ok && setInvites(r.data));
        void edu<RosterRow[]>("get", `/classes/${classId}/roster`, scope).then((r) => r.ok && setRoster(r.data));
    }, [classId, scope]);
    useEffect(() => {
        setCode("");
        setArchive(null);
        setArchiveReason("");
        reload();
    }, [reload]);

    const create = async () => {
        const r = await edu<{ id: string }>("post", "/classes", scope, { name: name.trim() });
        if (!r.ok) return setMsg(teacherError(r));
        setName("");
        onCreated(r.data.id);
    };
    const invite = async () => {
        const r = await edu<{ code: string }>("post", `/classes/${classId}/invites`, scope, { ttlHours: ttl, maxUses });
        if (!r.ok) return setMsg(teacherError(r));
        setCode(r.data.code);
        reload();
    };
    const previewArchive = async () => {
        const r = await edu<ArchiveImpact>("post", `/classes/${classId}/archive`, scope, {});
        if (!r.ok) return setMsg(teacherError(r));
        setArchive(r.data);
        setMsg("");
    };
    const confirmArchive = async () => {
        const problem = archiveReasonProblem(archiveReason);
        if (problem) return setMsg(problem);
        const r = await edu("post", `/classes/${classId}/archive`, scope, { confirm: true, reason: archiveReason.trim() });
        if (!r.ok) return setMsg(teacherError(r));
        setArchive(null);
        setArchiveReason("");
        setMsg("班级已归档");
        onCreated("");
    };
    const act = async (path: string, body: unknown, ok: string) => {
        const r = await edu("post", path, scope, body);
        setMsg(r.ok ? ok : teacherError(r));
        reload();
    };

    return (
        <div className="space-y-3 text-sm">
            {msg ? (
                <p role="status" aria-live="polite" className="text-amber-700">
                    {msg}
                </p>
            ) : null}
            <div className="flex gap-2">
                <input className="h-8 flex-1 rounded-md border px-2" placeholder="新班级名称" maxLength={120} value={name} onChange={(e) => setName(e.target.value)} />
                <button className={btn} disabled={!name.trim()} onClick={() => void create()}>
                    新建班级
                </button>
            </div>
            {classId ? (
                <>
                    <div>
                        <h3 className="font-medium">邀请码</h3>
                        <div className="mt-1 flex flex-wrap items-center gap-2">
                            <label>
                                有效小时 <input type="number" className="h-8 w-20 rounded-md border px-2" min={1} max={720} value={ttl} onChange={(e) => setTtl(Number(e.target.value))} />
                            </label>
                            <label>
                                最多使用 <input type="number" className="h-8 w-20 rounded-md border px-2" min={1} max={500} value={maxUses} onChange={(e) => setMaxUses(Number(e.target.value))} />
                            </label>
                            <button className={btn} onClick={() => void invite()}>
                                生成邀请码
                            </button>
                        </div>
                        {code ? (
                            <p className="mt-1 rounded-md bg-amber-50 p-2 font-mono dark:bg-amber-950" data-testid="invite-code">
                                {code}
                                <span className="ml-2 font-sans text-xs">只显示这一次，服务端只保存哈希，请现在分发</span>
                            </p>
                        ) : null}
                        <ul className="mt-1 space-y-1">
                            {invites.map((i) => (
                                <li key={i.id} className="flex flex-wrap items-center gap-2">
                                    <span>
                                        {i.id.slice(-6)}：{INVITE_STATUS_LABEL[i.status] || i.status}，已用 {i.uses}/{i.maxUses}，到期 {new Date(i.expiresAt).toLocaleString()}
                                    </span>
                                    {i.status === "active" ? (
                                        <button className={ghost} onClick={() => void act(`/classes/${classId}/invites/${i.id}/revoke`, {}, "邀请码已撤销")}>
                                            撤销
                                        </button>
                                    ) : null}
                                </li>
                            ))}
                        </ul>
                    </div>
                    <div>
                        <h3 className="font-medium">名册（{roster.filter((r) => r.status === "active").length} 名在读）</h3>
                        <input className="mt-1 h-8 w-full rounded-md border px-2" placeholder="同意凭证编号（纸质或系统记录编号，记录同意时必填建议）" value={evidence} onChange={(e) => setEvidence(e.target.value)} />
                        <ul className="mt-1 space-y-1">
                            {roster.map((s) => (
                                <li key={s.enrollmentId} className="flex flex-wrap items-center gap-2">
                                    <span>
                                        {s.studentUserId}：{s.status === "active" ? "在读" : "已退出"}，{s.consentOk ? "已记录同意" : "未记录同意（不能使用学习功能）"}
                                    </span>
                                    {s.status === "active" ? (
                                        <>
                                            <button className={ghost} onClick={() => void act("/consents", { studentUserId: s.studentUserId, status: s.consentOk ? "withdrawn" : "granted", evidenceRef: evidence }, "已记录")}>
                                                {s.consentOk ? "记录撤回同意" : "记录同意"}
                                            </button>
                                            <button className={ghost} onClick={() => confirm("让该学生退出班级？其作业访问会立即失效。") && void act(`/enrollments/${s.enrollmentId}/withdraw`, {}, "已退出班级")}>
                                                退出班级
                                            </button>
                                        </>
                                    ) : null}
                                </li>
                            ))}
                        </ul>
                    </div>
                    <RosterImportPanel scope={scope} classId={classId} />
                    <div>
                        <h3 className="font-medium">归档班级</h3>
                        {archive ? (
                            <div className="mt-1 space-y-2">
                                <p data-testid="archive-impact">{archiveImpactText(archive)}</p>
                                <label className="block">
                                    归档原因
                                    <input className="ml-2 h-8 w-80 rounded-md border px-2" maxLength={200} value={archiveReason} onChange={(e) => setArchiveReason(e.target.value)} />
                                </label>
                                <div className="flex gap-2">
                                    <button className={btn} onClick={() => void confirmArchive()}>
                                        确认归档
                                    </button>
                                    <button className={ghost} onClick={() => setArchive(null)}>
                                        取消
                                    </button>
                                </div>
                            </div>
                        ) : (
                            <button className={`${ghost} mt-1`} onClick={() => void previewArchive()}>
                                查看归档影响
                            </button>
                        )}
                    </div>
                </>
            ) : null}
        </div>
    );
}

function RosterImportPanel({ scope, classId }: { scope: EduScope; classId: string }) {
    const [csv, setCsv] = useState(ROSTER_CSV_TEMPLATE);
    const [ttl, setTtl] = useState(DEFAULT_CLAIM_TTL_HOURS);
    const [preview, setPreview] = useState<RosterImportPreview | null>(null);
    const [codes, setCodes] = useState<IssuedClaimCode[]>([]);
    const [entries, setEntries] = useState<RosterEntryRow[]>([]);
    const [issued, setIssued] = useState("");
    const [msg, setMsg] = useState("");
    const [busy, setBusy] = useState(false);

    const reloadEntries = useCallback(() => {
        void edu<RosterEntryRow[]>("get", `/classes/${classId}/roster-entries`, scope).then((r) => r.ok && setEntries(Array.isArray(r.data) ? r.data : []));
    }, [classId, scope]);
    useEffect(() => {
        setPreview(null);
        setCodes([]);
        setIssued("");
        setMsg("");
        reloadEntries();
    }, [reloadEntries]);

    const previewImport = async () => {
        setBusy(true);
        const r = await edu<RosterImportPreview>("post", `/classes/${classId}/roster-import`, scope, { csv, confirm: false, ttlHours: ttl });
        setBusy(false);
        if (!r.ok) return setMsg(teacherError(r));
        setPreview(r.data);
        setCodes([]);
        setMsg(rosterPreviewSummary(r.data));
    };
    const confirmImport = async () => {
        const problem = rosterConfirmProblem(preview, csv);
        if (problem) return setMsg(problem);
        setBusy(true);
        const r = await edu<RosterImportPreview>("post", `/classes/${classId}/roster-import`, scope, { csv, confirm: true, ttlHours: ttl });
        setBusy(false);
        if (!r.ok) return setMsg(teacherError(r));
        const issuedCodes = r.data.codes || [];
        setPreview(r.data);
        setCodes(issuedCodes);
        setIssued("");
        setMsg(rosterCodesNotice(issuedCodes.length));
        setCsv(ROSTER_CSV_TEMPLATE);
        reloadEntries();
    };
    const reissue = async (id: string) => {
        setBusy(true);
        const r = await edu<IssuedClaimCode>("post", `/classes/${classId}/roster-entries/${id}/reissue`, scope, { ttlHours: ttl });
        setBusy(false);
        if (!r.ok) return setMsg(teacherError(r));
        setIssued(r.data.code);
        setMsg(rosterCodesNotice(1));
        reloadEntries();
    };
    const revoke = async (id: string) => {
        if (!confirm("作废这条待认领记录？认领码将立即失效。")) return;
        setBusy(true);
        const r = await edu("post", `/classes/${classId}/roster-entries/${id}/revoke`, scope, {});
        setBusy(false);
        setMsg(r.ok ? "已作废该名单条目" : teacherError(r));
        reloadEntries();
    };
    const onFile = (f: File | undefined) => {
        if (!f) return;
        const reader = new FileReader();
        reader.onload = () => {
            const text = typeof reader.result === "string" ? reader.result : "";
            setCsv(text);
            setPreview(null);
            setCodes([]);
        };
        reader.readAsText(f);
    };
    const copyCodes = async () => {
        const text = formatClaimCodes(codes);
        try {
            await navigator.clipboard.writeText(text);
            setMsg("已复制认领码，请现在分发");
        } catch {
            setMsg("无法自动复制，请手动选中认领码后复制");
        }
    };

    return (
        <div>
            <h3 className="font-medium">导入名单</h3>
            <p className="mt-1 text-xs text-stone-500">只填写假名编号和短显示名，不要填写真实姓名、手机号、身份证号或邮箱。先预览，核对后再确认。认领码只显示一次。</p>
            <textarea
                className="mt-1 h-28 w-full rounded-md border px-2 font-mono text-xs"
                value={csv}
                onChange={(e) => {
                    setCsv(e.target.value);
                    setPreview(null);
                    setCodes([]);
                }}
                spellCheck={false}
                data-testid="roster-import-csv"
                aria-label="名单 CSV"
            />
            <div className="mt-1 flex flex-wrap items-center gap-2">
                <label className="text-xs">
                    选择 CSV 文件
                    <input className="ml-2 text-xs" type="file" accept=".csv,text/csv,text/plain" onChange={(e) => onFile(e.target.files?.[0])} />
                </label>
                <label>
                    认领有效小时 <input type="number" className="h-8 w-20 rounded-md border px-2" min={1} max={336} value={ttl} onChange={(e) => setTtl(Number(e.target.value))} />
                </label>
                <button className={ghost} disabled={busy || !csv.trim()} onClick={() => void previewImport()}>
                    预览名单
                </button>
                <button className={btn} disabled={busy || !!rosterConfirmProblem(preview, csv)} onClick={() => void confirmImport()}>
                    确认导入
                </button>
            </div>
            {msg ? (
                <p role="status" aria-live="polite" className="mt-1 text-amber-700" data-testid="roster-import-status">
                    {msg}
                </p>
            ) : null}
            {preview && !preview.confirmed ? (
                <ul className="mt-1 max-h-48 space-y-1 overflow-auto text-xs" data-testid="roster-import-preview">
                    {preview.rows.map((row: RosterImportRow) => (
                        <li key={`${row.line}-${row.externalRef}`}>
                            第 {row.line} 行 {row.externalRef}
                            {row.displayName ? `（${row.displayName}）` : ""}：{ROSTER_ROW_STATUS_LABEL[row.status] || row.status}
                            {row.error ? `，${row.error}` : ""}
                        </li>
                    ))}
                </ul>
            ) : null}
            {codes.length ? (
                <div className="mt-1 rounded-md bg-amber-50 p-2 dark:bg-amber-950" data-testid="roster-import-codes">
                    <p className="text-xs">{rosterCodesNotice(codes.length)}</p>
                    <pre className="mt-1 overflow-auto font-mono text-xs">{formatClaimCodes(codes)}</pre>
                    <button className={`${ghost} mt-1`} onClick={() => void copyCodes()}>
                        复制全部认领码
                    </button>
                </div>
            ) : null}
            {issued ? (
                <p className="mt-1 rounded-md bg-amber-50 p-2 font-mono text-xs dark:bg-amber-950" data-testid="roster-reissue-code">
                    {issued}
                    <span className="ml-2 font-sans">只显示这一次，请现在发给学生</span>
                </p>
            ) : null}
            <ul className="mt-2 space-y-1" data-testid="roster-entries">
                {entries.map((e) => (
                    <li key={e.id} className="flex flex-wrap items-center gap-2">
                        <span>
                            {e.externalRef}
                            {e.displayName ? `（${e.displayName}）` : ""}：{ROSTER_ENTRY_STATE_LABEL[e.state] || e.state}
                            {e.state === "pending" && e.expiresAt ? `，到期 ${new Date(e.expiresAt).toLocaleString()}` : ""}
                            {e.state === "claimed" && e.claimedBy ? `，已由 ${e.claimedBy} 认领` : ""}
                        </span>
                        {e.state === "pending" ? (
                            <>
                                <button className={ghost} disabled={busy} onClick={() => void reissue(e.id)}>
                                    重发认领码
                                </button>
                                <button className={ghost} disabled={busy} onClick={() => void revoke(e.id)}>
                                    作废
                                </button>
                            </>
                        ) : null}
                    </li>
                ))}
            </ul>
        </div>
    );
}

export function ExtensionPanel({ scope, classId, assignmentId, dueAt }: { scope: EduScope; classId: string; assignmentId: string; dueAt?: string }) {
    const [rows, setRows] = useState<ExtensionRow[]>([]);
    const [roster, setRoster] = useState<RosterRow[]>([]);
    const [student, setStudent] = useState("");
    const [local, setLocal] = useState("");
    const [reason, setReason] = useState("");
    const [msg, setMsg] = useState("");
    const reload = useCallback(() => {
        void edu<ExtensionRow[]>("get", `/assignments/${assignmentId}/extensions`, scope).then((r) => r.ok && setRows(r.data));
        void edu<RosterRow[]>("get", `/classes/${classId}/roster`, scope).then((r) => r.ok && setRoster(r.data.filter((x) => x.status === "active")));
    }, [assignmentId, classId, scope]);
    useEffect(reload, [reload]);
    const save = async () => {
        const due = extensionDueISO(local, dueAt);
        if (!due.ok) return setMsg(due.error);
        const r = await edu("post", `/assignments/${assignmentId}/extensions`, scope, { studentUserId: student, dueAt: due.iso, reason });
        setMsg(r.ok ? "已设置延期" : teacherError(r));
        reload();
    };
    const remove = async (sid: string) => {
        const r = await edu("post", `/assignments/${assignmentId}/extensions/${sid}/remove`, scope, {});
        setMsg(r.ok ? "已取消延期" : teacherError(r));
        reload();
    };
    return (
        <div className="text-sm">
            <p className="text-xs text-stone-500">原截止 {dueAt ? new Date(dueAt).toLocaleString() : "未设置（不能延期）"}；时间按本机时区输入，服务端以 UTC 保存。</p>
            {msg ? (
                <p role="status" aria-live="polite" className="text-amber-700">
                    {msg}
                </p>
            ) : null}
            <div className="mt-1 flex flex-wrap gap-2">
                <select className="h-8 rounded-md border px-2" value={student} onChange={(e) => setStudent(e.target.value)}>
                    <option value="">选择学生</option>
                    {roster.map((s) => (
                        <option key={s.studentUserId} value={s.studentUserId}>
                            {s.studentUserId}
                        </option>
                    ))}
                </select>
                <input type="datetime-local" className="h-8 rounded-md border px-2" value={local} onChange={(e) => setLocal(e.target.value)} />
                <input className="h-8 flex-1 rounded-md border px-2" maxLength={200} placeholder="原因（可选）" value={reason} onChange={(e) => setReason(e.target.value)} />
                <button className={btn} disabled={!student || !dueAt} onClick={() => void save()}>
                    设置延期
                </button>
            </div>
            <ul className="mt-1 space-y-1">
                {rows.map((x) => (
                    <li key={x.id} className="flex items-center gap-2">
                        <span>
                            {x.studentUserId}：延至 {new Date(x.dueAt).toLocaleString()}
                            {x.reason ? `（${x.reason}）` : ""}
                        </span>
                        <button className={ghost} onClick={() => void remove(x.studentUserId)}>
                            取消
                        </button>
                    </li>
                ))}
            </ul>
        </div>
    );
}

/** DEV-112：撤回任务先预览影响范围，填写原因后再确认；服务端校验权限并记录审计。 */
export function WithdrawAssignmentPanel({ scope, assignmentId, onDone }: { scope: EduScope; assignmentId: string; onDone: () => void }) {
    const [impact, setImpact] = useState<AssignmentImpact | null>(null);
    const [reason, setReason] = useState("");
    const [msg, setMsg] = useState("");
    useEffect(() => {
        setImpact(null);
        setReason("");
        setMsg("");
    }, [assignmentId]);
    const preview = async () => {
        const r = await edu<{ dryRun: boolean; impact: AssignmentImpact }>("post", `/assignments/${assignmentId}/withdraw`, scope, {});
        if (!r.ok) return setMsg(teacherError(r));
        setImpact(r.data.impact);
        setMsg("");
    };
    const confirmWithdraw = async () => {
        const problem = withdrawReasonProblem(reason);
        if (problem) return setMsg(problem);
        const r = await edu("post", `/assignments/${assignmentId}/withdraw`, scope, { confirm: true, reason: reason.trim() });
        if (!r.ok) return setMsg(teacherError(r));
        setImpact(null);
        setMsg("已撤回，学生不可见");
        onDone();
    };
    return (
        <div className="text-sm">
            {msg ? (
                <p role="status" aria-live="polite" className="text-amber-700">
                    {msg}
                </p>
            ) : null}
            {impact ? (
                <div className="space-y-2">
                    <p data-testid="withdraw-impact">{assignmentImpactText(impact)}</p>
                    <label className="block">
                        撤回原因
                        <input className="ml-2 h-8 w-80 rounded-md border px-2" maxLength={200} value={reason} onChange={(e) => setReason(e.target.value)} />
                    </label>
                    <div className="flex gap-2">
                        <button className={btn} onClick={() => void confirmWithdraw()}>
                            确认撤回
                        </button>
                        <button className={ghost} onClick={() => setImpact(null)}>
                            取消
                        </button>
                    </div>
                </div>
            ) : (
                <button className={ghost} onClick={() => void preview()}>
                    查看撤回影响
                </button>
            )}
        </div>
    );
}
