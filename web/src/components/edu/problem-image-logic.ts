/**
 * 单题图片上传与框选（DEV-068）。这里只做提交前的提示性检查；
 * 类型、尺寸、元数据清除与权利判断全部由服务端完成，前端检查不算安全边界。
 */
export const MAX_IMAGE_BYTES = 10 << 20;
export const MIN_EDGE = 16;

export type ProblemImage = {
    id: string;
    kind: "original" | "single";
    state: string;
    parentId?: string;
    sourceAssetId: string;
    problemId?: string;
    mime: string;
    width: number;
    height: number;
    bytes: number;
    sha256: string;
    cropX: number;
    cropY: number;
    cropW: number;
    cropH: number;
    createdAt: string;
};
export type Selection = { x: number; y: number; w: number; h: number };

/** 文件的提示性检查：大小与扩展名。返回空字符串表示可以提交。 */
export function fileHint(f: { name: string; size: number; type: string }): string {
    if (f.size <= 0) return "文件是空的";
    if (f.size > MAX_IMAGE_BYTES) return "图片超过 10 MB，请压缩或裁小后再上传";
    const ext = f.name.toLowerCase().split(".").pop() || "";
    if (!["png", "jpg", "jpeg"].includes(ext)) return "只支持 PNG 或 JPEG 图片";
    if (f.type && !["image/png", "image/jpeg"].includes(f.type)) return "只支持 PNG 或 JPEG 图片";
    return "";
}

/** 框选是否落在原图内且不小于最小边长；返回中文错误，空字符串表示有效。 */
export function selectionError(s: Selection, img: { width: number; height: number }): string {
    const v = [s.x, s.y, s.w, s.h];
    if (v.some((n) => !Number.isInteger(n))) return "框选坐标必须是整数像素";
    if (s.x < 0 || s.y < 0) return "框选不能超出图片左上边界";
    if (s.w < MIN_EDGE || s.h < MIN_EDGE) return `框选的宽和高至少 ${MIN_EDGE} 像素`;
    if (s.x + s.w > img.width || s.y + s.h > img.height) return "框选超出图片范围";
    return "";
}

/** 把拖拽得到的显示坐标换算为原图像素并收进图片范围内。scale 为显示宽度 / 原图宽度。 */
export function dragToSelection(a: { x: number; y: number }, b: { x: number; y: number }, scale: number, img: { width: number; height: number }): Selection {
    const k = scale > 0 ? 1 / scale : 1;
    const clamp = (n: number, max: number) => Math.min(Math.max(Math.round(n * k), 0), max);
    const x1 = clamp(Math.min(a.x, b.x), img.width);
    const y1 = clamp(Math.min(a.y, b.y), img.height);
    const x2 = clamp(Math.max(a.x, b.x), img.width);
    const y2 = clamp(Math.max(a.y, b.y), img.height);
    return { x: x1, y: y1, w: x2 - x1, h: y2 - y1 };
}

export function sizeText(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1 << 20) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1 << 20)).toFixed(1)} MB`;
}

/** 已关联本题的单题图排在最前，其余按时间倒序。 */
export function orderImages(list: ProblemImage[], problemId?: string): ProblemImage[] {
    return [...list].sort((a, b) => {
        const pa = problemId && a.problemId === problemId ? 0 : 1;
        const pb = problemId && b.problemId === problemId ? 0 : 1;
        return pa - pb || b.createdAt.localeCompare(a.createdAt);
    });
}
