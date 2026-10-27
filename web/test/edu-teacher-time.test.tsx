import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { authorPhase, diaryProblem, durationText, phaseLine, ratio, shouldBeat, teachingMetricLines, timeFootnote, type TeacherTimeView, type TeachingMetrics } from "../src/components/edu/teacher-time-logic";

const src = (p: string) => readFileSync(join(import.meta.dir, "..", p), "utf8");

describe("DEV-117 teacher time and teaching metrics", () => {
    test("beats only while visible and recently active", () => {
        expect(shouldBeat(100_000, 90_000, true)).toBe(true);
        expect(shouldBeat(100_000, 90_000, false)).toBe(false);
        expect(shouldBeat(200_000, 90_000, true)).toBe(false);
        expect(shouldBeat(100_000, 0, true)).toBe(false);
    });

    test("author page phase follows the lesson state", () => {
        expect(authorPhase(undefined, false, false)).toBe("prep");
        expect(authorPhase("ready_for_review", false, false)).toBe("review");
        expect(authorPhase("ready_for_review", true, false)).toBe("edit");
        expect(authorPhase("rejected", false, false)).toBe("edit");
        expect(authorPhase("approved", false, false)).toBe("publish");
        expect(authorPhase("ready_for_review", false, true)).toBe("publish");
    });

    test("durations, diary lines and the footnote state the method", () => {
        expect(durationText(119)).toBe("2 分钟");
        expect(durationText(3 * 3600 + 5 * 60)).toBe("3 小时 5 分钟");
        expect(phaseLine("review", { measuredSeconds: 60, diaryMinutes: 0, diaryEntries: 0, correctedSeconds: 60 })).toBe("审核：实测 1 分钟");
        expect(phaseLine("review", { measuredSeconds: 60, diaryMinutes: 6, diaryEntries: 1, correctedSeconds: 360 })).toBe("审核：实测 1 分钟；时间日记 6 分钟（1 条）；校正后 6 分钟");
        const v = { from: "2026-10-21", to: "2026-10-27", idleThresholdSeconds: 300, definition: "teacher_time/1" } as TeacherTimeView;
        expect(timeFootnote(v)).toContain("空闲超过 5 分钟不计");
        expect(timeFootnote(v)).toContain("teacher_time/1");
    });

    test("diary input follows the server limits", () => {
        expect(diaryProblem("2026-10-27", "30", "2026-10-27")).toBe("");
        expect(diaryProblem("2026-10-28", "30", "2026-10-27")).toBe("不能填写未来的日期");
        expect(diaryProblem("2026-10-27", "x", "2026-10-27")).toContain("整数");
        expect(diaryProblem("2026-10-27", "961", "2026-10-27")).toContain("960");
        expect(diaryProblem("", "3", "2026-10-27")).toBe("请选择日期");
    });

    test("teaching metrics keep denominators, small samples and demo data apart", () => {
        expect(ratio(1, 0)).toBe("1/0（不可计算）");
        expect(ratio(1, 3)).toBe("1/3（33%）");
        const m: TeachingMetrics = {
            denominator: 3,
            attempted: 2,
            outcomes: { independent: 1, assisted: 1, not_correct: 0, pending: 0, missing: 1 },
            firstTry: { correct: 1, of: 2 },
            independentSelfTests: 0,
            excludedOutsideRoster: 1,
            smallSample: true,
            demo: { classes: 1, records: 12, included: false },
            version: { definition: "teaching_metrics/1", publicationId: "epu_1", lessonRevisionId: "elr_1" },
        };
        const l = teachingMetricLines(m).join("\n");
        expect(l).toContain("独立答对 1/3（33%）");
        expect(l).toContain("看提示后答对 1/3（33%）");
        expect(l).toContain("缺测（未有效作答）1/3（33%）");
        expect(l).toContain("已离班学生 1 人的作答不计入");
        expect(l).toContain("样本少于 5 人");
        expect(l).toContain("12 条虚构记录单独存放");
        expect(l).toContain("teaching_metrics/1");
    });

    test("pages report phases, not content", () => {
        expect(src("src/pages/edu/author.tsx")).toContain("useTeacherBeat(scope, authorPhase(");
        expect(src("src/pages/edu/teacher.tsx")).toContain('useTeacherBeat(scope, "after_class", asId)');
        expect(src("src/pages/edu/home.tsx")).toContain("<TeacherTimePanel scope={scope} />");
        const hook = src("src/components/edu/teacher-time.tsx");
        expect(hook).toContain('{ phase, objectId: objectId || "" }');
        expect(hook).not.toContain("innerText");
    });
});
