import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { accessRowText, retentionImpact, retentionLine, retentionProblem } from "../src/components/edu/org-admin-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

const bounds = { tempDays: [1, 30], imageDays: [7, 365], learningDays: [30, 1095], backupDays: [1, 30] } as const;
const days = { tempDays: 7, imageDays: 90, learningDays: 180, backupDays: 30 };

describe("DEV-106 retention", () => {
    test("periods and reason are checked like the server", () => {
        const b = bounds as unknown as Parameters<typeof retentionProblem>[1];
        expect(retentionProblem(days, b, "校务会确认")).toBe("");
        expect(retentionProblem({ ...days, learningDays: 10 }, b, "校务会确认")).toContain("30 到 1095");
        expect(retentionProblem({ ...days, backupDays: 60 }, b, "校务会确认")).toContain("1 到 30");
        expect(retentionProblem(days, b, "短")).toContain("4 到 200");
        expect(retentionProblem(days, b, "<b>校务会</b>")).toContain("尖括号");
    });

    test("impact names what is removed and what is kept", () => {
        expect(retentionImpact({ enrollments: 0, learningRows: {}, rosterEntries: 0, artifacts: 0 })).toContain("没有需要清理");
        const t = retentionImpact({ enrollments: 1, learningRows: { edu_attempts: 2, edu_help_requests: 1 }, rosterEntries: 1, artifacts: 3 });
        expect(t).toContain("已离开班级 1 人次");
        expect(t).toContain("作答 2 条");
        expect(t).toContain("求助 1 条");
        expect(t).toContain("临时文件 3 个");
        expect(t).toContain("账本、用量与审计记录不在清理范围内");
    });

    test("overview and log read the confirmed state and counts", () => {
        expect(retentionLine({ state: "suggested_not_enforced", reference: "09 3.3" })).toContain("尚未执行自动清理");
        expect(retentionLine({ state: "confirmed", reference: "09 3.3", learningDays: 180, tempDays: 7 })).toContain("离开班级 180 天后清理");
        const row = { id: "1", action: "retention.run", actorId: "system:retention", subject: "tn_1", createdAt: "", detail: "enrollments=1 learning_rows=3 roster_entries=0 artifacts=2" };
        expect(accessRowText(row)).toBe("执行数据保留清理（system:retention，对象 tn_1）；离开班级 1 人次，学习记录 3 条，名单认领 0 条，临时文件 2 个");
        expect(accessRowText({ ...row, action: "retention.set", actorId: "u_a", detail: "temp=7 image=90 learning=180 backup=30 | 校务会确认" })).toContain("理由：校务会确认");
    });

    test("unconfirmed run has a Chinese explanation", () => {
        expect(teacherError({ status: 409, code: "RETENTION_NOT_CONFIRMED" })).toContain("建议值");
    });

    test("cleanup runs only after a dry run in the panel", () => {
        const panel = readFileSync(join(import.meta.dir, "../src/components/edu/org-admin.tsx"), "utf8");
        expect(panel).toContain("disabled={!retDry} onClick={() => void runRetention(true)}");
    });
});
