import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { MAX_IMAGE_BYTES, dragToSelection, fileHint, orderImages, selectionError, sizeText, type ProblemImage } from "../src/components/edu/problem-image-logic";
import { accessRowText, retentionImpact } from "../src/components/edu/org-admin-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

const src = (p: string) => readFileSync(join(import.meta.dir, "..", p), "utf8");
const img = (o: Partial<ProblemImage>): ProblemImage => ({
    id: "epi_1",
    kind: "single",
    state: "active",
    sourceAssetId: "esa_1",
    mime: "image/png",
    width: 100,
    height: 50,
    bytes: 10,
    sha256: "x",
    cropX: 0,
    cropY: 0,
    cropW: 100,
    cropH: 50,
    createdAt: "2026-10-28T00:00:00Z",
    ...o,
});

describe("DEV-068 problem images", () => {
    test("file hints catch size and type before upload", () => {
        expect(fileHint({ name: "a.png", size: 10, type: "image/png" })).toBe("");
        expect(fileHint({ name: "a.JPEG", size: 10, type: "image/jpeg" })).toBe("");
        expect(fileHint({ name: "a.png", size: MAX_IMAGE_BYTES + 1, type: "image/png" })).toContain("10 MB");
        expect(fileHint({ name: "a.svg", size: 10, type: "image/svg+xml" })).toContain("PNG");
        expect(fileHint({ name: "a.png", size: 10, type: "image/gif" })).toContain("PNG");
        expect(fileHint({ name: "a.png", size: 0, type: "image/png" })).toContain("空");
    });
    test("selection must lie inside the picture", () => {
        const im = { width: 320, height: 200 };
        expect(selectionError({ x: 20, y: 10, w: 160, h: 90 }, im)).toBe("");
        expect(selectionError({ x: 300, y: 0, w: 40, h: 40 }, im)).toContain("超出");
        expect(selectionError({ x: -1, y: 0, w: 40, h: 40 }, im)).toContain("边界");
        expect(selectionError({ x: 0, y: 0, w: 8, h: 40 }, im)).toContain("16");
        expect(selectionError({ x: 0.5, y: 0, w: 40, h: 40 }, im)).toContain("整数");
    });
    test("drag maps display pixels to picture pixels and clamps", () => {
        expect(dragToSelection({ x: 50, y: 40 }, { x: 10, y: 5 }, 0.5, { width: 320, height: 200 })).toEqual({ x: 20, y: 10, w: 80, h: 70 });
        expect(dragToSelection({ x: -10, y: -10 }, { x: 999, y: 999 }, 1, { width: 320, height: 200 })).toEqual({ x: 0, y: 0, w: 320, h: 200 });
    });
    test("attached image first, then newest", () => {
        const l = orderImages([img({ id: "a", createdAt: "2026-10-27T00:00:00Z" }), img({ id: "b" }), img({ id: "c", problemId: "ep_1", createdAt: "2026-10-01T00:00:00Z" })], "ep_1");
        expect(l.map((i) => i.id)).toEqual(["c", "b", "a"]);
        expect(sizeText(2048)).toBe("2.0 KB");
    });
    test("every image error code has teacher text", () => {
        for (const code of ["IMAGE_TOO_LARGE", "UNSUPPORTED_IMAGE", "IMAGE_TYPE_MISMATCH", "IMAGE_DECODE_FAILED", "IMAGE_DIMENSIONS", "IMAGE_BUSY", "IMAGE_NOT_SINGLE"]) {
            expect(teacherError({ status: 422, code })).not.toContain("错误代码");
        }
    });
    test("the author page mounts the panel; no student page touches problem images", () => {
        expect(src("src/pages/edu/author.tsx")).toContain("<ProblemImagePanel scope={scope} problemId={rev?.problemId} />");
        const panel = src("src/components/edu/problem-image.tsx");
        expect(panel).toContain("URL.revokeObjectURL");
        expect(panel).not.toContain("dangerouslySetInnerHTML");
        for (const p of ["src/pages/edu/student.tsx", "src/components/edu/classroom.tsx", "src/components/edu/step-viewer.tsx"]) {
            expect(src(p)).not.toContain("problem-image");
        }
    });
    test("retention preview and audit name unattached problem images", () => {
        expect(retentionImpact({ enrollments: 0, learningRows: {}, rosterEntries: 0, artifacts: 0, problemImages: 2 })).toContain("题图 2 张");
        expect(retentionImpact({ enrollments: 0, learningRows: {}, rosterEntries: 0, artifacts: 0, problemImages: 0 })).toContain("没有需要清理");
        const row = { action: "retention.run", actorId: "u", subject: "t", detail: "enrollments=1 learning_rows=2 roster_entries=0 problem_images=3 artifacts=1", createdAt: "2026-10-28T00:00:00Z" };
        expect(accessRowText(row as Parameters<typeof accessRowText>[0])).toContain("题图 3 张");
        expect(accessRowText({ ...row, detail: "enrollments=1 learning_rows=2 roster_entries=0 artifacts=1" } as Parameters<typeof accessRowText>[0])).toContain("临时文件 1 个");
    });
});
