/**
 * 内容来源标识（DEV-101）。来源由服务端从讲解元数据得出，学生与教师看到同一文字；
 * 前端不改写、不隐藏，缺失时如实显示“生成方式未记录”。
 */
export type Provenance = { ai_generated: boolean; generator: string; teacher_edited: boolean; label: string };

export function provenanceText(p: Provenance | null | undefined): string {
    return `内容来源：${p?.label || "生成方式未记录"}`;
}
