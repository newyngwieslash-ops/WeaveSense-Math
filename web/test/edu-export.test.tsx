import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { EXPORT_LABEL, EXPORT_NOTE, safeFilename } from "../src/components/edu/export-logic";
import { teacherError } from "../src/components/edu/teacher-errors";

const src = (p: string) => readFileSync(join(import.meta.dir, "../src", p), "utf8");

describe("DEV-103 export", () => {
    test("download names cannot carry paths or markup", () => {
        expect(safeFilename("epub_1-student.html")).toBe("epub_1-student.html");
        expect(safeFilename("../../etc/passwd")).toBe("_.._etc_passwd.html");
        expect(safeFilename("<b>x</b>.html")).toBe("_b_x__b_.html");
        expect(safeFilename("")).toBe("export.html");
    });

    test("labels say what each file contains", () => {
        expect(EXPORT_LABEL.teacher).toContain("含答案");
        expect(EXPORT_NOTE.student).toContain("不含未开放的答案");
        expect(EXPORT_NOTE.report).toContain("不含学生姓名");
    });

    test("student version is the default request; teacher version is explicit", () => {
        const c = src("components/edu/export.tsx");
        expect(c).toContain("`/publications/${publicationId}/export`");
        expect(c).toContain("export?audience=teacher");
        expect(src("pages/edu/author.tsx")).toContain("<ContentExport");
        expect(src("pages/edu/teacher.tsx")).toContain("<ReportExport");
    });

    test("rights refusal mentions export and the next step", () => {
        const t = teacherError({ status: 409, code: "RIGHTS_RESTRICTED" });
        expect(t).toContain("导出");
        expect(t).toContain("来源管理");
    });
});
