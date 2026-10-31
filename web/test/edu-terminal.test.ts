import { describe, expect, it } from "bun:test";
import { eduTerminal } from "../src/services/api/edu";

describe("eduTerminal (DEV-118)", () => {
    it("classifies by viewport width only", () => {
        expect(eduTerminal(320)).toBe("phone");
        expect(eduTerminal(599)).toBe("phone");
        expect(eduTerminal(600)).toBe("tablet");
        expect(eduTerminal(1023)).toBe("tablet");
        expect(eduTerminal(1024)).toBe("desktop");
    });
    it("sends nothing when the width is unknown", () => {
        expect(eduTerminal(undefined)).toBeUndefined();
        expect(eduTerminal(0)).toBeUndefined();
        expect(eduTerminal(Number.NaN)).toBeUndefined();
    });
});
