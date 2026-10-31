import { formOf, LENGTH_UNITS, paramError, setParam, setTargetUnit, setUnit } from "./problem-params-logic";

/** 题目参数表单（DEV-073）：逐项填写参数与单位，改动直接写回结构化输入。 */
export function ProblemParamsForm({ family, json, onChange, disabled }: { family: string; json: string; onChange: (json: string) => void; disabled?: boolean }) {
    const form = formOf(json, family);
    if (!form) return <p className="mt-2 text-xs text-stone-500">没有可逐项填写的参数，可在下方高级区查看结构化输入。</p>;
    const unitSelect = (label: string, value: string, set: (u: string) => void) => (
        <select aria-label={label} className="h-9 rounded-md border px-2 text-sm" value={value} disabled={disabled} onChange={(e) => set(e.target.value)}>
            {LENGTH_UNITS.map((u) => (
                <option key={u} value={u}>
                    {u || "无单位"}
                </option>
            ))}
        </select>
    );
    return (
        <fieldset className="mt-2 grid gap-2 sm:grid-cols-2" data-testid="problem-params">
            <legend className="mb-1 text-sm font-medium">题目参数</legend>
            {form.fields.map((f) => {
                const err = paramError(f.value);
                return (
                    <label key={f.key} className="text-sm">
                        <span className="mb-1 block">{f.label}</span>
                        <span className="flex gap-2">
                            <input className="h-9 min-w-0 flex-1 rounded-md border px-2" value={f.value} disabled={disabled} aria-invalid={!!err} data-testid={`param-${f.key}`} onChange={(e) => onChange(setParam(json, f.key, e.target.value))} />
                            {form.hasUnits ? unitSelect(`${f.label} 的单位`, f.unit || "", (u) => onChange(setUnit(json, f.key, u))) : null}
                        </span>
                        {err ? <span className="mt-1 block text-xs text-amber-700">{err}</span> : null}
                    </label>
                );
            })}
            {form.hasUnits ? (
                <label className="text-sm">
                    <span className="mb-1 block">答案单位</span>
                    {unitSelect("答案单位", form.targetUnit || "", (u) => onChange(setTargetUnit(json, u)))}
                </label>
            ) : null}
        </fieldset>
    );
}
