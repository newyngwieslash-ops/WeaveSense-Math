import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";

import { RenderCheckPanel } from "../src/components/edu/render-check-panel";
import { blockingCodeLabel, checkLine, renderCheckHeadline, type RenderCheck } from "../src/components/edu/render-check-logic";

const base: RenderCheck = {
    state: "passed",
    scope: "presentation",
    mathVerification: false,
    checker: "render-check/1.0.0",
    renderer: "preview-svg/1.1.0",
    environment: "isolated_subprocess_empty_env",
    blocking: [],
    warnings: [],
    checks: [
        { id: "clean_reproduction", status: "pass" },
        { id: "audio_sync", status: "not_applicable" },
    ],
};

describe("render check panel (DEV-082)", () => {
    test("headline per state, and old revisions say the check did not run", () => {
        expect(renderCheckHeadline(base)).toBe("呈现检查全部通过");
        expect(renderCheckHeadline({ ...base, state: "warnings", warnings: ["label_occlusion"] })).toContain("1 项提醒");
        expect(renderCheckHeadline({ ...base, state: "failed", blocking: ["no_host_paths"] })).toContain("不可通过审核");
        expect(renderCheckHeadline(null)).toContain("未运行");
    });
    test("lines carry text, not only color, and audio is not applicable rather than passed", () => {
        expect(checkLine({ id: "label_occlusion", status: "warn", count: 2 })).toBe("标注互不遮挡：提醒，2 处");
        expect(checkLine({ id: "audio_sync", status: "not_applicable" })).toContain("不适用");
        expect(blockingCodeLabel("render_check:no_host_paths")).toBe("呈现检查未通过：无本机路径与 file 链接");
    });
    test("panel never claims math verification and alerts on failure", () => {
        const ok = renderToStaticMarkup(<RenderCheckPanel check={base} />);
        expect(ok).toContain("不代表数学验证");
        expect(ok).not.toContain('role="alert"');
        const bad = renderToStaticMarkup(<RenderCheckPanel check={{ ...base, state: "failed", blocking: ["glyph_coverage"], checks: [{ id: "glyph_coverage", status: "fail", count: 1 }] }} />);
        expect(bad).toContain('role="alert"');
        expect(bad).toContain("字形均可显示：未通过，1 处");
    });
});
