import { useCallback, useEffect, useState } from "react";

import { NOTICE_STATE_LABEL, noticeLine, reviewInputError, type ReviewInput, type RevisionNotice } from "@/components/edu/incident-sop-logic";
import { teacherError } from "@/components/edu/teacher-errors";
import { edu, type EduScope } from "@/services/api/edu";

const input = "rounded-md border border-stone-300 px-2 py-1 text-sm dark:border-stone-600 dark:bg-stone-900";
const btn = "h-8 rounded-md border border-stone-300 px-3 text-sm disabled:opacity-50 dark:border-stone-600";

/** DEV-126 修订通知：内容事故以修正版关闭后，任课教师收到的未读通知；迁移任务后自动关闭，也可手动确认。 */
export function RevisionNoticePanel({ scope }: { scope: EduScope }) {
    const [list, setList] = useState<RevisionNotice[]>([]);
    const [error, setError] = useState("");
    const load = useCallback(async () => {
        const r = await edu<RevisionNotice[]>("get", "/revision-notices?state=unread", scope);
        setList(r.ok ? r.data : []);
        setError(r.ok ? "" : teacherError(r));
    }, [scope]);
    useEffect(() => {
        void load();
    }, [load]);
    const ack = async (id: string) => {
        const r = await edu("post", `/revision-notices/${encodeURIComponent(id)}/ack`, scope);
        setError(r.ok ? "" : teacherError(r));
        if (r.ok) void load();
    };
    return (
        <div data-testid="revision-notices" className="mt-2 text-sm">
            {error && (
                <p className="text-red-600" role="alert">
                    {error}
                </p>
            )}
            <ul aria-label="修订通知">
                {list.map((n) => (
                    <li key={n.notice.id} className="flex flex-wrap items-center gap-2 border-t border-stone-100 py-1 dark:border-stone-800">
                        <span className="w-12 text-amber-700">{NOTICE_STATE_LABEL[n.notice.state]}</span>
                        <span className="flex-1">{noticeLine(n)}</span>
                        <button type="button" className={btn} onClick={() => void ack(n.notice.id)}>
                            确认已知
                        </button>
                    </li>
                ))}
            </ul>
        </div>
    );
}

type Incident = { id: string; publicationId: string; severity: string; reason: string; resolvedAt?: string };

/** DEV-126 事故复盘：已关闭的阻断事故每个写一次复盘（根因、发现途径、纠正措施、后续事项），写后不可修改。 */
export function IncidentReviewPanel({ scope }: { scope: EduScope }) {
    const [list, setList] = useState<Incident[] | null>(null);
    const [open, setOpen] = useState("");
    const [v, setV] = useState<ReviewInput>({ rootCause: "", detection: "", correctiveActions: "", followUps: "" });
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);
    const load = useCallback(async () => {
        const r = await edu<Incident[]>("get", "/incidents?reviewPending=1", scope);
        setList(r.ok ? r.data : null);
        if (!r.ok) setError(teacherError(r));
    }, [scope]);
    useEffect(() => {
        void load();
    }, [load]);
    const submit = async () => {
        const bad = reviewInputError(v);
        if (bad) {
            setError(bad);
            return;
        }
        setBusy(true);
        const r = await edu("post", `/incidents/${encodeURIComponent(open)}/review`, scope, v);
        setBusy(false);
        setError(r.ok ? "" : teacherError(r));
        if (r.ok) {
            setOpen("");
            setV({ rootCause: "", detection: "", correctiveActions: "", followUps: "" });
            void load();
        }
    };
    const field = (k: keyof ReviewInput, label: string) => (
        <label className="block">
            <span className="text-xs text-stone-500">{label}</span>
            <textarea className={`${input} block w-full`} aria-label={label} rows={2} value={v[k]} onChange={(e) => setV({ ...v, [k]: e.target.value })} />
        </label>
    );
    return (
        <div data-testid="incident-reviews" className="text-sm">
            <p className="mb-2 text-xs text-stone-500">处置顺序：定位影响，撤回或暂停，修订并重新审核，发布修正版并通知任课教师，最后复盘。复盘不得写学生姓名或联系方式。</p>
            {list && list.length === 0 && <p className="text-stone-500">没有待复盘的事故。</p>}
            <ul aria-label="待复盘事故">
                {(list ?? []).map((i) => (
                    <li key={i.id} className="border-t border-stone-100 py-1 dark:border-stone-800">
                        <div className="flex items-center gap-2">
                            <span className="flex-1">{i.reason}</span>
                            <button type="button" className={btn} onClick={() => setOpen(open === i.id ? "" : i.id)}>
                                写复盘
                            </button>
                        </div>
                        {open === i.id && (
                            <div className="mt-2 space-y-2">
                                {field("rootCause", "根因")}
                                {field("detection", "发现途径")}
                                {field("correctiveActions", "纠正措施")}
                                {field("followUps", "后续事项")}
                                <button type="button" className={btn} disabled={busy} onClick={() => void submit()}>
                                    提交复盘（提交后不可修改）
                                </button>
                            </div>
                        )}
                    </li>
                ))}
            </ul>
            {error && (
                <p className="mt-2 text-red-600" role="alert">
                    {error}
                </p>
            )}
        </div>
    );
}
