import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { alertValueText, availabilityText, durationText, notCollectedText, rateText, sortedAlerts, validJobId, type ObsAlert, type Observability } from "../src/components/edu/observability-logic";

const src = (p: string) => readFileSync(join(import.meta.dir, "..", p), "utf8");
const alert = (o: Partial<ObsAlert>): ObsAlert => ({ id: "stuck_jobs", state: "ok", value: 0, threshold: 0, rule: "", ...o });
const obs = (o: Partial<Observability>): Observability =>
    ({
        definition: "observability/1",
        windowHours: 24,
        generation: { byKind: [], errorCodes: [], finished: 0, failed: 0 },
        submission: { attempts: 0, selfTests: 0, helpRequests: 0 },
        reading: { collected: false },
        queue: { queued: 0, running: 0, oldestQueuedSeconds: null, stuck: [], stuckTotal: 0 },
        cost: { reconciliationRequired: 0, heldUnknown: 0, unsettledReservations: 0, openLedgerAnomalies: 0 },
        availability: { target: 0.995, measured: null, reason: "external probe required" },
        alerts: [],
        firing: 0,
        notCollected: [],
        ...o,
    }) as Observability;

describe("DEV-121 observability", () => {
    test("firing alerts come first, then insufficient data, then ok", () => {
        const s = sortedAlerts([alert({ id: "b", state: "ok" }), alert({ id: "a", state: "insufficient_data" }), alert({ id: "c", state: "firing" })]);
        expect(s.map((a) => a.id)).toEqual(["c", "a", "b"]);
    });
    test("values read as rate, duration or count", () => {
        expect(alertValueText(alert({ id: "failure_rate", value: 0.25 }))).toBe("25.0%");
        expect(alertValueText(alert({ id: "queue_age", value: 1200 }))).toBe("20 分钟");
        expect(alertValueText(alert({ id: "stuck_jobs", value: 2 }))).toBe("2");
        expect(durationText(null)).toBe("无");
        expect(durationText(7200)).toBe("2.0 小时");
        expect(rateText(null)).toBe("不可计算");
    });
    test("availability is a target, never a made up measurement", () => {
        expect(availabilityText(obs({}))).toBe("目标 99.5%，未测量（需外部探针）");
        expect(
            notCollectedText(
                obs({
                    notCollected: [
                        { dimension: "availability", reason: "" },
                        { dimension: "reading", reason: "" },
                    ],
                }),
            ),
        ).toContain("可用性");
    });
    test("job ids are validated before a trace request", () => {
        expect(validJobId("ejb_abc-1")).toBe(true);
        expect(validJobId("../x")).toBe(false);
        expect(validJobId("")).toBe(false);
    });
    test("the admin page mounts the panel and the panel only reads admin routes", () => {
        expect(src("src/pages/edu/admin.tsx")).toContain("<ObservabilityPanel scope={scope} />");
        const panel = src("src/components/edu/observability.tsx");
        expect(panel).toContain("/observability?hours=");
        expect(panel).toContain("/trace");
        expect(panel).not.toContain('"post"');
    });
});
