import { describe, expect, test } from "bun:test";

import { authorLink, todoLines, type HomeView } from "../src/components/edu/home-logic";

const base: HomeView = {
    myLessons: [],
    pendingReview: [],
    classes: [],
    openHelp: 0,
    urgentHelp: 0,
    runningJobs: [],
    openIncidents: 0,
    reviewMode: "self_allowed",
    sample: { sample: true, familyId: "SOLID-ANGLE-02", title: "示例", note: "n" },
    firstRun: true,
};

describe("teacher home (DEV-067)", () => {
    test("incidents and urgent questions come first", () => {
        const v = { ...base, openIncidents: 1, urgentHelp: 2, openHelp: 5, pendingReview: [{ id: "l", title: "t", familyId: "f", state: "ready_for_review", mine: true, canReview: true, createdAt: "" }] };
        expect(todoLines(v)).toEqual(["1 个内容事故未关闭", "2 条内容报错或无法访问的求助", "1 份讲解等待你审核", "3 条学生提问待回复"]);
        expect(todoLines(base)).toEqual([]);
    });
    test("author links carry the workspace", () => {
        expect(authorLink("ws1", { lesson: "elr_1" })).toBe("/edu/author?ws=ws1&lesson=elr_1");
        expect(authorLink("ws1", { sample: "SOLID-ANGLE-02" })).toBe("/edu/author?ws=ws1&sample=SOLID-ANGLE-02");
    });
});
