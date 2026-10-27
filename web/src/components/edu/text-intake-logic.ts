/**
 * 文字题目识别（DEV-031）。老师粘贴题目文字，数学服务按题族的固定整句表述识别（不按文本相似度套题），
 * 得到结构化草稿；草稿不保存、不扣费，仍需老师核对参数、保存并确认题意后才能分析。
 * 识别不了时给出原因和下一步：用结构化表单填写，或人工编写讲解。图片识别尚未提供。
 */
export const INTAKE_MAX = 600;

export type IntakeSummary = { object: string; conditions: string[]; target: string; unit: string; method: string; method_stated: boolean };
export type IntakeDraft = {
    state: "needs_confirmation";
    family_id: string;
    structured: Record<string, unknown>;
    summary: IntakeSummary;
    problem_text: string;
};

/** 提交前的本地检查，与服务端限制一致；返回空串表示可以提交。 */
export function intakeTextProblem(text: string): string {
    const t = text.trim();
    if (!t) return "请先粘贴题目文字";
    if ([...t].length > INTAKE_MAX) return `题目文字超过 ${INTAKE_MAX} 字，请删减或改用结构化表单`;
    return "";
}

/** 识别结果摘要，逐行给老师核对。 */
export function draftLines(d: IntakeDraft): string[] {
    const s = d.summary;
    return [`题型：${s.object}`, `条件：${s.conditions.join("；")}`, `所求：${s.target}`, `单位：${s.unit || "未标单位"}`, `方法：${s.method}${s.method_stated ? "（题目指定）" : "（题目未指定，使用默认方法）"}`];
}

/**
 * 识别失败时的原因。只采用服务端单独给出的 reason 字段（数学服务固定的中文说明），
 * 不显示服务端 message 原文；不像固定说明的内容一律丢弃，只按错误码说明。
 */
export function intakeReason(detail: unknown): string {
    const e = (detail as { error?: { reason?: unknown } } | undefined)?.error;
    const r = typeof e?.reason === "string" ? e.reason : "";
    if (!r || [...r].length > 200 || /[<>\n\r]/.test(r) || !/^[\u4e00-\u9fff]/.test(r)) return "";
    return r;
}

/** 识别失败后的下一步，按错误码指向结构化表单或人工编写讲解。 */
export function intakeNextStep(code: string): string {
    if (code === "UNSUPPORTED_FAMILY") return "改用“目录外题目（人工编写讲解）”：保存并确认题意后，在第 2 步人工编写讲解（未经自动验证）";
    if (code === "INPUT_AMBIGUOUS" || code === "CONDITION_CONFLICT" || code === "PARAMETER_LIMIT" || code === "METHOD_MISMATCH") return "请在下方结构化表单中直接填写参数，或修改题目文字后重新识别";
    return "";
}
