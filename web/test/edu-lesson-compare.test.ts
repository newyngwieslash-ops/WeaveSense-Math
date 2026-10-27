import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";

import { changeLabel, changeValue, diffEdits, draftOf, draftProblems, EDIT_CLASS_LABEL, type Editable } from "../src/components/edu/lesson-edit-logic";

// DEV-073：图形文字编辑与版本对比。
const base: Editable = {
    lessonRevisionId: "elr_1",
    lessonHash: "sha256:abc",
    state: "approved",
    beats: [{ id: "s1", title: "找垂足", say: "过顶点作底面垂线。", board: ["PO ⊥ 底面"] }],
    hints: [],
    pronunciations: null,
    editReviewInherit: "none",
    latestRevisionId: "elr_1",
    limits: { maxEdits: 50, maxText: 600, maxBoardLines: 12, maxLine: 200 },
    figure: { description: "拖动旋转", prediction: { id: "p1", prompt: "哪个角大？", options: ["甲", "乙"] } },
};

describe("figure text edits", () => {
    test("untouched figure has no edits, including drafts saved before figure editing existed", () => {
        expect(diffEdits(base, draftOf(base))).toEqual([]);
        const old = draftOf(base);
        delete old.figure;
        expect(diffEdits(base, old)).toEqual([]);
    });
    test("description, prompt and options become figure edits", () => {
        const d = draftOf(base);
        d.figure = { description: " 先观察 ", prompt: "先猜一猜", options: "甲\n\n乙\n说不准\n" };
        expect(diffEdits(base, d)).toEqual([
            { target: "figure", field: "description", value: "先观察" },
            { target: "figure", field: "prediction_prompt", value: "先猜一猜" },
            { target: "figure", field: "prediction_options", value: ["甲", "乙", "说不准"] },
        ]);
    });
    test("a figure without prediction only offers the description", () => {
        const e = { ...base, figure: { description: "拖动旋转", prediction: null } };
        const d = draftOf(e);
        d.figure = { description: "拖动旋转", prompt: "x", options: "a\nb" };
        expect(diffEdits(e, d)).toEqual([]);
        expect(diffEdits({ ...base, figure: null }, d)).toEqual([]);
    });
    test("local checks for prediction choices", () => {
        const one = [{ target: "figure" as const, field: "prediction_options" as const, value: ["甲"] }];
        expect(draftProblems(base, one)).toEqual(["预测选项需 2 到 6 个"]);
        const dup = [{ target: "figure" as const, field: "prediction_options" as const, value: ["甲", "甲"] }];
        expect(draftProblems(base, dup)).toEqual(["预测选项不能重复"]);
        expect(draftProblems(base, [{ target: "figure", field: "description", value: "" }])).toEqual(["文字需 1 到 600 字"]);
    });
    test("every server edit class has a label", () => {
        for (const c of ["reasoning", "wording", "pronunciation", "layout", "figure_text"]) expect(EDIT_CLASS_LABEL[c]).toBeTruthy();
    });
});

describe("version comparison text", () => {
    test("labels name where and what changed", () => {
        expect(changeLabel({ section: "beat", key: "s1", field: "say", before: "a", after: "b" })).toBe("步骤 s1 讲解");
        expect(changeLabel({ section: "beat", key: "s3", field: "board", before: null, after: ["x"] })).toBe("步骤 s3 板书（新增）");
        expect(changeLabel({ section: "hint", key: "2", field: "text", before: "h", after: null })).toBe("提示 2 文字（删除）");
        expect(changeLabel({ section: "pronunciation", key: "棱", field: "reading", before: "léng", after: "líng" })).toBe("读音「棱」 读音");
        expect(changeLabel({ section: "figure", key: "prediction", field: "options", before: [], after: [] })).toBe("图形 预测选项");
    });
    test("values render plainly", () => {
        expect(changeValue(null)).toBe("无");
        expect(changeValue(["甲", "乙"])).toBe("甲 / 乙");
        expect(changeValue("")).toBe("空");
        expect(changeValue(3)).toBe("3");
    });
    test("editor wires the compare endpoint and figure fields", () => {
        const src = readFileSync(new URL("../src/components/edu/lesson-editor.tsx", import.meta.url), "utf8");
        expect(src).toContain("/compare?with=");
        expect(src).toContain('data-testid="lesson-compare"');
        expect(src).toContain('data-testid="figure-edit"');
    });
});
