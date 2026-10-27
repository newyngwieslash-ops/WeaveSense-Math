import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";

import { QualityPanel } from "../src/components/edu/quality-panel";
import { qualityDimensions, type Quality } from "../src/components/edu/quality-panel-logic";

const base: Quality = {
    math: { state: "verified", solutionStatus: "solved", evidenceLevels: ["exact_instance", "conditions_checked", "symbolic_domain_identity", "numeric_sampling"], domainProof: true, sampledOnly: false, source: "math_service_verify" },
    teaching: { state: "not_reviewed", humanOnly: true, selfReview: false, checklist: { pass: 0, issue: 0, na: 0 }, findings: 0, edited: false, staleReviews: 0, reviewRecords: 0 },
    presentation: { state: "passed", blocking: [], warnings: [], mathVerification: false },
    audio: { state: "not_generated", autoplay: false },
    combinedScore: false,
};

describe("DEV-075 teaching quality panel", () => {
    test("keeps math, teaching, presentation and audio as separate dimensions", () => {
        const d = qualityDimensions(base);
        expect(d.map((x) => x.key)).toEqual(["math", "teaching", "presentation", "audio"]);
        expect(d[0].status).toBe("ok");
        expect(d[0].lines.join()).toContain("不含人工审校");
        expect(d[1].status).toBe("none");
        expect(d[1].lines.join()).toContain("系统不自动打分");
        expect(d[2].lines.join()).toContain("不代表数学验证");
        expect(d[3].lines.join()).toContain("不会自动播放");
    });

    test("sampling is never shown as proof and human approval never as verification", () => {
        const sampled = qualityDimensions({ ...base, math: { ...base.math, evidenceLevels: ["exact_instance", "numeric_sampling"], domainProof: false, sampledOnly: true } });
        expect(sampled[0].status).toBe("warn");
        expect(sampled[0].lines.join()).toContain("不等于全域证明");
        const reviewed = qualityDimensions({ ...base, math: { ...base.math, state: "partially_verified" }, teaching: { ...base.teaching, state: "approved", checklist: { pass: 6, issue: 0, na: 1 }, staleReviews: 1 } });
        expect(reviewed[0].status).toBe("warn");
        expect(reviewed[1].status).toBe("ok");
        expect(reviewed[1].lines.join()).toContain("通过 6 项");
        expect(reviewed[1].lines.join()).toContain("针对旧内容");
        const failed = qualityDimensions({ ...base, presentation: { ...base.presentation, state: "failed", blocking: ["render_check:figure_in_view"] } });
        expect(failed[2].status).toBe("bad");
    });

    test("renders without a combined score and without media", () => {
        const html = renderToStaticMarkup(<QualityPanel quality={base} />);
        expect(html).toContain("教学质量面板");
        expect(html).toContain("不合成总分");
        expect(html).not.toMatch(/<audio|<video|autoplay/i);
    });

    test("no edu teacher component autoplays media", () => {
        const root = join(import.meta.dir, "..", "src");
        const files = [...readdirSync(join(root, "components", "edu")).map((f) => join(root, "components", "edu", f)), ...readdirSync(join(root, "pages", "edu")).map((f) => join(root, "pages", "edu", f))].filter((f) => f.endsWith(".tsx"));
        for (const f of files) expect({ f, hit: /autoPlay|\.play\(\)/.test(readFileSync(f, "utf8")) }).toEqual({ f, hit: false });
    });
});
