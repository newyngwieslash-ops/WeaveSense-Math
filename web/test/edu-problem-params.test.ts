import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { formOf, LENGTH_UNITS, paramError, PARAM_LABELS, setParam, setTargetUnit, setUnit } from "../src/components/edu/problem-params-logic";

const angle = JSON.stringify({ family_id: "SOLID-ANGLE-02", input_state: "sufficient", unit: "", params: { a: "2", h: "3" } }, null, 2);
const vol = JSON.stringify({ family_id: "SOLID-VOL-01", input_state: "sufficient", params: { l: "3", w: "20", h: "5" }, units: { l: "cm", w: "mm", h: "cm" }, target_unit: "cm" }, null, 2);

describe("problem parameter form (DEV-073)", () => {
    test("reads labelled fields in family order", () => {
        const f = formOf(angle, "SOLID-ANGLE-02");
        expect(f?.fields).toEqual([
            { key: "a", label: "底面边长 a", value: "2", unit: undefined },
            { key: "h", label: "高 h", value: "3", unit: undefined },
        ]);
        expect(f?.hasUnits).toBe(false);
    });
    test("edits write back without touching other keys", () => {
        const next = JSON.parse(setParam(angle, "a", "2*sqrt(3)"));
        expect(next).toEqual({ family_id: "SOLID-ANGLE-02", input_state: "sufficient", unit: "", params: { a: "2*sqrt(3)", h: "3" } });
    });
    test("volume units and the target unit are editable", () => {
        const f = formOf(vol, "SOLID-VOL-01");
        expect(f?.fields.map((x) => [x.key, x.unit])).toEqual([
            ["l", "cm"],
            ["w", "mm"],
            ["h", "cm"],
        ]);
        expect(f?.targetUnit).toBe("cm");
        const next = JSON.parse(setTargetUnit(setUnit(vol, "w", "cm"), "m"));
        expect(next.units).toEqual({ l: "cm", w: "cm", h: "cm" });
        expect(next.target_unit).toBe("m");
    });
    test("invalid JSON or no params gives no form and is never rewritten", () => {
        expect(formOf("{", "SOLID-ANGLE-02")).toBeNull();
        expect(formOf(JSON.stringify({ family_id: "MANUAL", params: {} }), "MANUAL")).toBeNull();
        expect(setParam("{", "a", "1")).toBe("{");
    });
    test("format hints mirror the service parser whitelist", () => {
        expect(paramError("3/2")).toBe("");
        expect(paramError("√3")).toBe("");
        expect(paramError("2*sqrt(3)/3")).toBe("");
        expect(paramError("")).toBe("请填写");
        expect(paramError("1.5")).not.toBe("");
        expect(paramError("__import__('os')")).not.toBe("");
        expect(paramError("1".repeat(49))).not.toBe("");
    });
    test("labels and units match the math service", () => {
        const root = join(import.meta.dir, "..", "..", "services", "math-service", "mathsvc");
        const vol = readFileSync(join(root, "families", "solid_vol_01.py"), "utf8");
        expect(vol).toContain(`UNITS = (${LENGTH_UNITS.map((u) => `"${u}"`).join(", ")})`);
        const author = readFileSync(join(import.meta.dir, "..", "src", "pages", "edu", "author.tsx"), "utf8");
        for (const [fam, labels] of Object.entries(PARAM_LABELS)) {
            const m = author.match(new RegExp(`"${fam}":[\\s\\S]*?params: \\{([^}]*)\\}`));
            const keys = [...(m?.[1] || "").matchAll(/(\w+):/g)].map((x) => x[1]);
            expect(keys.sort()).toEqual(Object.keys(labels).sort());
        }
    });
    test("the author page uses the form and keeps raw JSON behind an advanced toggle", () => {
        const author = readFileSync(join(import.meta.dir, "..", "src", "pages", "edu", "author.tsx"), "utf8");
        expect(author).toContain("<ProblemParamsForm");
        expect(author.indexOf("高级：查看结构化输入")).toBeLessThan(author.indexOf('aria-label="结构化题目输入（JSON）"'));
        const editor = readFileSync(join(import.meta.dir, "..", "src", "components", "edu", "lesson-editor.tsx"), "utf8");
        expect(editor).toContain('data-testid="geometry-exercises"');
    });
});
