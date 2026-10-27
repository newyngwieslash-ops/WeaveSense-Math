import { useEffect, useState } from "react";

import { formatMicro } from "@/components/edu/ledger-logic";
import { ALERT_LABEL, ALERT_STATE_LABEL, SOURCE_LABEL, alertValueText, availabilityText, durationText, notCollectedText, rateText, sortedAlerts, validJobId, type JobTrace, type Observability } from "@/components/edu/observability-logic";
import { edu, type EduScope } from "@/services/api/edu";

const input = "h-8 rounded-md border border-stone-300 px-2 text-sm dark:border-stone-600 dark:bg-stone-900";

/** DEV-121 运行观测：队列、失败、未知费用与资源泄露告警，以及单个任务的 trace。仅机构管理员。 */
export function ObservabilityPanel({ scope }: { scope: EduScope }) {
    const [hours, setHours] = useState(24);
    const [data, setData] = useState<Observability | null>(null);
    const [error, setError] = useState("");
    const [jobId, setJobId] = useState("");
    const [trace, setTrace] = useState<JobTrace | null>(null);
    const [traceError, setTraceError] = useState("");
    useEffect(() => {
        let live = true;
        void edu<Observability>("get", `/observability?hours=${hours}`, scope).then((r) => {
            if (!live) return;
            setData(r.ok ? r.data : null);
            setError(r.ok ? "" : "无法读取运行观测");
        });
        return () => {
            live = false;
        };
    }, [scope, hours]);
    const lookup = async () => {
        const id = jobId.trim();
        if (!validJobId(id)) {
            setTraceError("请输入任务编号");
            return;
        }
        const r = await edu<JobTrace>("get", `/jobs/${encodeURIComponent(id)}/trace`, scope);
        setTrace(r.ok ? r.data : null);
        setTraceError(r.ok ? "" : "找不到该任务");
    };
    return (
        <div data-testid="observability">
            <div className="mb-2 flex items-center gap-2 text-sm">
                <select className={input} aria-label="观测窗口" value={hours} onChange={(e) => setHours(Number(e.target.value))}>
                    {[1, 24, 168].map((h) => (
                        <option key={h} value={h}>
                            最近 {h === 168 ? "7 天" : `${h} 小时`}
                        </option>
                    ))}
                </select>
                {data && <span className={data.firing > 0 ? "font-medium text-red-600" : "text-stone-500"}>{data.firing > 0 ? `${data.firing} 项告警` : "无告警"}</span>}
            </div>
            {error && <p className="text-sm text-red-600">{error}</p>}
            {data && (
                <>
                    <ul className="text-sm" aria-label="告警">
                        {sortedAlerts(data.alerts).map((a) => (
                            <li key={a.id} className="flex gap-2 border-t border-stone-100 py-1 dark:border-stone-800">
                                <span className={a.state === "firing" ? "w-16 text-red-600" : "w-16 text-stone-500"}>{ALERT_STATE_LABEL[a.state]}</span>
                                <span className="flex-1">{ALERT_LABEL[a.id] || a.id}</span>
                                <span className="font-mono">{alertValueText(a)}</span>
                            </li>
                        ))}
                    </ul>
                    <p className="mt-2 text-sm">
                        排队 {data.queue.queued}，执行中 {data.queue.running}，最久排队 {durationText(data.queue.oldestQueuedSeconds)}；提交 {data.submission.attempts} 次作答、{data.submission.selfTests} 次自测、
                        {data.submission.helpRequests} 条求助；可用性 {availabilityText(data)}
                    </p>
                    {data.queue.stuck.length > 0 && <p className="text-sm text-red-600">卡住：{data.queue.stuck.map((s) => `${s.id}（${s.kind}，${s.stage || s.status}，${durationText(s.silentSeconds)}无进展）`).join("；")}</p>}
                    <table className="mt-2 w-full text-sm">
                        <thead>
                            <tr className="text-left text-xs text-stone-500">
                                <th>生成任务</th>
                                <th className="text-right">新建</th>
                                <th className="text-right">成功</th>
                                <th className="text-right">失败</th>
                                <th className="text-right">失败率</th>
                                <th className="text-right">p50</th>
                                <th className="text-right">p95</th>
                            </tr>
                        </thead>
                        <tbody>
                            {data.generation.byKind.map((k) => (
                                <tr key={k.kind} className="border-t border-stone-100 dark:border-stone-800">
                                    <td>{k.kind}</td>
                                    <td className="text-right">{k.created}</td>
                                    <td className="text-right">{k.succeeded}</td>
                                    <td className="text-right">{k.failed}</td>
                                    <td className="text-right">{rateText(k.failureRate)}</td>
                                    <td className="text-right">{durationText(k.p50Seconds)}</td>
                                    <td className="text-right">{durationText(k.p95Seconds)}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                    {data.generation.errorCodes.length > 0 && <p className="mt-1 text-xs text-stone-500">错误码：{data.generation.errorCodes.map((c) => `${c.errorCode} ${c.count}`).join("，")}</p>}
                    <p className="mt-1 text-xs text-stone-500">
                        口径 {data.definition}；告警在读取时计算。{notCollectedText(data)}
                    </p>
                </>
            )}
            <div className="mt-3 flex items-center gap-2 text-sm">
                <input className={input} value={jobId} onChange={(e) => setJobId(e.target.value)} aria-label="任务编号" placeholder="任务编号" />
                <button className="h-8 rounded-md border border-stone-300 px-3 dark:border-stone-600" onClick={() => void lookup()}>
                    查看 trace
                </button>
            </div>
            {traceError && <p className="text-sm text-red-600">{traceError}</p>}
            {trace && (
                <div className="mt-2 text-sm" data-testid="job-trace">
                    <p>
                        {trace.job.kind} · {trace.job.status}
                        {trace.job.errorCode && ` · ${trace.job.errorCode}`} · 结算 {trace.job.settlement} · 用量 {formatMicro(trace.usage.costMicro)}，{trace.usage.computeMs} 毫秒
                    </p>
                    <ol className="mt-1 text-xs">
                        {trace.steps.map((s, i) => (
                            <li key={i}>
                                {new Date(s.at).toLocaleString("zh-CN", { hour12: false })} {SOURCE_LABEL[s.source]} {s.type}
                                {s.status && ` → ${s.status}`}
                            </li>
                        ))}
                    </ol>
                </div>
            )}
        </div>
    );
}
