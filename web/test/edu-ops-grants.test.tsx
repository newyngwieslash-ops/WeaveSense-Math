import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { opsGrantProblem, opsGrantText, opsRevokeProblem, type OpsGrantForm } from "../src/components/edu/ops-grant-logic";
import { accessRowText } from "../src/components/edu/org-admin-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

const ok: OpsGrantForm = { operatorUserId: "u_ops", scope: "jobs", classId: "", includeContent: false, purpose: "排查作业失败 工单 OPS-7", hours: 4 };

describe("DEV-108 operator access", () => {
    test("grant form is checked like the server", () => {
        expect(opsGrantProblem(ok, "u_admin")).toBe("");
        expect(opsGrantProblem({ ...ok, operatorUserId: " " }, "")).toContain("用户编号");
        expect(opsGrantProblem({ ...ok, operatorUserId: "u_admin" }, "u_admin")).toContain("自己");
        expect(opsGrantProblem({ ...ok, hours: 73 }, "")).toContain("1 到 72");
        expect(opsGrantProblem({ ...ok, hours: 0 }, "")).toContain("1 到 72");
        expect(opsGrantProblem({ ...ok, includeContent: true }, "")).toContain("不能包含作答内容");
        expect(opsGrantProblem({ ...ok, scope: "class" }, "")).toContain("选择班级");
        expect(opsGrantProblem({ ...ok, scope: "class", classId: "ecl_1", includeContent: true }, "")).toBe("");
        expect(opsGrantProblem({ ...ok, purpose: "短" }, "")).toContain("4 到 200");
        expect(opsGrantProblem({ ...ok, purpose: "<b>排查</b>" }, "")).toContain("尖括号");
        expect(opsRevokeProblem("排查已完成")).toBe("");
        expect(opsRevokeProblem("完成")).toContain("4 到 200");
    });

    test("grant text says exactly what the operator can see", () => {
        const g = { id: "eog_1", operatorUserId: "u_ops", scope: "class", classId: "ecl_1", includeContent: false, purpose: "排查判分异常", expiresAt: "2026-10-27T12:00:00Z", status: "active", grantedBy: "u_admin", createdAt: "" };
        expect(opsGrantText(g, "高一(1)班")).toContain("高一(1)班 的作答记录（不含作答内容）");
        expect(opsGrantText({ ...g, includeContent: true }, "高一(1)班")).toContain("（含作答内容）");
        expect(opsGrantText({ ...g, scope: "jobs" })).toContain("生成任务的状态与错误码");
        expect(opsGrantText(g)).toContain("目的：排查判分异常");
    });

    test("errors and access log lines are readable", () => {
        expect(teacherError({ code: "OPS_GRANT_INVALID", status: 403 })).toContain("没有有效的运维授权");
        expect(accessRowText({ id: "1", action: "ops.read", actorId: "u_ops", subject: "eog_1", createdAt: "", detail: "class=ecl_1 | 排查判分异常" })).toBe("运维查看（u_ops，对象 eog_1）；class=ecl_1 | 排查判分异常");
        expect(accessRowText({ id: "2", action: "ops.revoke", actorId: "u_admin", subject: "eog_1", createdAt: "", detail: "排查已完成" })).toContain("撤销运维授权");
    });

    test("panel is mounted for org admins and never lets job scope carry content", () => {
        const panel = readFileSync(join(import.meta.dir, "../src/components/edu/ops-grants.tsx"), "utf8");
        expect(panel).toContain('includeContent: form.scope === "class" && form.includeContent');
        expect(panel).toContain("/ops-grants/${id}/revoke");
        expect(readFileSync(join(import.meta.dir, "../src/components/edu/org-admin.tsx"), "utf8")).toContain("<OpsGrantsPanel");
    });
});
