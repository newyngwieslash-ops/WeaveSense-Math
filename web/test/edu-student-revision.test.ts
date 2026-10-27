import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "bun:test";
import { revisionBadge, revisionNotice } from "@/components/edu/student-revision-logic";

const when = () => "T";

describe("student revision notice (DEV-126)", () => {
    it("is silent without a revision", () => {
        expect(revisionNotice(null, when)).toBe("");
        expect(revisionBadge(undefined)).toBe("");
    });
    it("says a correction happened and keeps earlier answers visible to the teacher", () => {
        const t = revisionNotice({ revisedAt: "2026-10-28T00:00:00Z", correction: true, earlierAttempts: 2 }, when);
        expect(t).toContain("更正");
        expect(t).toContain("2 次作答已保留");
        expect(revisionBadge({ revisedAt: "", correction: true, earlierAttempts: 0 })).toBe("已更正");
    });
    it("plain update without earlier answers", () => {
        const t = revisionNotice({ revisedAt: "x", correction: false, earlierAttempts: 0 }, when);
        expect(t).toContain("更新");
        expect(t).not.toContain("作答已保留");
    });
});

// DEV-097: the student surface keeps only the step index and the student's own
// queued attempts in localStorage; lesson content, hints and answers never land
// in browser storage.
describe("student browser storage", () => {
    const root = join(__dirname, "..", "src");
    const read = (p: string) => readFileSync(join(root, p), "utf8");
    it("student page writes storage only through the offline queue", () => {
        const s = read("pages/edu/student.tsx");
        expect(s).not.toMatch(/localStorage\.setItem|sessionStorage|indexedDB|caches\.open/);
    });
    it("step viewer stores only a step index", () => {
        const s = read("components/edu/step-viewer.tsx");
        const writes = s.match(/localStorage\.setItem\([^)]*\)/g) || [];
        expect(writes).toEqual(["localStorage.setItem(storageKey, String(i)"]);
    });
});
