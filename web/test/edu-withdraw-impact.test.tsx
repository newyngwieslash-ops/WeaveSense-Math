import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { assignmentImpactText, withdrawReasonProblem } from "../src/components/edu/edu-admin-logic";
import { accessRowText } from "../src/components/edu/org-admin-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

describe("DEV-112 withdraw impact", () => {
    test("impact text names students, attempts, help and extensions", () => {
        const t = assignmentImpactText({ students: 30, studentsWithAttempts: 12, attempts: 40, openHelp: 3, extensions: 2 });
        expect(t).toContain("30 名在读学生");
        expect(t).toContain("12 人已作答（共 40 次作答，记录保留不删除）");
        expect(t).toContain("3 条求助尚未处理");
        expect(t).toContain("2 条个人延期");
        expect(assignmentImpactText({ students: 1, studentsWithAttempts: 0, attempts: 0, openHelp: 0, extensions: 0 })).not.toContain("求助");
    });

    test("reason is checked like the server", () => {
        expect(withdrawReasonProblem("题目换成新版本")).toBe("");
        expect(withdrawReasonProblem(" 换题 ")).toContain("至少 4 个字");
        expect(withdrawReasonProblem("<b>换题了</b>")).toContain("尖括号");
        expect(withdrawReasonProblem("换题\n原因")).toContain("换行");
        expect(withdrawReasonProblem("字".repeat(201))).toContain("200");
    });

    test("errors and audit lines are readable", () => {
        expect(teacherError({ code: "ASSIGNMENT_WITHDRAWN", status: 409 })).toContain("任务已经撤回");
        expect(accessRowText({ id: "1", action: "assignment.withdraw", actorId: "u_alice", subject: "ecl_1", createdAt: "", detail: "eas_1 students=2 attempts=2 help=1 extensions=0 | 题目换成新版本" })).toContain("撤回任务（u_alice，对象 ecl_1）");
    });

    test("teacher page previews before confirming", () => {
        const panel = readFileSync(join(import.meta.dir, "../src/components/edu/class-admin.tsx"), "utf8");
        expect(panel).toContain("{ confirm: true, reason: reason.trim() }");
        expect(panel).toContain("查看撤回影响");
        expect(readFileSync(join(import.meta.dir, "../src/pages/edu/teacher.tsx"), "utf8")).toContain("<WithdrawAssignmentPanel");
    });
});
