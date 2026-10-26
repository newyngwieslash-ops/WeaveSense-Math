import { describe, expect, test } from "bun:test";

import { keyAction, shownBeat, step } from "../src/components/edu/classroom-logic";

describe("classroom mode (DEV-077)", () => {
    const ans = { id: "b3", title: "结论", say: "所以答案是 60 度", board: ["θ = 60°"], reveals_answer: true };
    test("answer beats stay masked until the teacher reveals them", () => {
        const s = shownBeat(ans, new Set());
        expect(s.masked).toBe(true);
        expect(JSON.stringify(s)).not.toContain("60");
        expect(shownBeat(ans, new Set(["b3"]))).toEqual({ title: "结论", lines: ["θ = 60°"], caption: "所以答案是 60 度", masked: false });
        expect(shownBeat({ id: "b1", title: "t", say: "s", board: null }, new Set()).masked).toBe(false);
    });
    test("keyboard paging is clamped", () => {
        expect(keyAction("ArrowRight")).toBe("next");
        expect(keyAction("a")).toBe("reveal");
        expect(keyAction("x")).toBeNull();
        expect(step(0, 3, "prev")).toBe(0);
        expect(step(2, 3, "next")).toBe(2);
        expect(step(1, 3, "last")).toBe(2);
        expect(step(0, 0, "next")).toBe(0);
    });
});
