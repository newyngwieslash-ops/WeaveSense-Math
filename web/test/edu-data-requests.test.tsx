import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { impactText, jsonFilename, parseOutcome, textProblem } from "../src/components/edu/data-request-logic";
import { accessRowText } from "../src/components/edu/org-admin-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

describe("DEV-107 data requests", () => {
    test("reason and verification reference are checked like the server", () => {
        expect(textProblem("监护人书面申请", 200, "申请说明")).toBe("");
        expect(textProblem("短", 200, "申请说明")).toContain("4 到 200");
        expect(textProblem("x".repeat(121), 120, "核验记录编号")).toContain("4 到 120");
        expect(textProblem("<b>核验</b>", 120, "核验记录编号")).toContain("尖括号");
    });

    test("delete impact names every category", () => {
        const t = impactText({
            kind: "delete",
            stopNewProcessing: { studentRoleRevoked: 1, consentWithdrawn: 1, enrollmentsWithdrawn: 2 },
            onlineDeleted: { learningRows: { edu_attempts: 3, edu_help_requests: 1 }, rosterEntries: 1 },
            backupExpiry: "2026-11-26",
        });
        expect(t).toContain("停止新的处理");
        expect(t).toContain("移出班级 2 个");
        expect(t).toContain("在线删除学习记录 4 条");
        expect(t).toContain("无单独缓存");
        expect(t).toContain("2026-11-26 前随备份到期");
        expect(t).toContain("账本、用量与审计依法单独保留");
    });

    test("access and withdraw say what is and is not included", () => {
        expect(impactText({ kind: "access", records: { edu_attempts: 2 } })).toContain("教师批注与答案不在副本内");
        expect(impactText({ kind: "withdraw", stopNewProcessing: { studentRoleRevoked: 1, consentWithdrawn: 0, enrollmentsWithdrawn: 1 }, recordsKept: "x" })).toContain("保留到保留期或删除请求");
        expect(parseOutcome({ id: "1", studentUserId: "u", kind: "access", requester: "student", reason: "r", status: "rejected", createdAt: "", outcome: '{"rejected":"无法核验"}' })).toEqual({ rejected: "无法核验" });
        expect(parseOutcome({ id: "1", studentUserId: "u", kind: "access", requester: "student", reason: "r", status: "received", createdAt: "", outcome: "{" })).toBeNull();
    });

    test("package file name is safe", () => {
        expect(jsonFilename("data-request-edr_1")).toBe("data-request-edr_1.json");
        expect(jsonFilename("../../etc/x.json")).toBe("_.._etc_x.json");
    });

    test("errors and log rows are explained", () => {
        expect(teacherError({ status: 409, code: "DATA_REQUEST_NOT_VERIFIED" })).toContain("核验");
        expect(teacherError({ status: 409, code: "DATA_REQUEST_OPEN" })).toContain("同类请求");
        expect(accessRowText({ id: "1", action: "datareq.complete", actorId: "u_a", subject: "edr_1", createdAt: "", detail: "kind=delete" })).toBe("完成数据请求（u_a，对象 edr_1）；kind=delete");
    });

    test("completion and replay need a preview first; panel mounted", () => {
        const panel = readFileSync(join(import.meta.dir, "../src/components/edu/data-requests.tsx"), "utf8");
        expect(panel).toContain("disabled={!pv}");
        expect(panel).toContain("disabled={!replay}");
        expect(readFileSync(join(import.meta.dir, "../src/components/edu/org-admin.tsx"), "utf8")).toContain("<DataRequestsPanel");
    });
});
