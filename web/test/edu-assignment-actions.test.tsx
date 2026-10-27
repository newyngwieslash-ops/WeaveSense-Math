import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { actionReasonProblem, migratePreviewText, publicationImpactText, regradeChangeText, regradePreviewText } from "../src/components/edu/assignment-actions-logic";

const src = (p: string) => readFileSync(join(import.meta.dir, "..", p), "utf8");

describe("DEV-112 teacher impact confirmation", () => {
    test("reasons follow the server limits", () => {
        expect(actionReasonProblem("  ", 400)).toBe("请填写原因");
        expect(actionReasonProblem("改", 200)).toBe("");
        expect(actionReasonProblem("字".repeat(201), 200)).toContain("200");
        expect(actionReasonProblem("字".repeat(400), 400)).toBe("");
    });

    test("migration preview names kept attempts and dropped variant sets", () => {
        const t = migratePreviewText({ fromAssignmentId: "a", toPublicationId: "p2", affectedStudents: 3, attemptsKept: 7, incidentId: "inc_1", variantSetDropped: true, confirmed: false });
        expect(t).toContain("3 名学生的 7 次作答留在原任务");
        expect(t).toContain("变式题组不会带到新任务");
        expect(t).toContain("inc_1");
        expect(t).toContain("只能迁移一次");
    });

    test("regrade preview tolerates empty lists and keeps teacher judgements", () => {
        expect(regradePreviewText({ refPublicationId: "p", examined: 2, changed: [], unchanged: 2, skipped: null, affectedStudents: 0, confirmed: false })).toBe("检查 2 次作答；0 次结果会改变（涉及 0 名学生）；2 次不变。");
        const ch = { attemptId: "at", studentUserId: "u_bob", oldResult: "correct", newResult: "incorrect", teacherJudged: true };
        const t = regradePreviewText({ refPublicationId: "p", examined: 3, changed: [ch], unchanged: 1, skipped: ["x"], affectedStudents: 1, confirmed: false });
        expect(t).toContain("1 次结果会改变（涉及 1 名学生）");
        expect(t).toContain("跳过且不改为错误");
        expect(t).toContain("人工判定保持不变");
        expect(regradeChangeText(ch)).toBe("u_bob：正确 → 错误（老师已人工判定，保持不变）");
    });

    test("publication withdrawal lists every affected object", () => {
        const t = publicationImpactText({ assignments: [{ id: "a1" }, { id: "a2" }], classes: ["c1"], students: 25, attempts: 40, variantSets: ["v"], revokedArtifacts: ["x", "y"], childRevisions: ["r"] });
        expect(t).toContain("涉及 2 个任务（1 个班级、25 名在读学生）");
        expect(t).toContain("撤回 1 个变式题组");
        expect(t).toContain("2 个产物链接立即失效");
        expect(t).toContain("1 个由此编辑出的修订");
        expect(publicationImpactText({ assignments: null, classes: null, students: 0, attempts: 0, variantSets: null, revokedArtifacts: null, childRevisions: null })).toContain("涉及 0 个任务");
    });

    test("each panel previews without confirm, then confirms with the same body", () => {
        const p = src("src/components/edu/assignment-actions.tsx");
        expect(p).toContain("`/assignments/${assignmentId}/migrate`, scope, body(false)");
        expect(p).toContain("`/assignments/${assignmentId}/migrate`, scope, body(true)");
        expect(p).toContain("`/assignments/${assignmentId}/regrade`, scope, body(false)");
        expect(p).toContain("`/assignments/${assignmentId}/regrade`, scope, body(true)");
        expect(p).toContain('{ severity: "blocking", reason: reason.trim() }');
        expect(p).toContain('{ severity: "blocking", reason: reason.trim(), confirm: true }');
        expect(p).toContain("publications.filter((p) => p.id !== publicationId)");
        const page = src("src/pages/edu/teacher.tsx");
        for (const c of ["<MigrateAssignmentPanel", "<RegradePanel", "<WithdrawPublicationPanel"]) expect(page).toContain(c);
        expect(src("src/router.tsx")).toContain('path: "/dev/edu-teach"');
        expect(src("scripts/edu-chrome-e2e.mjs")).toContain("runTeacher");
    });
});
