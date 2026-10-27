import { describe, expect, test } from "bun:test";

import { actionLabel, blockerText, durationText, queueText, stageEstimateText, teachingBody, type LessonPlan } from "../src/components/edu/lesson-plan-logic";

const plan: LessonPlan = {
    admitted: false,
    blockers: [{ code: "GENERATION_PAUSED", message: "new generation is paused", action: "wait_or_contact_admin" }],
    teaching: { goals: ["understand_method", "practice", "fix_mistake"], styles: ["concise", "standard", "detailed"], minMinutes: 3, maxMinutes: 8, defaultMinutes: 5 },
    estimate: { stages: [{ name: "compose_lesson", maxComputeMs: 10000, maxCostMicro: 0 }], maxComputeMs: 75000, maxCostMicro: 0, unitMicro: 0, priceVersion: "v1" },
    queue: { queued: 0, running: 0 },
};

describe("lesson plan (DEV-070)", () => {
    test("teaching body omits an unset goal", () => {
        expect(teachingBody({ goal: "", style: "standard", targetMinutes: 5 })).toEqual({ style: "standard", targetMinutes: 5 });
        expect(teachingBody({ goal: "practice", style: "concise", targetMinutes: 3 })).toEqual({ goal: "practice", style: "concise", targetMinutes: 3 });
    });
    test("blockers carry a next step", () => {
        expect(blockerText(plan.blockers[0])).toContain("等管理员解除暂停后再生成");
        expect(blockerText(plan.blockers[0])).not.toContain("paused");
        expect(actionLabel("reconcile")).toContain("不会自动重复计费");
        expect(actionLabel("unknown_x")).toBe("unknown_x");
    });
    test("estimate and queue text", () => {
        expect(stageEstimateText(plan)).toContain("最多 75 秒计算");
        expect(stageEstimateText(plan)).toContain("未对计算计费");
        expect(stageEstimateText({ ...plan, estimate: { ...plan.estimate, unitMicro: 2, maxCostMicro: 150000 } })).toContain("费用上限 150000");
        expect(queueText(plan)).toBe("当前无排队任务");
        expect(queueText({ ...plan, queue: { queued: 2, running: 1 } })).toContain("排队 2 个");
    });
    test("duration is labelled as an estimate and compared with guidance and target", () => {
        expect(durationText(null)).toBe("");
        const t = durationText({ estimatedSeconds: 125, basis: "narration_text_estimate", fit: "shorter_than_guidance", targetMinutes: 5, targetFit: "shorter" });
        expect(t).toContain("约 2 分 5 秒");
        expect(t).toContain("仅为估算");
        expect(t).toContain("短于建议");
        expect(t).toContain("比目标 5 分钟短");
        expect(durationText({ estimatedSeconds: 300, basis: "x", fit: "within_guidance" })).not.toContain("目标");
    });
});
