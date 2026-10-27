import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { renderToStaticMarkup } from "react-dom/server";

import { ManualLessonForm } from "../src/components/edu/manual-lesson";
import { buildManualLesson, emptyManualDraft, isManualLesson, MANUAL_NOTICE_STUDENT, type ManualDraft } from "../src/components/edu/manual-lesson-logic";
import { qualityDimensions, type Quality } from "../src/components/edu/quality-panel-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

const good: ManualDraft = {
    title: "数列求和",
    problemText: "已知数列 a(n) = 2n − 1，求前 10 项和。",
    beats: [
        { title: "观察", lines: "首项为 1，公差为 2。", revealsAnswer: false },
        { title: "求和", lines: "用首末项求和公式。\n\n所以和为 100。", revealsAnswer: true },
    ],
    answer: "100",
    hints: "先判断数列类型。\n",
};

describe("DEV-039 manual lesson", () => {
    test("builds the service payload with trimmed lines and no empty entries", () => {
        const r = buildManualLesson(good);
        expect(r.ok).toBe(true);
        if (!r.ok) return;
        expect(r.lesson.beats[1].lines).toEqual(["用首末项求和公式。", "所以和为 100。"]);
        expect(r.lesson.beats[1].reveals_answer).toBe(true);
        expect(r.lesson.answer).toEqual({ plain: "100" });
        expect(r.lesson.hints).toEqual(["先判断数列类型。"]);
        expect(Object.keys(r.lesson).sort()).toEqual(["answer", "beats", "hints", "problem_text", "title"]);
    });

    test("refuses incomplete or unsafe content with a next step, before any request", () => {
        const cases: [ManualDraft, string][] = [
            [emptyManualDraft(), "请填写标题"],
            [{ ...good, problemText: "<b>x</b>" }, "尖括号"],
            [{ ...good, beats: good.beats.map((b) => ({ ...b, revealsAnswer: false })) }, "给出答案的那一步"],
            [{ ...good, hints: "1\n2\n3\n4\n5\n6" }, "提示最多 5 条"],
            [{ ...good, beats: [{ title: "a", lines: "", revealsAnswer: true }] }, "1 到 8 行"],
            [{ ...good, answer: "x".repeat(81) }, "不能超过 80"],
        ];
        for (const [d, want] of cases) {
            const r = buildManualLesson(d);
            expect(r.ok).toBe(false);
            if (!r.ok) {
                expect(r.error).toContain(want);
                expect(r.error).toContain("下一步");
            }
        }
    });

    test("is labelled as not verified for teachers, students and the quality panel", () => {
        expect(isManualLesson({ verification: "manual_unverified" })).toBe(true);
        expect(isManualLesson({})).toBe(false);
        expect(MANUAL_NOTICE_STUDENT).toContain("未经自动数学验证");
        const q: Quality = {
            math: { state: "manual_unverified", solutionStatus: "manual", evidenceLevels: [], domainProof: false, sampledOnly: false, source: "teacher_manual" },
            teaching: { state: "approved", humanOnly: true, selfReview: true, checklist: { pass: 6, issue: 0, na: 0 }, findings: 0, edited: false, staleReviews: 0, reviewRecords: 1 },
            presentation: { state: "passed", blocking: [], warnings: [], mathVerification: false },
            audio: { state: "not_generated", autoplay: false },
            combinedScore: false,
        };
        const m = qualityDimensions(q)[0];
        expect(m.status).toBe("warn");
        expect(m.lines.join()).toContain("未经自动数学验证");
        expect(m.lines.join()).not.toContain("数学服务独立验证");
        expect(teacherError({ ok: false, code: "MANUAL_UNVERIFIED", status: 409, message: "x" } as never)).toContain("人工批改");
    });

    test("form renders labelled fields and the not verified warning", () => {
        const html = renderToStaticMarkup(<ManualLessonForm scope={{ workspace: "w" }} revisionId="epr_1" onJob={() => {}} />);
        for (const label of ["讲解标题", "题面", "第 1 步标题", "第 1 步板书", "答案", "分级提示"]) expect(html).toContain(`aria-label="${label}"`);
        expect(html).toContain("不经过自动数学验证");
    });

    test("student page shows the notice and has no self-test start for manual lessons", () => {
        const src = readFileSync(join(import.meta.dir, "../src/pages/edu/student.tsx"), "utf8");
        expect(src).toContain("MANUAL_NOTICE_STUDENT");
        expect(src).toMatch(/isManualLesson\(lesson\?\.meta\) && !state\?\.selfTest/);
    });
});
