import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";

import { provenanceText } from "../src/components/edu/provenance-logic";

// DEV-101：学生页与教师编辑器显示服务端给出的来源标识。
describe("content provenance", () => {
    test("shows the server label as is", () => {
        expect(provenanceText({ kind: "template_only", ai_generated: false, generator: "deterministic-template", ai_parts: [], teacher_edited: true, label: "讲解由程序按题族模板生成，平台未调用生成式人工智能撰写内容；含老师修改" })).toBe(
            "内容来源：讲解由程序按题族模板生成，平台未调用生成式人工智能撰写内容；含老师修改",
        );
    });
    test("missing provenance is reported, not guessed", () => {
        expect(provenanceText(null)).toBe("内容来源：生成方式未记录");
        expect(provenanceText(undefined)).toBe("内容来源：生成方式未记录");
    });
    test("student page and editor render it", () => {
        const student = readFileSync(new URL("../src/pages/edu/student.tsx", import.meta.url), "utf8");
        expect(student).toContain('data-testid="content-provenance"');
        expect(student).toContain("provenanceText(provenance)");
        const editor = readFileSync(new URL("../src/components/edu/lesson-editor.tsx", import.meta.url), "utf8");
        expect(editor).toContain('data-testid="editor-provenance"');
    });
});
