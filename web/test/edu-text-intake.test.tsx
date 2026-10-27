import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";

import { TextIntake } from "../src/components/edu/text-intake";
import { draftLines, INTAKE_MAX, intakeNextStep, intakeReason, intakeTextProblem, type IntakeDraft } from "../src/components/edu/text-intake-logic";

const draft: IntakeDraft = {
    state: "needs_confirmation",
    family_id: "SOLID-ANGLE-01",
    structured: { family_id: "SOLID-ANGLE-01", input_state: "sufficient", unit: "", params: { e: "2" } },
    summary: { object: "正方体线面角", conditions: ["棱长 2"], target: "直线 A1C 与底面 ABCD 所成角的正弦值", unit: "", method: "坐标法", method_stated: false },
    problem_text: "正方体 ABCD-A1B1C1D1 的棱长为 2。",
};

describe("DEV-031 text intake", () => {
    test("checks the text locally with the same limit as the service", () => {
        expect(intakeTextProblem("  ")).toContain("请先粘贴");
        expect(intakeTextProblem("正".repeat(INTAKE_MAX + 1))).toContain(`${INTAKE_MAX}`);
        expect(intakeTextProblem("正".repeat(INTAKE_MAX))).toBe("");
    });

    test("summarises the draft for the teacher to check, including unit and method source", () => {
        const lines = draftLines(draft);
        expect(lines).toContain("条件：棱长 2");
        expect(lines).toContain("单位：未标单位");
        expect(lines.at(-1)).toContain("题目未指定");
        expect(draftLines({ ...draft, summary: { ...draft.summary, unit: "cm", method: "几何法", method_stated: true } })).toContain("方法：几何法（题目指定）");
    });

    test("shows only the service's fixed reason, never the raw message", () => {
        expect(intakeReason({ error: { code: "UNSUPPORTED_FAMILY", reason: "左焦点不在题族内（题族只覆盖右焦点）", message: "x" } })).toBe("左焦点不在题族内（题族只覆盖右焦点）");
        expect(intakeReason({ error: { code: "X", message: "Traceback ..." } })).toBe("");
        expect(intakeReason({ error: { reason: "Traceback (most recent call last)" } })).toBe("");
        expect(intakeReason({ error: { reason: "左<script>" } })).toBe("");
        expect(intakeReason({ error: { reason: "左".repeat(201) } })).toBe("");
        expect(intakeReason(undefined)).toBe("");
    });

    test("points refusals at the structured form or manual authoring", () => {
        expect(intakeNextStep("UNSUPPORTED_FAMILY")).toContain("人工编写讲解");
        expect(intakeNextStep("UNSUPPORTED_FAMILY")).toContain("未经自动验证");
        expect(intakeNextStep("INPUT_AMBIGUOUS")).toContain("结构化表单");
        expect(intakeNextStep("PROVIDER_UNAVAILABLE")).toBe("");
    });

    test("renders a labelled text box and says images are not supported", () => {
        const html = renderToStaticMarkup(<TextIntake scope={{ workspace: "w" }} onDraft={() => {}} onManual={() => {}} />);
        expect(html).toContain('aria-label="题目文字"');
        expect(html).toContain(`maxLength="${INTAKE_MAX}"`);
        expect(html).toContain("暂不支持图片识别");
        expect(html).toContain("不扣费");
    });

    test("the author page fills the form from the draft and still requires confirmation", () => {
        const src = readFileSync(join(import.meta.dir, "../src/pages/edu/author.tsx"), "utf8");
        expect(src).toContain("<TextIntake");
        expect(src).toContain("setJson(JSON.stringify(d.structured, null, 2))");
        expect(src).toContain("请核对参数后保存并确认题意");
        expect(src).not.toMatch(/onDraft=\{[^}]*confirmRev/);
    });
});
