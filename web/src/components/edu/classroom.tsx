import { useCallback, useEffect, useRef, useState } from "react";

import { edu, type EduScope } from "@/services/api/edu";

import { keyAction, shownBeat, step, type ClassroomBeat } from "./classroom-logic";
import { teacherError } from "./teacher-errors";

/** 课堂演示（DEV-077）：大字号逐步投屏，键盘翻页，含答案步骤需教师揭示。数据来自教师可见的讲解修订。 */
const ctl = "inline-flex h-9 items-center rounded-md border border-stone-400 px-3 text-sm disabled:opacity-40";

export function Classroom({ scope, lessonId, onClose }: { scope: EduScope; lessonId: string; onClose: () => void }) {
    const [beats, setBeats] = useState<ClassroomBeat[]>([]);
    const [i, setI] = useState(0);
    const [revealed, setRevealed] = useState<Set<string>>(new Set());
    const [msg, setMsg] = useState("加载中");
    const root = useRef<HTMLDivElement>(null);

    useEffect(() => {
        void edu<{ beats: ClassroomBeat[] }>("get", `/lesson-revisions/${lessonId}/editable`, scope).then((r) => {
            if (!r.ok) return setMsg(teacherError(r));
            setBeats(r.data.beats || []);
            setMsg("");
        });
    }, [scope, lessonId]);
    useEffect(() => root.current?.focus(), [beats.length]);

    const b = beats[i];
    const act = useCallback(
        (k: ReturnType<typeof keyAction>) => {
            if (!k) return;
            if (k === "reveal") {
                if (b) setRevealed((s) => new Set(s).add(b.id));
                return;
            }
            setI((x) => step(x, beats.length, k));
        },
        [b, beats.length],
    );
    const fullscreen = () => void root.current?.requestFullscreen?.().catch(() => undefined);

    const s = b ? shownBeat(b, revealed) : null;
    return (
        <div
            ref={root}
            tabIndex={-1}
            role="region"
            aria-label="课堂演示"
            data-testid="classroom"
            className="mt-3 rounded-lg bg-stone-950 p-6 text-white outline-none"
            onKeyDown={(e) => {
                if (e.key === "Escape") return onClose();
                const k = keyAction(e.key);
                if (k) {
                    e.preventDefault();
                    act(k);
                }
            }}
        >
            <div className="mb-4 flex flex-wrap items-center gap-2 text-sm text-stone-300">
                <span>课堂演示</span>
                <span>
                    第 {beats.length ? i + 1 : 0} / {beats.length} 步
                </span>
                <button type="button" className={ctl} onClick={fullscreen}>
                    全屏
                </button>
                <button type="button" className={ctl} onClick={onClose}>
                    退出（Esc）
                </button>
                <span>方向键翻页，A 揭示答案</span>
            </div>
            {msg ? (
                <p role="status" aria-live="polite">
                    {msg}
                </p>
            ) : null}
            {s ? (
                <div className="min-h-64" aria-live="polite">
                    <h3 className="mb-4 text-3xl font-semibold">{s.title}</h3>
                    {s.masked ? (
                        <button type="button" className={`${ctl} text-lg`} onClick={() => act("reveal")} data-testid="classroom-reveal">
                            此步含答案，点击或按 A 揭示
                        </button>
                    ) : (
                        <>
                            {s.lines.map((line, k) => (
                                <p key={k} className="whitespace-pre-wrap text-2xl leading-10">
                                    {line}
                                </p>
                            ))}
                            {s.caption ? <p className="mt-6 text-xl text-stone-300">{s.caption}</p> : null}
                        </>
                    )}
                </div>
            ) : null}
            <div className="mt-6 flex gap-2">
                <button type="button" className={ctl} disabled={i === 0} onClick={() => act("prev")}>
                    上一步
                </button>
                <button type="button" className={ctl} disabled={i >= beats.length - 1} onClick={() => act("next")}>
                    下一步
                </button>
            </div>
        </div>
    );
}
