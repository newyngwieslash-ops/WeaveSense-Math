import { describe, expect, test } from "bun:test";

import { DEFAULT_CLAIM_TTL_HOURS, formatClaimCodes, joinCodeKind, joinErrorText, joinSuccessText, MAX_ROSTER_CSV_BYTES, rosterCodesNotice, rosterConfirmProblem, rosterCsvTooLarge, rosterPreviewSummary } from "../src/components/edu/roster-import-logic";

describe("DEV-016 roster import UI (E93)", () => {
    test("preview summary and confirm gates", () => {
        expect(rosterPreviewSummary({ new: 2, existing: 1, invalid: 1 })).toContain("无效行须改正后才能确认");
        expect(rosterPreviewSummary({ new: 0, existing: 3, invalid: 0 })).toContain("没有可导入的新行");
        expect(rosterPreviewSummary({ new: 2, existing: 0, invalid: 0 })).toBe("新 2 行，已存在 0 行，无效 0 行。");
        expect(rosterConfirmProblem(null, "")).toContain("请粘贴或选择");
        expect(rosterConfirmProblem(null, "externalRef\nS1")).toContain("请先预览");
        expect(rosterConfirmProblem({ new: 2, invalid: 1 }, "externalRef\nS1")).toContain("改正无效行");
        expect(rosterConfirmProblem({ new: 0, invalid: 0 }, "externalRef\nS1")).toContain("没有可导入的新行");
        expect(rosterConfirmProblem({ new: 2, invalid: 0 }, "externalRef\nS1")).toBe("");
        expect(rosterCsvTooLarge("a".repeat(MAX_ROSTER_CSV_BYTES))).toBe(false);
        expect(rosterCsvTooLarge("a".repeat(MAX_ROSTER_CSV_BYTES + 1))).toBe(true);
        expect(rosterConfirmProblem({ new: 1, invalid: 0 }, "a".repeat(MAX_ROSTER_CSV_BYTES + 1))).toContain("64 KiB");
        expect(DEFAULT_CLAIM_TTL_HOURS).toBe(336);
        expect(rosterCodesNotice(3)).toContain("只显示这一次");
        expect(formatClaimCodes([{ entryId: "erc_1", externalRef: "S001", code: "erc_1.abc", expiresAt: "" }])).toBe("S001\terc_1.abc");
    });

    test("invite and claim codes are distinguished, unknown rejected", () => {
        const secret = "a".repeat(48);
        expect(joinCodeKind(`eci_demo.${secret}`)).toBe("invite");
        expect(joinCodeKind(` erc_demo.${secret} `)).toBe("claim");
        expect(joinCodeKind("eci_demo.short")).toBe("unknown");
        expect(joinCodeKind("not-a-code")).toBe("unknown");
        expect(joinCodeKind("")).toBe("unknown");
    });

    test("student join messages never include server text", () => {
        expect(joinSuccessText("invite", false)).toBe("已加入班级");
        expect(joinSuccessText("claim", true)).toContain("必要同意");
        expect(joinErrorText({ status: 404 })).toContain("无效或已过期");
        expect(joinErrorText({ code: "POLICY_REVOKED", status: 403 })).toContain("请联系老师");
        expect(joinErrorText({ offline: true })).toContain("网络不可用");
        expect(joinErrorText({ code: "SOMETHING_NEW", status: 500 })).toContain("SOMETHING_NEW");
        expect(joinErrorText({ code: "SOMETHING_NEW", status: 500 })).not.toContain("stack");
    });
});
