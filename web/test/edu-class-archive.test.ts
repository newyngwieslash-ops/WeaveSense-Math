import { describe, expect, test } from "bun:test";

import { archiveImpactText, archiveReasonProblem } from "../src/components/edu/edu-admin-logic";
import { accessRowText, retentionImpact } from "../src/components/edu/org-admin-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

describe("DEV-106 class archive and artifact purge", () => {
    test("archive impact names invites, students and the retention start", () => {
        const t = archiveImpactText({ activeInvites: 2, activeEnrollments: 31 });
        expect(t).toContain("2 个有效邀请码立即撤销");
        expect(t).toContain("31 名在读学生不再看到本班任务");
        expect(t).toContain("从归档时起算");
    });

    test("reason is checked like the server", () => {
        expect(archiveReasonProblem("学期结束归档")).toBe("");
        expect(archiveReasonProblem(" 结束 ")).toContain("至少 4 个字");
        expect(archiveReasonProblem("<b>学期结束</b>")).toContain("尖括号");
        expect(archiveReasonProblem("学期\n结束")).toContain("换行");
        expect(archiveReasonProblem("字".repeat(201))).toContain("200");
    });

    test("errors, impact and audit lines are readable", () => {
        expect(teacherError({ code: "CLASS_ARCHIVED", status: 409 })).toContain("班级已归档");
        expect(retentionImpact({ enrollments: 0, learningRows: { edu_attempts: 4 }, rosterEntries: 0, artifacts: 1, archivedClasses: 1 })).toContain("归档期满的班级 1 个");
        expect(retentionImpact({ enrollments: 0, learningRows: {}, rosterEntries: 0, artifacts: 1 })).toContain("从数学服务存储删除");
        const run = { id: "1", action: "retention.run", actorId: "system:retention", subject: "tn_1", createdAt: "" };
        expect(accessRowText({ ...run, detail: "enrollments=0 archived_classes=2 learning_rows=5 roster_entries=0 problem_images=0 artifacts=1" })).toContain("归档期满班级 2 个，学习记录 5 条");
        expect(accessRowText({ ...run, detail: "enrollments=1 learning_rows=3 roster_entries=0 artifacts=2" })).toContain("学习记录 3 条");
        expect(accessRowText({ ...run, action: "class.archive", actorId: "u_alice", subject: "ecl_1", detail: "invites_revoked=1 enrollments=30 | 学期结束" })).toBe("归档班级（u_alice，对象 ecl_1）；撤销邀请码 1 个，在读学生 30 人；原因：学期结束");
        expect(accessRowText({ ...run, action: "artifact.purge", subject: "ema_1", detail: "purged" })).toContain("删除临时文件内容");
    });
});
