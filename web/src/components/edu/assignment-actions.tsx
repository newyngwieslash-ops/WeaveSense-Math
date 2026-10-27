import { useEffect, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";

import { actionReasonProblem, migratePreviewText, publicationImpactText, regradeChangeText, regradePreviewText, type MigratePreview, type PublicationImpact, type RegradePreview } from "./assignment-actions-logic";
import { teacherError } from "./teacher-errors";

/**
 * DEV-112 教师端影响确认入口：迁移任务、批量重评分、撤回发布。
 * 每个操作先请求服务端预览（不改任何状态），显示具体影响，再由教师填写原因确认。
 * 权限（任课教师或机构管理员）、来源权利、状态与重复操作的检查都在服务端完成。
 */
const btn = "inline-flex h-8 items-center rounded-md bg-stone-900 px-3 text-sm text-white disabled:opacity-40 dark:bg-stone-100 dark:text-stone-900";
const ghost = "inline-flex h-8 items-center rounded-md border border-stone-300 px-2 text-sm disabled:opacity-40 dark:border-stone-600";
const danger = "inline-flex h-8 items-center rounded-md bg-red-700 px-3 text-sm text-white disabled:opacity-40";

export type PublicationRow = { id: string; title: string; familyId: string; createdAt: string };

function Status({ msg, id }: { msg: string; id: string }) {
    return msg ? (
        <p role="status" aria-live="polite" data-testid={id} className="text-amber-700">
            {msg}
        </p>
    ) : null;
}

function useReset(key: string, reset: () => void) {
    // eslint-disable-next-line react-hooks/exhaustive-deps
    useEffect(reset, [key]);
}

export function MigrateAssignmentPanel({ scope, assignmentId, publicationId, publications, onDone }: { scope: EduScope; assignmentId: string; publicationId: string; publications: PublicationRow[]; onDone: (newAssignmentId: string) => void }) {
    const [target, setTarget] = useState("");
    const [reason, setReason] = useState("");
    const [preview, setPreview] = useState<MigratePreview | null>(null);
    const [msg, setMsg] = useState("");
    useReset(assignmentId, () => {
        setTarget("");
        setReason("");
        setPreview(null);
        setMsg("");
    });
    const body = (confirm: boolean) => ({ publicationId: target, reason: reason.trim(), confirm });
    const ask = async () => {
        const problem = !target ? "请选择迁移到的发布版本" : actionReasonProblem(reason, 400);
        if (problem) return setMsg(problem);
        const r = await edu<MigratePreview>("post", `/assignments/${assignmentId}/migrate`, scope, body(false));
        if (!r.ok) return setMsg(teacherError(r));
        setPreview(r.data);
        setMsg("");
    };
    const confirmMigrate = async () => {
        const r = await edu<MigratePreview>("post", `/assignments/${assignmentId}/migrate`, scope, body(true));
        if (!r.ok) return setMsg(teacherError(r));
        setPreview(null);
        setMsg("已迁移到新任务，原任务的作答原样保留");
        onDone(r.data.toAssignmentId || "");
    };
    const options = publications.filter((p) => p.id !== publicationId);
    return (
        <div className="space-y-2 text-sm">
            <Status msg={msg} id="migrate-status" />
            <div className="flex flex-wrap gap-2">
                <select
                    aria-label="迁移到的发布版本"
                    data-testid="migrate-target"
                    className="h-8 rounded-md border px-2"
                    value={target}
                    onChange={(e) => {
                        setTarget(e.target.value);
                        setPreview(null);
                    }}
                >
                    <option value="">选择发布版本</option>
                    {options.map((p) => (
                        <option key={p.id} value={p.id}>
                            {p.title}（{new Date(p.createdAt).toLocaleString()}）
                        </option>
                    ))}
                </select>
                <input
                    aria-label="迁移原因"
                    data-testid="migrate-reason"
                    className="h-8 flex-1 rounded-md border px-2"
                    maxLength={400}
                    placeholder="原因（必填）"
                    value={reason}
                    onChange={(e) => {
                        setReason(e.target.value);
                        setPreview(null);
                    }}
                />
                <button className={ghost} data-testid="migrate-preview" onClick={() => void ask()}>
                    查看迁移影响
                </button>
            </div>
            {preview ? (
                <div className="space-y-2 rounded-md bg-stone-100 p-2 dark:bg-stone-800">
                    <p data-testid="migrate-impact">{migratePreviewText(preview)}</p>
                    <div className="flex gap-2">
                        <button className={btn} data-testid="migrate-confirm" onClick={() => void confirmMigrate()}>
                            确认迁移
                        </button>
                        <button className={ghost} onClick={() => setPreview(null)}>
                            取消
                        </button>
                    </div>
                </div>
            ) : null}
        </div>
    );
}

export function RegradePanel({ scope, assignmentId, publications }: { scope: EduScope; assignmentId: string; publications: PublicationRow[] }) {
    const [ref, setRef] = useState("");
    const [reason, setReason] = useState("");
    const [preview, setPreview] = useState<RegradePreview | null>(null);
    const [msg, setMsg] = useState("");
    useReset(assignmentId, () => {
        setRef("");
        setReason("");
        setPreview(null);
        setMsg("");
    });
    const body = (confirm: boolean) => ({ ...(ref ? { publicationId: ref } : {}), reason: reason.trim(), confirm });
    const ask = async () => {
        const problem = actionReasonProblem(reason, 200);
        if (problem) return setMsg(problem);
        const r = await edu<RegradePreview>("post", `/assignments/${assignmentId}/regrade`, scope, body(false));
        if (!r.ok) return setMsg(teacherError(r));
        setPreview(r.data);
        setMsg("");
    };
    const confirmRegrade = async () => {
        const r = await edu<RegradePreview>("post", `/assignments/${assignmentId}/regrade`, scope, body(true));
        if (!r.ok) return setMsg(teacherError(r));
        setPreview(null);
        setMsg(`已重评分：${r.data.written ?? 0} 次作答结果已更新，学生会收到“老师修正内容后重新评分”的通知`);
    };
    return (
        <div className="space-y-2 text-sm">
            <Status msg={msg} id="regrade-status" />
            <div className="flex flex-wrap gap-2">
                <select
                    aria-label="参照发布版本"
                    data-testid="regrade-ref"
                    className="h-8 rounded-md border px-2"
                    value={ref}
                    onChange={(e) => {
                        setRef(e.target.value);
                        setPreview(null);
                    }}
                >
                    <option value="">默认参照（迁移目标或本任务的发布）</option>
                    {publications.map((p) => (
                        <option key={p.id} value={p.id}>
                            {p.title}（{new Date(p.createdAt).toLocaleString()}）
                        </option>
                    ))}
                </select>
                <input
                    aria-label="重评分原因"
                    data-testid="regrade-reason"
                    className="h-8 flex-1 rounded-md border px-2"
                    maxLength={200}
                    placeholder="原因（必填，不会显示给学生）"
                    value={reason}
                    onChange={(e) => {
                        setReason(e.target.value);
                        setPreview(null);
                    }}
                />
                <button className={ghost} data-testid="regrade-preview" onClick={() => void ask()}>
                    预览重评分
                </button>
            </div>
            {preview ? (
                <div className="space-y-2 rounded-md bg-stone-100 p-2 dark:bg-stone-800">
                    <p data-testid="regrade-impact">{regradePreviewText(preview)}</p>
                    {preview.changed?.length ? (
                        <ul className="list-inside list-disc">
                            {preview.changed.slice(0, 50).map((c) => (
                                <li key={c.attemptId}>{regradeChangeText(c)}</li>
                            ))}
                        </ul>
                    ) : null}
                    <div className="flex gap-2">
                        <button className={btn} data-testid="regrade-confirm" onClick={() => void confirmRegrade()}>
                            确认重评分
                        </button>
                        <button className={ghost} onClick={() => setPreview(null)}>
                            取消
                        </button>
                    </div>
                </div>
            ) : null}
        </div>
    );
}

export function WithdrawPublicationPanel({ scope, publicationId, onDone }: { scope: EduScope; publicationId: string; onDone: () => void }) {
    const [reason, setReason] = useState("");
    const [impact, setImpact] = useState<PublicationImpact | null>(null);
    const [msg, setMsg] = useState("");
    useReset(publicationId, () => {
        setReason("");
        setImpact(null);
        setMsg("");
    });
    const url = `/publications/${publicationId}/incidents`;
    const ask = async () => {
        const problem = actionReasonProblem(reason, 400);
        if (problem) return setMsg(problem);
        const r = await edu<{ dryRun?: boolean; impact: PublicationImpact }>("post", url, scope, { severity: "blocking", reason: reason.trim() });
        if (!r.ok) return setMsg(teacherError(r));
        setImpact(r.data.impact);
        setMsg("");
    };
    const confirmWithdraw = async () => {
        const r = await edu("post", url, scope, { severity: "blocking", reason: reason.trim(), confirm: true });
        if (!r.ok) return setMsg(teacherError(r));
        setImpact(null);
        setMsg("已撤回发布：相关任务已暂停，学生不能再访问此讲解");
        onDone();
    };
    return (
        <div className="space-y-2 text-sm">
            <p className="text-xs text-stone-500">用于讲解内容有错、必须立即停止学生使用的情况。措辞类小问题请在编辑中修改后重新发布。</p>
            <Status msg={msg} id="withdraw-pub-status" />
            <div className="flex flex-wrap gap-2">
                <input
                    aria-label="撤回发布原因"
                    data-testid="withdraw-pub-reason"
                    className="h-8 flex-1 rounded-md border px-2"
                    maxLength={400}
                    placeholder="错误说明（必填）"
                    value={reason}
                    onChange={(e) => {
                        setReason(e.target.value);
                        setImpact(null);
                    }}
                />
                <button className={ghost} data-testid="withdraw-pub-preview" onClick={() => void ask()}>
                    查看撤回影响
                </button>
            </div>
            {impact ? (
                <div className="space-y-2 rounded-md bg-red-50 p-2 dark:bg-red-950">
                    <p data-testid="withdraw-pub-impact">{publicationImpactText(impact)}</p>
                    <div className="flex gap-2">
                        <button className={danger} data-testid="withdraw-pub-confirm" onClick={() => void confirmWithdraw()}>
                            确认撤回发布
                        </button>
                        <button className={ghost} onClick={() => setImpact(null)}>
                            取消
                        </button>
                    </div>
                </div>
            ) : null}
        </div>
    );
}
