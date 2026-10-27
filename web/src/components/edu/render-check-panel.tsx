import { checkLine, renderCheckHeadline, STATUS_MARK, type RenderCheck } from "./render-check-logic";

/** 呈现自动检查结果（DEV-082）。只说明呈现，不说明数学是否正确。 */
export function RenderCheckPanel({ check }: { check: RenderCheck | null | undefined }) {
    const tone = !check ? "border-stone-300" : check.state === "failed" ? "border-red-400" : check.state === "warnings" ? "border-amber-400" : "border-emerald-400";
    return (
        <section aria-labelledby="render-check-title" className={`rounded-md border p-2 text-sm ${tone}`}>
            <h4 id="render-check-title" className="font-medium">
                呈现自动检查
            </h4>
            <p role={check?.state === "failed" ? "alert" : undefined}>{renderCheckHeadline(check)}</p>
            <p className="text-xs text-stone-500">呈现检查只核对页面能否在干净环境中完整、清楚地显示，不代表数学验证；数学结论以验证报告为准。</p>
            {check ? (
                <ul className="mt-1 space-y-0.5">
                    {check.checks.map((c) => (
                        <li key={c.id}>
                            <span aria-hidden="true" className="mr-1 inline-block w-3 text-center">
                                {STATUS_MARK[c.status]}
                            </span>
                            {checkLine(c)}
                        </li>
                    ))}
                </ul>
            ) : null}
        </section>
    );
}
