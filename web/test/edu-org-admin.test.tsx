import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { accessRowText, consentLine, purposeProblem, PURPOSE_MAX, regionLine, retentionLine, staffLine } from "../src/components/edu/org-admin-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

describe("DEV-102 institution admin", () => {
    test("purpose is checked like the server before any individual request", () => {
        expect(purposeProblem("ab")).toContain("至少 4");
        expect(purposeProblem("x".repeat(PURPOSE_MAX + 1))).toContain(`${PURPOSE_MAX}`);
        expect(purposeProblem("<b>核查</b>")).toContain("尖括号");
        expect(purposeProblem("家长投诉核查")).toBe("");
    });

    test("overview lines are aggregate and say what is not in force", () => {
        expect(staffLine({ teacher: 2, reviewer: 0, org_admin: 1 })).toBe("教师 2 人，审核员 0 人，教育管理员 1 人");
        expect(consentLine({ enrolled: 0, guardianConsent: 0, consentMissing: 0 })).toContain("还没有学生");
        expect(consentLine({ enrolled: 3, guardianConsent: 2, consentMissing: 1 })).toContain("缺少同意 1 人");
        expect(regionLine({ state: "not_configured", blockedBy: "DEV-105" })).toContain("尚未配置");
        expect(retentionLine({ state: "suggested_not_enforced", reference: "09 3.3" })).toContain("尚未执行自动清理");
    });

    test("access log rows show the stated purpose, other rows no detail", () => {
        const row = { id: "1", action: "org.individual_access", actorId: "u_dave", subject: "ecl_1", createdAt: "", detail: "GET /api/edu/classes/:id/roster | 家长投诉核查" };
        expect(accessRowText(row)).toBe("查看个体学生记录（u_dave，对象 ecl_1）；目的：家长投诉核查");
        expect(accessRowText({ ...row, action: "class.create", detail: undefined })).toBe("创建班级（u_dave，对象 ecl_1）");
    });

    test("purpose refusal has a Chinese explanation and next step", () => {
        const t = teacherError({ status: 403, code: "ACCESS_PURPOSE_REQUIRED" });
        expect(t).toContain("业务目的");
        expect(t).toContain("访问记录");
    });

    test("the purpose travels percent-encoded and only for non-teaching classes", () => {
        const api = readFileSync(join(import.meta.dir, "../src/services/api/edu.ts"), "utf8");
        expect(api).toContain('headers["X-Edu-Access-Purpose"] = encodeURIComponent(scope.purpose)');
        const panel = readFileSync(join(import.meta.dir, "../src/components/edu/org-admin.tsx"), "utf8");
        expect(panel).toContain("cls?.youTeach ? scope : { ...scope, purpose: look.purpose.trim() }");
        expect(readFileSync(join(import.meta.dir, "../src/pages/edu/admin.tsx"), "utf8")).toContain("<OrgAdminPanel");
    });
});
