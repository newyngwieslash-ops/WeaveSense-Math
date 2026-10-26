import { describe, expect, test } from "bun:test";

import { canApprove, canReject, checklistComplete, parseFindings, REVIEW_ITEMS, targetLabel, type Checklist } from "../src/components/edu/review-checklist-logic";

const all = (v: "pass" | "issue" | "na"): Checklist => Object.fromEntries(REVIEW_ITEMS.map((k) => [k, v])) as Checklist;

describe("review checklist (DEV-076)", () => {
    test("approval needs a complete, clean checklist and no findings or blocking errors", () => {
        expect(canApprove(all("pass"), [], "")).toBe(true);
        expect(canApprove({ ...all("pass"), self_test: "na" }, [], "")).toBe(true);
        const partial = all("pass");
        delete partial.hints;
        expect(checklistComplete(partial)).toBe(false);
        expect(canApprove(partial, [], "")).toBe(false);
        expect(canApprove({ ...all("pass"), visuals: "issue" }, [], "")).toBe(false);
        expect(canApprove(all("pass"), [{ target: "problem", item: "problem", note: "x" }], "")).toBe(false);
        expect(canApprove(all("pass"), [], "v1:blocking")).toBe(false);
    });
    test("rejection needs a reason", () => {
        expect(canReject({}, [], "  ")).toBe(false);
        expect(canReject({}, [], "重做")).toBe(true);
        expect(canReject({ method: "issue" }, [], "")).toBe(true);
        expect(canReject({}, [{ target: "beat:b1", item: "method", note: "x" }], "")).toBe(true);
    });
    test("targets read as steps and hints", () => {
        const beats = [
            { id: "b0", title: "建系" },
            { id: "b1", title: "求法向量" },
        ];
        expect(targetLabel("beat:b1", beats)).toBe("第 2 步 求法向量");
        expect(targetLabel("hint:3")).toBe("提示 3");
        expect(targetLabel("answer")).toBe("答案");
        expect(parseFindings("not json")).toEqual([]);
        expect(parseFindings('[{"target":"beat:b0","item":"visuals","note":"n"}]')[0].target).toBe("beat:b0");
    });
});
