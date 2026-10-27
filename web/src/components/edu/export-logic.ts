/**
 * 内容与学习报告导出（DEV-103）。服务端生成不含脚本和外部资源的 HTML，并在导出时复核来源权利；
 * 学生版只来自服务端学生投影，教师版需单独选择。这里只负责文件名与保存。
 */
export type ExportFile = { filename: string; html: string };

/** 只保留字母、数字、下划线、连字符和点，避免路径或特殊字符进入下载文件名。 */
export function safeFilename(name: string): string {
    const base = name.replace(/[^A-Za-z0-9_.-]/g, "_").replace(/^\.+/, "");
    return (base || "export").slice(0, 120).endsWith(".html") ? base.slice(0, 120) : `${(base || "export").slice(0, 115)}.html`;
}

export const EXPORT_LABEL = {
    student: "导出学生版",
    teacher: "导出教师版（含答案）",
    report: "导出学习报告（班级汇总）",
};

export const EXPORT_NOTE = {
    student: "学生版不含未开放的答案、讲解正文和会透露答案的提示。",
    teacher: "教师版含答案与完整讲解，不要直接发给学生。",
    report: "学习报告只含班级汇总，1 至 2 人的格子显示为“少于3”，不含学生姓名、学号或作答。",
};

export function saveHtml(f: ExportFile) {
    const url = URL.createObjectURL(new Blob([f.html], { type: "text/html;charset=utf-8" }));
    const a = document.createElement("a");
    a.href = url;
    a.download = safeFilename(f.filename);
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
}
