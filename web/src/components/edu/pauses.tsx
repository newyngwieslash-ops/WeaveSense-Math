import { useCallback, useEffect, useState } from "react";

import { SCOPE_LABEL, TARGET_HINT, activeFirst, impactText, pauseInputError, type Pause, type PauseImpact, type PauseList, type PauseScope } from "@/components/edu/pause-logic";
import { teacherError } from "@/components/edu/teacher-errors";
import { edu, type EduScope } from "@/services/api/edu";

const input = "h-8 rounded-md border border-stone-300 px-2 text-sm dark:border-stone-600 dark:bg-stone-900";
const btn = "h-8 rounded-md border border-stone-300 px-3 text-sm disabled:opacity-50 dark:border-stone-600";

/** DEV-125 暂停开关：按机构、题族、发布版或渲染服务版本暂停，先预览影响再确认，可解除。仅机构管理员。 */
export function PausePanel({ scope }: { scope: EduScope }) {
    const [list, setList] = useState<PauseList | null>(null);
    const [error, setError] = useState("");
    const [kind, setKind] = useState<PauseScope>("publication");
    const [target, setTarget] = useState("");
    const [reason, setReason] = useState("");
    const [impact, setImpact] = useState<PauseImpact | null>(null);
    const [note, setNote] = useState("");
    const [busy, setBusy] = useState(false);

    const load = useCallback(async () => {
        const r = await edu<PauseList>("get", "/pauses", scope);
        setList(r.ok ? r.data : null);
        if (!r.ok) setError("无法读取暂停状态");
    }, [scope]);
    useEffect(() => {
        void load();
    }, [load]);

    const body = (confirm: boolean) => ({ scope: kind, targetId: kind === "generation" ? "" : target.trim(), reason: reason.trim(), confirm });
    const preview = async () => {
        const bad = pauseInputError(kind, target, reason);
        if (bad) {
            setError(bad);
            return;
        }
        setBusy(true);
        const r = await edu<{ impact: PauseImpact }>("post", "/pauses", scope, body(false));
        setBusy(false);
        setImpact(r.ok ? r.data.impact : null);
        setError(r.ok ? "" : teacherError(r));
    };
    const confirm = async () => {
        setBusy(true);
        const r = await edu<{ created: boolean }>("post", "/pauses", scope, body(true));
        setBusy(false);
        setError(r.ok ? (r.data.created ? "" : "已有相同的暂停，未重复创建") : teacherError(r));
        if (r.ok) {
            setImpact(null);
            setReason("");
            void load();
        }
    };
    const lift = async (p: Pause) => {
        if (note.trim().length < 4) {
            setError("解除前请写明 4 字以上的说明");
            return;
        }
        setBusy(true);
        const r = await edu<{ lifted: boolean }>("post", `/pauses/${encodeURIComponent(p.id)}/lift`, scope, { note: note.trim() });
        setBusy(false);
        setError(r.ok ? "" : teacherError(r));
        if (r.ok) {
            setNote("");
            void load();
        }
    };

    return (
        <div data-testid="pauses" className="text-sm">
            {list?.platformGenerationPaused && <p className="mb-2 font-medium text-red-600">平台已暂停全部新生成（运维开关），解除需联系平台运维。</p>}
            <div className="flex flex-wrap items-center gap-2">
                <select
                    className={input}
                    aria-label="暂停范围"
                    value={kind}
                    onChange={(e) => {
                        setKind(e.target.value as PauseScope);
                        setImpact(null);
                    }}
                >
                    {(Object.keys(SCOPE_LABEL) as PauseScope[]).map((k) => (
                        <option key={k} value={k}>
                            {SCOPE_LABEL[k]}
                        </option>
                    ))}
                </select>
                {kind !== "generation" && (
                    <input
                        className={input}
                        aria-label="暂停对象"
                        placeholder={TARGET_HINT[kind]}
                        value={target}
                        onChange={(e) => {
                            setTarget(e.target.value);
                            setImpact(null);
                        }}
                    />
                )}
                <input className={`${input} min-w-48 flex-1`} aria-label="暂停原因" placeholder="原因" value={reason} onChange={(e) => setReason(e.target.value)} />
                <button type="button" className={btn} disabled={busy} onClick={() => void preview()}>
                    预览影响
                </button>
            </div>
            {impact && (
                <div className="mt-2 rounded-md bg-amber-50 p-2 dark:bg-amber-950">
                    <p>{impactText(impact)}</p>
                    <button type="button" className={`${btn} mt-2`} disabled={busy} onClick={() => void confirm()}>
                        确认暂停
                    </button>
                </div>
            )}
            {error && (
                <p className="mt-2 text-red-600" role="alert">
                    {error}
                </p>
            )}
            <p className="mt-2 text-xs text-stone-500">回退到旧版本：在任务页把任务迁移到仍有效的旧发布版。已撤回的错误内容在解除暂停后仍保持撤回。</p>
            {list && (
                <ul className="mt-2" aria-label="暂停记录">
                    {list.pauses.length === 0 && <li className="text-stone-500">暂无暂停。</li>}
                    {activeFirst(list.pauses).map((p) => (
                        <li key={p.id} className="flex flex-wrap items-center gap-2 border-t border-stone-100 py-1 dark:border-stone-800">
                            <span className={p.state === "active" ? "w-14 text-red-600" : "w-14 text-stone-500"}>{p.state === "active" ? "暂停中" : "已解除"}</span>
                            <span>{SCOPE_LABEL[p.scope] || p.scope}</span>
                            {p.targetId && <span className="font-mono">{p.targetId}</span>}
                            <span className="flex-1 text-stone-600 dark:text-stone-400">{p.reason}</span>
                            {p.state === "active" && (
                                <>
                                    <input className={input} aria-label="解除说明" placeholder="解除说明" value={note} onChange={(e) => setNote(e.target.value)} />
                                    <button type="button" className={btn} disabled={busy} onClick={() => void lift(p)}>
                                        解除
                                    </button>
                                </>
                            )}
                        </li>
                    ))}
                </ul>
            )}
        </div>
    );
}
