import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { SCOPE_LABEL, activeFirst, impactText, pauseInputError, type Pause } from "../src/components/edu/pause-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

const src = (p: string) => readFileSync(join(import.meta.dir, "..", p), "utf8");
const pause = (o: Partial<Pause>): Pause => ({ id: "eps_1", scope: "family", targetId: "f", reason: "bad", state: "active", createdBy: "u", createdAt: "2026-10-27T00:00:00Z", ...o });

describe("DEV-125 pauses", () => {
    test("every scope has a label", () => {
        expect(Object.keys(SCOPE_LABEL).sort()).toEqual(["family", "generation", "publication", "renderer"]);
    });
    test("input is validated before any request", () => {
        expect(pauseInputError("publication", "", "错误内容需要暂停")).toBe("请填写暂停对象");
        expect(pauseInputError("generation", "", "错误内容需要暂停")).toBe("");
        expect(pauseInputError("family", "a b", "错误内容需要暂停")).toContain("字符");
        expect(pauseInputError("renderer", "preview-svg/1.1.0", "短")).toContain("原因");
        expect(pauseInputError("renderer", "preview-svg/1.1.0", "渲染错位需要暂停")).toBe("");
    });
    test("impact says what is blocked and what is kept", () => {
        const t = impactText({ assignments: 2, publications: 1, students: 30, openJobs: 1 });
        expect(t).toContain("2 个任务");
        expect(t).toContain("30 名学生");
        expect(t).toContain("继续完成并结算");
        expect(t).toContain("不删除账本");
    });
    test("active pauses are listed first, newest first", () => {
        const s = activeFirst([pause({ id: "a", state: "lifted" }), pause({ id: "b", createdAt: "2026-10-26T00:00:00Z" }), pause({ id: "c" })]);
        expect(s.map((p) => p.id)).toEqual(["c", "b", "a"]);
    });
    test("pause errors have teacher text", () => {
        expect(teacherError({ status: 423, code: "GENERATION_PAUSED" })).toContain("暂停");
        expect(teacherError({ status: 423, code: "CONTENT_PAUSED" })).not.toContain("错误代码");
        expect(teacherError({ status: 503, code: "PAUSE_CHECK_UNAVAILABLE" })).toContain("没有执行");
    });
    test("the admin page mounts the panel with preview before confirm", () => {
        expect(src("src/pages/edu/admin.tsx")).toContain("<PausePanel scope={scope} />");
        const panel = src("src/components/edu/pauses.tsx");
        expect(panel).toContain('"/pauses"');
        expect(panel).toContain("/lift");
        expect(panel).toContain("body(false)");
    });
});

test("students see a plain pause message, never the raw server text", () => {
    const s = src("src/pages/edu/student.tsx");
    expect(s).toContain('CONTENT_PAUSED: "这份作业暂时停用');
    expect(s).toContain("PAUSE_CHECK_UNAVAILABLE:");
});
