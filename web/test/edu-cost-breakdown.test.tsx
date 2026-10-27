import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { costRowLabel, notCollectedText, ratioText, reconciles, rowDetail, type CostBreakdown, type CostRow } from "../src/components/edu/cost-breakdown-logic";

const src = (p: string) => readFileSync(join(import.meta.dir, "..", p), "utf8");
const row = (o: Partial<CostRow>): CostRow => ({
    key: "k",
    costMicro: 0,
    machineMicro: 0,
    failedMicro: 0,
    supportMicro: 0,
    jobs: 0,
    retriedJobs: 0,
    accepted: 0,
    classDeliveries: 0,
    perAcceptedMicro: null,
    perClassDeliveryMicro: null,
    computable: false,
    ...o,
});

describe("DEV-118 cost breakdown", () => {
    test("zero denominators read as not computable, never zero", () => {
        expect(ratioText(null, 0)).toBe("不可计算");
        expect(ratioText(0, 0)).toBe("不可计算");
        expect(ratioText(1_500_000, 2)).toBe("1.5000");
    });
    test("labels strata and residual rows", () => {
        expect(costRowLabel("class:ecl_1")).toBe("班级 ecl_1");
        expect(costRowLabel("no_accepted_lesson")).toBe("未形成接受课件");
        expect(costRowLabel("reused")).toBe("多班复用");
        expect(costRowLabel("SOLID-ANGLE-02")).toBe("SOLID-ANGLE-02");
    });
    test("detail shows failed work, retries and support inside the batch", () => {
        const d = rowDetail(row({ machineMicro: 3_000_000, failedMicro: 1_000_000, retriedJobs: 2, supportMicro: 500_000 }));
        expect(d).toContain("失败或取消 1.0000");
        expect(d).toContain("重试 2 次");
        expect(d).toContain("人工支持 0.5000");
    });
    test("lists dimensions without a source and checks the rows add up", () => {
        const b: CostBreakdown = {
            period: "2026-10",
            definition: "cost_breakdown/1",
            by: "family",
            rows: [row({ costMicro: 3 }), row({ costMicro: 4 })],
            totalMicro: 7,
            supportUnattributedMicro: 0,
            excludedOpenJobs: 0,
            notCollected: [
                { dimension: "method", reason: "" },
                { dimension: "terminal", reason: "" },
            ],
        };
        expect(reconciles(b)).toBe(true);
        expect(reconciles({ ...b, totalMicro: 8 })).toBe(false);
        expect(notCollectedText(b)).toBe("未采集，无法分层：方法、终端");
    });
    test("the admin cost section mounts the breakdown", () => {
        expect(src("src/pages/edu/admin.tsx")).toContain("<CostBreakdownPanel scope={scope} period={period} />");
        expect(src("src/components/edu/cost-breakdown.tsx")).toContain("/cost-report/breakdown?period=");
    });
});
