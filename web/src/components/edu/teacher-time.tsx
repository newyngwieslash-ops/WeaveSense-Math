import { useCallback, useEffect, useRef, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";

import { teacherError } from "./teacher-errors";
import { BEAT_EVERY_MS, diaryProblem, PHASE_LABEL, PHASES, phaseLine, shouldBeat, teachingMetricLines, timeFootnote, type TeacherPhase, type TeacherTimeView, type TeachingMetrics } from "./teacher-time-logic";

/**
 * 教师活跃时长心跳（DEV-117）：页面可见且老师一分钟内有操作时，每分钟报一次当前阶段。
 * 不记录操作内容，只记录阶段与对象编号。
 */
export function useTeacherBeat(scope: EduScope, phase: TeacherPhase, objectId?: string) {
    const lastActive = useRef(0);
    useEffect(() => {
        const mark = () => {
            lastActive.current = Date.now();
        };
        const events = ["pointerdown", "keydown", "wheel", "touchstart"] as const;
        events.forEach((e) => window.addEventListener(e, mark, { passive: true }));
        return () => events.forEach((e) => window.removeEventListener(e, mark));
    }, []);
    useEffect(() => {
        if (!scope.workspace) return;
        const id = window.setInterval(() => {
            if (!shouldBeat(Date.now(), lastActive.current, document.visibilityState === "visible")) return;
            void edu("post", "/teacher-time/beat", scope, { phase, objectId: objectId || "" });
        }, BEAT_EVERY_MS);
        return () => window.clearInterval(id);
    }, [scope, phase, objectId]);
}

const btn = "inline-flex h-8 items-center rounded-md border px-3 text-sm";

function today(): string {
    const d = new Date(Date.now() + 8 * 3600_000);
    return d.toISOString().slice(0, 10);
}

/** 本人最近 7 天的活跃时长与时间日记校正。 */
export function TeacherTimePanel({ scope }: { scope: EduScope }) {
    const [view, setView] = useState<TeacherTimeView | null>(null);
    const [msg, setMsg] = useState("");
    const [day, setDay] = useState(today);
    const [phase, setPhase] = useState<TeacherPhase>("prep");
    const [minutes, setMinutes] = useState("");
    const load = useCallback(() => {
        void edu<TeacherTimeView>("get", "/teacher-time", scope).then((r) => (r.ok ? setView(r.data) : setMsg(teacherError(r))));
    }, [scope]);
    useEffect(() => {
        if (scope.workspace) load();
    }, [scope.workspace, load]);
    const save = async () => {
        const p = diaryProblem(day, minutes, today());
        if (p) return setMsg(p);
        const r = await edu("put", "/teacher-time/diary", scope, { day, phase, minutes: Number(minutes) });
        if (!r.ok) return setMsg(teacherError(r));
        setMsg(`已记录 ${day} ${PHASE_LABEL[phase]} ${minutes} 分钟`);
        setMinutes("");
        load();
    };
    return (
        <section className="mb-4 rounded-lg border bg-white p-4" data-testid="teacher-time">
            <h2 className="mb-2 font-semibold">我的工作时长（最近 7 天）</h2>
            {view ? (
                <>
                    <ul className="text-sm" data-testid="teacher-time-phases">
                        {PHASES.map((p) => (
                            <li key={p}>{phaseLine(p, view.phases[p])}</li>
                        ))}
                    </ul>
                    <p className="mt-1 text-xs text-stone-500">{timeFootnote(view)}</p>
                </>
            ) : (
                <p className="text-sm text-stone-500">正在读取…</p>
            )}
            <div className="mt-2 flex flex-wrap items-center gap-2 text-sm">
                <span>时间日记校正：</span>
                <input aria-label="日期" type="date" className="h-8 rounded-md border px-2" value={day} max={today()} onChange={(e) => setDay(e.target.value)} />
                <select aria-label="阶段" className="h-8 rounded-md border px-2" value={phase} onChange={(e) => setPhase(e.target.value as TeacherPhase)}>
                    {PHASES.map((p) => (
                        <option key={p} value={p}>
                            {PHASE_LABEL[p]}
                        </option>
                    ))}
                </select>
                <input aria-label="分钟" inputMode="numeric" className="h-8 w-20 rounded-md border px-2" value={minutes} onChange={(e) => setMinutes(e.target.value)} placeholder="分钟" />
                <button className={btn} onClick={() => void save()} data-testid="teacher-time-save">
                    记录
                </button>
            </div>
            {msg && (
                <p className="mt-1 text-sm" role="status" data-testid="teacher-time-status">
                    {msg}
                </p>
            )}
        </section>
    );
}

/** 一个任务的教学指标：独立、辅助、缺测与演示分开，均带分母与口径版本。 */
export function TeachingMetricsPanel({ scope, assignmentId }: { scope: EduScope; assignmentId: string }) {
    const [m, setM] = useState<TeachingMetrics | null>(null);
    const [msg, setMsg] = useState("");
    useEffect(() => {
        setM(null);
        setMsg("");
        if (!assignmentId) return;
        void edu<TeachingMetrics>("get", `/assignments/${assignmentId}/teaching-metrics`, scope).then((r) => (r.ok ? setM(r.data) : setMsg(teacherError(r))));
    }, [scope, assignmentId]);
    if (msg) return <p className="text-sm text-red-700">{msg}</p>;
    if (!m) return <p className="text-sm text-stone-500">正在读取…</p>;
    return (
        <ul className="text-sm" data-testid="teaching-metrics">
            {teachingMetricLines(m).map((l) => (
                <li key={l}>{l}</li>
            ))}
        </ul>
    );
}
