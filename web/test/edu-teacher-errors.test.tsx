import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

import { emptyState, jobErrorText, LESSON_STATE_TEXT, lessonStateLine, STAGE_TEXT, teacherError, teacherErrorParts } from "../src/components/edu/teacher-errors";

const SERVER_TEXT = "pq: duplicate key value violates unique constraint at /srv/internal";

describe("DEV-083 teacher error mapping", () => {
    test("covers no permission, conflict, budget, out of scope, failure and withdrawn with a next action", () => {
        const cases: [Parameters<typeof teacherError>[0], string][] = [
            [{ status: 403, code: "HTTP_403" }, "没有这个操作的权限"],
            [{ status: 403, code: "FORBIDDEN" }, "没有这个操作的权限"],
            [{ status: 409, code: "REVISION_CONFLICT" }, "不是最新修订"],
            [{ status: 402, code: "BUDGET_EXCEEDED" }, "额度不足"],
            [{ status: 422, code: "UNSUPPORTED_FAMILY" }, "不在支持范围"],
            [{ status: 422, code: "PARAMETER_LIMIT" }, "超出支持范围"],
            [{ status: 503, code: "PROVIDER_UNAVAILABLE" }, "暂时不可用"],
            [{ status: 410, code: "WITHDRAWN" }, "已撤回"],
            [{ status: 0, code: "NETWORK", offline: true }, "网络不可用"],
        ];
        for (const [e, want] of cases) {
            const s = teacherError(e);
            expect(s).toContain(want);
            expect(s).toContain("下一步：");
            expect(s).not.toContain("错误代码");
        }
        expect(teacherError({ code: "TIMEOUT_UNDETERMINED" })).toContain("不代表无解");
        expect(teacherError({ code: "BUDGET_EXCEEDED" })).toContain("没有扣费");
    });

    test("unknown codes show only the code, never server text", () => {
        const e = { status: 500, code: "SOMETHING_NEW", message: SERVER_TEXT };
        const s = teacherError(e);
        expect(s).toContain("错误代码 SOMETHING_NEW");
        expect(s).not.toContain("pq:");
        expect(s).not.toContain("/srv");
        expect(teacherErrorParts({ status: 500 }).code).toBe("HTTP_500");
        expect(jobErrorText({ code: "MATH_TOOL_FAILED", message: SERVER_TEXT } as { code: string })).not.toContain("pq:");
        expect(jobErrorText(null)).toContain("下一步：");
    });

    test("empty states give a next step and stay quiet while loading", () => {
        expect(emptyState({ classes: 0, classId: "", assignments: -1, asId: "", attempts: -1 })).toContain("新建班级");
        expect(emptyState({ classes: -1, classId: "", assignments: -1, asId: "", attempts: -1 })).toBe("");
        expect(emptyState({ classes: 2, classId: "c", assignments: 0, asId: "", attempts: -1 })).toContain("布置到这个班级");
        expect(emptyState({ classes: 2, classId: "c", assignments: 1, asId: "a", attempts: 0 })).toContain("还没有学生作答");
        expect(emptyState({ classes: 2, classId: "c", assignments: 1, asId: "a", attempts: 3 })).toBe("");
    });

    test("lesson state line names withdrawn and blocking codes in Chinese", () => {
        expect(LESSON_STATE_TEXT.withdrawn).toContain("学生不可见");
        const line = lessonStateLine("rejected", JSON.stringify(["render_check:figure_in_view"]), (c) => `标签${c}`);
        expect(line).toContain("不可发布");
        expect(line).toContain("标签render_check:figure_in_view");
        expect(lessonStateLine("approved", "", (c) => c)).toBe("已通过");
        expect(lessonStateLine("weird", "", (c) => c)).toBe("状态未知");
        expect(STAGE_TEXT.check_render).toBe("呈现检查");
    });
});

describe("DEV-083 static guard", () => {
    const root = join(import.meta.dir, "..", "src");
    const teacherFiles = [
        ...["teacher.tsx", "author.tsx", "home.tsx", "admin.tsx"].map((f) => join(root, "pages", "edu", f)),
        ...readdirSync(join(root, "components", "edu"))
            .filter((f) => f.endsWith(".tsx"))
            .map((f) => join(root, "components", "edu", f)),
    ];

    test("teacher pages never render server error message text", () => {
        for (const f of teacherFiles) {
            const src = readFileSync(f, "utf8");
            expect({ f, hit: /\br\.message\b|error\??\.message|\.error\.message/.test(src) }).toEqual({ f, hit: false });
        }
    });

    test("teacher notices are announced to assistive technology", () => {
        for (const f of teacherFiles) {
            const src = readFileSync(f, "utf8");
            for (const m of src.matchAll(/\{(msg|err) \? \(?\s*<p([^>]*)>/g)) {
                expect({ f, ok: /role="(status|alert)"/.test(m[2]) }).toEqual({ f, ok: true });
            }
        }
    });
});
