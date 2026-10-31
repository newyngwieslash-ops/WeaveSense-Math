/**
 * 题目参数表单（DEV-073）：老师逐项修改题族参数与单位，不需要编辑 JSON。
 * 图形几何与练习都由参数决定，改参数保存为新的题目修订，重新分析、生成与审核；
 * 已审核与已发布的修订不变（约束 3）。这里只做前端格式检查，服务端解析器才是准入标准。
 */

export const PARAM_LABELS: Record<string, Record<string, string>> = {
    "SOLID-ANGLE-02": { a: "底面边长 a", h: "高 h" },
    "SOLID-ANGLE-01": { e: "棱长 e" },
    "SOLID-VOL-01": { l: "长 l", w: "宽 w", h: "高 h" },
    "CONIC-CHORD-01": { a: "长半轴 a", b: "短半轴 b" },
    "CONIC-SLOPE-01": { a: "长半轴 a", b: "短半轴 b" },
    "CONIC-PARABOLA-01": { p: "焦参数 p" },
};

/** 与数学服务 SOLID-VOL-01 的 UNITS 一致；空串表示不带单位。 */
export const LENGTH_UNITS = ["", "mm", "cm", "dm", "m"] as const;
export const MAX_PARAM_TEXT = 48;

export type ParamField = { key: string; label: string; value: string; unit?: string };
export type ParamForm = { family: string; fields: ParamField[]; targetUnit?: string; hasUnits: boolean };

type Structured = { family_id?: unknown; params?: unknown; units?: unknown; target_unit?: unknown; [k: string]: unknown };

function parse(json: string): Structured | null {
    try {
        const v = JSON.parse(json) as unknown;
        return v && typeof v === "object" && !Array.isArray(v) ? (v as Structured) : null;
    } catch {
        return null;
    }
}

const asRecord = (v: unknown): Record<string, unknown> => (v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : {});

/** 从结构化输入读出表单；JSON 不合法或没有参数时返回 null（老师可在高级区查看原文）。 */
export function formOf(json: string, family: string): ParamForm | null {
    const s = parse(json);
    if (!s) return null;
    const fam = typeof s.family_id === "string" && s.family_id ? s.family_id : family;
    const params = asRecord(s.params);
    const labels = PARAM_LABELS[fam] || {};
    const keys = [...Object.keys(labels).filter((k) => k in params), ...Object.keys(params).filter((k) => !(k in labels))];
    if (!keys.length) return null;
    const units = asRecord(s.units);
    const hasUnits = fam === "SOLID-VOL-01";
    return {
        family: fam,
        hasUnits,
        targetUnit: hasUnits ? String(s.target_unit ?? "") : undefined,
        fields: keys.map((k) => ({ key: k, label: labels[k] || k, value: String(params[k] ?? ""), unit: hasUnits ? String(units[k] ?? "") : undefined })),
    };
}

function write(json: string, change: (s: Structured) => void): string {
    const s = parse(json);
    if (!s) return json;
    change(s);
    return JSON.stringify(s, null, 2);
}

export const setParam = (json: string, key: string, value: string) => write(json, (s) => (s.params = { ...asRecord(s.params), [key]: value }));
export const setUnit = (json: string, key: string, unit: string) => write(json, (s) => (s.units = { ...asRecord(s.units), [key]: unit }));
export const setTargetUnit = (json: string, unit: string) => write(json, (s) => (s.target_unit = unit));

/** 与服务端 params.parse_exact 的字符白名单和长度一致的提示；通过不代表服务端一定接受。 */
export function paramError(value: string): string {
    const v = value.trim().replace(/√\s*(\d+)/g, "sqrt($1)");
    if (!v) return "请填写";
    if (v.length > MAX_PARAM_TEXT) return `不超过 ${MAX_PARAM_TEXT} 个字符`;
    if (!/^[0-9sqrt()+\-*/ ]+$/.test(v)) return "只能用整数、分数与 sqrt()，例如 3/2、2*sqrt(3)";
    return "";
}

/** 讲解编辑器里“图形与练习”一节的说明：两者都不能在讲解修订里直接改。 */
export const GEOMETRY_NOTE = "图形的几何形状由题目参数决定。改参数会保存为新的题目修订，需要重新分析、生成讲解并审核；当前修订和已发布版本保持不变。";
export const EXERCISE_NOTE = "练习题来自举一反三题组。可以在题组里逐题替换并重新审核题组；讲解自带的同构自测题随题目修订一起变化。";
