import { describe, expect, test } from "bun:test";

import { selfTestPreview } from "../src/components/edu/edu-admin-logic";

describe("self-test preview (DEV-075)", () => {
    test("keeps only what a student sees", () => {
        const p = selfTestPreview({
            items: [{ id: "v1", family_id: "F", params: { a: "3" }, unit: "cm", prompt: "求 x", answer_format: "number", math_state: "verified", evidence_levels: ["symbolic"] }],
        });
        expect(p.questions).toEqual([{ id: "v1", prompt: "求 x", unit: "cm", answer_format: "number" }]);
        expect(p.state).toBe("open");
        expect(JSON.stringify(p)).not.toContain("verified");
        expect("answerKeys" in p).toBe(false);
    });
});
