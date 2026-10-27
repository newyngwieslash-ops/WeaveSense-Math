import { useEffect, useState } from "react";

import { COST_BY, COST_BY_LABEL, costRowLabel, notCollectedText, ratioText, reconciles, rowDetail, type CostBreakdown, type CostBy } from "@/components/edu/cost-breakdown-logic";
import { formatMicro } from "@/components/edu/ledger-logic";
import { edu, type EduScope } from "@/services/api/edu";

type Scope = EduScope;

/** DEV-118 成本分层：失败、重试与人工支持归入同批接受课件；分母为零显示不可计算。 */
export function CostBreakdownPanel({ scope, period }: { scope: Scope; period: string }) {
    const [by, setBy] = useState<CostBy>("family");
    const [data, setData] = useState<CostBreakdown | null>(null);
    const [error, setError] = useState("");
    useEffect(() => {
        if (!/^20\d{2}-(0[1-9]|1[0-2])$/.test(period)) return;
        let live = true;
        void edu<CostBreakdown>("get", `/cost-report/breakdown?period=${period}&by=${by}`, scope).then((r) => {
            if (!live) return;
            setData(r.ok ? r.data : null);
            setError(r.ok ? "" : "无法读取成本分层");
        });
        return () => {
            live = false;
        };
    }, [scope, period, by]);
    return (
        <div className="mt-3" data-testid="cost-breakdown">
            <div className="mb-2 flex items-center gap-2 text-sm">
                <span className="font-medium">成本分层</span>
                <select className="h-8 rounded-md border border-stone-300 px-2 dark:border-stone-600 dark:bg-stone-900" aria-label="分层方式" value={by} onChange={(e) => setBy(e.target.value as CostBy)}>
                    {COST_BY.map((b) => (
                        <option key={b} value={b}>
                            {COST_BY_LABEL[b]}
                        </option>
                    ))}
                </select>
            </div>
            {error && <p className="text-sm text-red-600">{error}</p>}
            {data && (
                <>
                    <table className="w-full text-sm">
                        <thead>
                            <tr className="text-left text-xs text-stone-500">
                                <th>分层</th>
                                <th className="text-right">成本</th>
                                <th className="text-right">接受课件</th>
                                <th className="text-right">每份</th>
                                <th className="text-right">每班次</th>
                                <th className="pl-3">构成</th>
                            </tr>
                        </thead>
                        <tbody>
                            {(data.rows || []).map((r) => (
                                <tr key={r.key} className="border-t border-stone-100 dark:border-stone-800">
                                    <td>{costRowLabel(r.key)}</td>
                                    <td className="text-right font-mono">{formatMicro(r.costMicro)}</td>
                                    <td className="text-right">{r.accepted}</td>
                                    <td className="text-right font-mono">{ratioText(r.perAcceptedMicro, r.accepted)}</td>
                                    <td className="text-right font-mono">{ratioText(r.perClassDeliveryMicro, r.classDeliveries)}</td>
                                    <td className="pl-3 text-xs text-stone-500">{rowDetail(r)}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                    <p className="mt-2 text-xs text-stone-500">
                        合计 {formatMicro(data.totalMicro)}
                        {!reconciles(data) && "（分层合计与总额不一致，请联系运维核对）"}；口径 {data.definition}：失败、取消与重试的计算和归属到课件的人工支持计入同批接受课件，批次成本平均分到其接受课件，再平均分到布置的班级
                        {data.excludedOpenJobs > 0 && `；${data.excludedOpenJobs} 个任务尚未结算，未计入`}。{notCollectedText(data)}
                    </p>
                </>
            )}
        </div>
    );
}
