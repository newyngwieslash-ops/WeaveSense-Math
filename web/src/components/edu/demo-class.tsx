import { useCallback, useEffect, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";

import { demoSummaryLines, isLabeledDemo, type DemoView } from "./home-logic";
import { teacherError } from "./teacher-errors";

/**
 * 示例班级（DEV-067）：虚构学生与虚构学习记录，用来熟悉班级反馈面板。
 * 与真实班级、学生、统计、计费与导出完全隔离（服务端独立数据表），教师可随时重置或删除。
 */
const btn = "inline-flex h-8 items-center rounded-md border border-amber-400 px-3 text-sm";

export function DemoClassPanel({ scope, onChange }: { scope: EduScope; onChange?: () => void }) {
    const [view, setView] = useState<DemoView | null>(null);
    const [msg, setMsg] = useState("");
    const load = useCallback(
        (method: "get" | "post", path: string) =>
            void edu<DemoView>(method, path, scope).then((r) => {
                if (!r.ok) return setMsg(teacherError(r));
                if (!isLabeledDemo(r.data)) return setMsg("示例数据缺少演示标注，已拒绝展示");
                setMsg("");
                setView(r.data);
                if (method === "post") onChange?.();
            }),
        [scope, onChange],
    );
    useEffect(() => {
        if (scope.workspace) load("get", "/demo-class");
    }, [scope.workspace, load]);

    return (
        <section className="rounded-lg border border-dashed border-amber-400 p-4" data-testid="demo-class">
            <div className="flex flex-wrap items-center gap-2">
                <span className="rounded bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-800">演示数据</span>
                <h2 className="font-semibold">{view?.class?.name || "示例班级"}</h2>
                <button type="button" className={btn} onClick={() => load("post", "/demo-class/reset")}>
                    {view?.class ? "重置示例班级" : "生成示例班级"}
                </button>
                {view?.class ? (
                    <button type="button" className={btn} onClick={() => load("post", "/demo-class/remove")}>
                        删除示例班级
                    </button>
                ) : null}
            </div>
            <p className="mt-1 text-xs text-amber-800">{view?.label || "虚构班级与虚构学习记录，不是真实学生，不计入任何统计、计费与导出"}</p>
            {msg ? (
                <p role="status" aria-live="polite" className="text-sm text-red-600">
                    {msg}
                </p>
            ) : null}
            {view?.class ? (
                <>
                    <ul className="mt-2 ml-4 list-disc text-sm">
                        {demoSummaryLines(view).map((l) => (
                            <li key={l}>{l}</li>
                        ))}
                    </ul>
                    {view.dashboard?.commonErrors.length ? <p className="mt-1 text-sm">常见错误（虚构）：{view.dashboard.commonErrors.join("；")}</p> : null}
                    <ul className="mt-2 text-xs text-stone-600">
                        {(view.records || [])
                            .filter((rec) => rec.kind === "help")
                            .map((rec) => (
                                <li key={rec.id}>
                                    {rec.studentName}（虚构）：{rec.message}
                                </li>
                            ))}
                    </ul>
                </>
            ) : null}
        </section>
    );
}
