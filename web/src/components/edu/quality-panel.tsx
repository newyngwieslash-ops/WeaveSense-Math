import { qualityDimensions, STATUS_TEXT, type Quality } from "./quality-panel-logic";

const TONE = { ok: "border-emerald-400", warn: "border-amber-400", bad: "border-red-400", none: "border-stone-300" } as const;

/** 教学质量面板（DEV-075）：三个维度分开展示，不合成分数；音频不自动播放。 */
export function QualityPanel({ quality }: { quality: Quality }) {
    return (
        <section aria-labelledby="quality-title" className="rounded-md border border-stone-200 p-2 text-sm dark:border-stone-700">
            <h4 id="quality-title" className="font-medium">
                教学质量面板
            </h4>
            <p className="text-xs text-stone-500">数学、教学、呈现分别记录，互不替代，不合成总分。</p>
            <ul className="mt-1 grid gap-2 md:grid-cols-2">
                {qualityDimensions(quality).map((d) => (
                    <li key={d.key} className={`rounded-md border p-2 ${TONE[d.status]}`}>
                        <p className="font-medium">
                            {d.title}：{STATUS_TEXT[d.status]}
                        </p>
                        {d.lines.map((l) => (
                            <p key={l} className="text-xs">
                                {l}
                            </p>
                        ))}
                    </li>
                ))}
            </ul>
        </section>
    );
}
