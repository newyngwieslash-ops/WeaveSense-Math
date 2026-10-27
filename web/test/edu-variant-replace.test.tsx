import { describe, expect, test } from "bun:test";

import { blockingLabel, gradingScopeLabel, parseBlocking, replaceParamsError } from "../src/components/edu/edu-admin-logic";

describe("teacher own item replacement (DEV-078)", () => {
    test("params must cover every family parameter with a short numeric expression", () => {
        expect(replaceParamsError(["a", "h"], { a: "5", h: "2" })).toBe("");
        expect(replaceParamsError(["a", "h"], { a: "5" })).toContain("h");
        expect(replaceParamsError(["a"], { a: "sqrt(2)/3" })).toBe("");
        expect(replaceParamsError(["a"], { a: "__import__('os')" })).toContain("数值表达式");
        expect(replaceParamsError(["a"], { a: "1".repeat(40) })).toContain("数值表达式");
    });
    test("blocking errors name the own item and the reason", () => {
        expect(parseBlocking('["teacher_item_dropped:answer_reused"]').map(blockingLabel)).toEqual(["自有题未通过：答案与原题或其他变式重复"]);
        expect(blockingLabel("item_dropped:not_verified:candidate")).toContain("candidate");
        expect(blockingLabel("v1:math_not_verified")).toContain("数学未验证");
        expect(parseBlocking("")).toEqual([]);
        expect(parseBlocking("plain")).toEqual(["plain"]);
    });
    test("grading scope says what is and is not auto graded", () => {
        const s = gradingScopeLabel({ auto: true, covers: "final_answer", equivalence: "exact_value", decimal: "needs_exact", steps: "not_graded", unit: "checked_when_expected" });
        expect(s).toContain("只判最终答案");
        expect(s).toContain("解题过程不自动评分");
        expect(gradingScopeLabel({ auto: false, covers: "none", steps: "not_graded" })).toContain("教师批改");
        expect(gradingScopeLabel(undefined)).toContain("未知");
    });
});
