import { useEffect, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";

import { teacherError } from "./teacher-errors";
import { blockerText, GOAL_LABEL, queueText, stageEstimateText, STYLE_LABEL, type LessonPlan, type Teaching } from "./lesson-plan-logic";

/**
 * 教师端：生成讲解前的目标、风格与时长选择（DEV-070），并显示准入结果、阶段估价与排队。
 * 目标与风格只记录在讲解元数据中并影响时长估算，不改动已验证的数学内容。
 */
export function LessonPlanPanel({ scope, solutionId, refresh, value, onChange }: { scope: EduScope; solutionId: string; refresh: number; value: Teaching; onChange: (t: Teaching) => void }) {
    const [plan, setPlan] = useState<LessonPlan | null>(null);
    const [err, setErr] = useState("");
    useEffect(() => {
        if (!solutionId) return;
        void edu<LessonPlan>("get", `/solution-revisions/${solutionId}/lesson-plan`, scope).then((r) => (r.ok ? (setPlan(r.data), setErr("")) : setErr(teacherError(r))));
    }, [scope, solutionId, refresh]);
    if (err)
        return (
            <p role="status" className="text-sm text-amber-700">
                {err}
            </p>
        );
    if (!plan) return null;
    const minutes = Array.from({ length: plan.teaching.maxMinutes - plan.teaching.minMinutes + 1 }, (_, i) => plan.teaching.minMinutes + i);
    return (
        <div className="mb-2 space-y-2 text-sm" data-testid="lesson-plan">
            <div className="flex flex-wrap gap-2">
                <select aria-label="教学目标" className="h-8 rounded-md border px-2" value={value.goal} onChange={(e) => onChange({ ...value, goal: e.target.value })}>
                    {["", ...plan.teaching.goals].map((g) => (
                        <option key={g} value={g}>
                            目标：{GOAL_LABEL[g] || g}
                        </option>
                    ))}
                </select>
                <select aria-label="讲解风格" className="h-8 rounded-md border px-2" value={value.style} onChange={(e) => onChange({ ...value, style: e.target.value })}>
                    {plan.teaching.styles.map((s) => (
                        <option key={s} value={s}>
                            风格：{STYLE_LABEL[s] || s}
                        </option>
                    ))}
                </select>
                <select aria-label="目标时长" className="h-8 rounded-md border px-2" value={value.targetMinutes} onChange={(e) => onChange({ ...value, targetMinutes: Number(e.target.value) })}>
                    {minutes.map((m) => (
                        <option key={m} value={m}>
                            时长约 {m} 分钟
                        </option>
                    ))}
                </select>
            </div>
            <p className="text-stone-500">
                {stageEstimateText(plan)}；{queueText(plan)}
            </p>
            {plan.admitted ? null : (
                <ul className="list-disc pl-5 text-amber-800 dark:text-amber-200" role="alert">
                    {plan.blockers.map((b) => (
                        <li key={b.code}>{blockerText(b)}</li>
                    ))}
                </ul>
            )}
        </div>
    );
}
