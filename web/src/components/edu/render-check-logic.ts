/**
 * 呈现自动检查（DEV-082）。数学服务在独立的干净进程中重新渲染预览并检查；
 * 这里只把服务端摘要变成教师能读懂的文字。呈现检查不代表数学验证。
 */
export type RenderCheckItem = { id: string; status: "pass" | "warn" | "fail" | "not_applicable"; count?: number; codes?: string[] };
export type RenderCheck = {
    state: "passed" | "warnings" | "failed";
    scope: string;
    mathVerification: boolean;
    checker: string;
    renderer: string;
    environment: string;
    blocking: string[];
    warnings: string[];
    checks: RenderCheckItem[];
};

export const CHECK_LABEL: Record<string, string> = {
    clean_reproduction: "干净环境重新渲染一致",
    no_external_resources: "无外部资源与脚本",
    no_host_paths: "无本机路径与 file 链接",
    glyph_coverage: "字形均可显示",
    figure_in_view: "图形在画面内",
    label_overflow: "标注未超出画面",
    label_occlusion: "标注互不遮挡",
    figure_text_consistency: "图文一致（题面与图中点名）",
    board_narration: "每步有板书与讲述",
    audio_sync: "音画对应",
    student_page_hides_answer: "学生页不显示未开放答案",
};

export const STATUS_TEXT: Record<RenderCheckItem["status"], string> = { pass: "通过", warn: "提醒", fail: "未通过", not_applicable: "不适用" };
// 非颜色标识：每个状态都有文字和符号，颜色只是辅助
export const STATUS_MARK: Record<RenderCheckItem["status"], string> = { pass: "✓", warn: "!", fail: "✗", not_applicable: "–" };

export function renderCheckHeadline(rc: RenderCheck | null | undefined): string {
    if (!rc) return "此修订生成于呈现检查上线之前，未运行呈现检查";
    if (rc.state === "failed") return `呈现检查未通过（${rc.blocking.length} 项），不可通过审核，请修改后重新生成`;
    if (rc.state === "warnings") return `呈现检查有 ${rc.warnings.length} 项提醒，请在预览中确认`;
    return "呈现检查全部通过";
}

export function checkLine(c: RenderCheckItem): string {
    const label = CHECK_LABEL[c.id] || c.id;
    if (c.status === "not_applicable") return `${label}：不适用（当前讲解没有配音）`;
    const n = c.count ? `，${c.count} 处` : "";
    return `${label}：${STATUS_TEXT[c.status]}${n}`;
}

export function blockingCodeLabel(code: string): string {
    if (code.startsWith("render_check:")) return `呈现检查未通过：${CHECK_LABEL[code.slice(13)] || code.slice(13)}`;
    if (code === "math_not_verified") return "数学未通过验证";
    return code;
}
