/**
 * 教学质量面板（DEV-075）的纯逻辑：数学、教学、呈现三个维度分开说明，不合成一个分数。
 * 数学来自数学服务的独立验证；教学只来自绑定当前讲解哈希的人工审核；呈现来自干净环境重新渲染检查。
 */
export type Quality = {
    math: { state: string; solutionStatus: string; evidenceLevels: string[]; domainProof: boolean; sampledOnly: boolean; source: string };
    teaching: {
        state: string;
        humanOnly: boolean;
        boundHash?: string;
        selfReview: boolean;
        checklist: Record<string, number>;
        findings: number;
        edited: boolean;
        editClasses?: string;
        staleReviews: number;
        reviewRecords: number;
    };
    presentation: { state: string; blocking: string[]; warnings: string[]; mathVerification: boolean };
    audio: { state: string; autoplay: boolean };
    combinedScore: boolean;
};

export const EVIDENCE_LABEL: Record<string, string> = {
    exact_instance: "本题数值精确核对",
    conditions_checked: "条件逐项核对",
    symbolic_domain_identity: "全域符号恒等证明",
    symbolic_domain: "全域符号推导",
    parametric_identity: "参数化恒等证明",
    numeric_sampling: "数值抽样（不等于全域证明）",
};

const MATH_STATE: Record<string, string> = { verified: "已自动验证", partially_verified: "部分验证", failed: "验证未通过", unknown: "没有验证结果", manual_unverified: "老师人工编写，未经自动数学验证" };
const TEACHING_STATE: Record<string, string> = {
    not_reviewed: "尚未人工审核",
    approved: "人工审核通过",
    rejected: "人工审核退回",
    inherited: "沿用上一修订的审核（仅限机构允许的读音类修改）",
};
const PRESENTATION_STATE: Record<string, string> = { passed: "通过", warnings: "通过，有提示", failed: "未通过，不可发布", not_run: "未运行" };

export type Dimension = { key: "math" | "teaching" | "presentation" | "audio"; title: string; status: "ok" | "warn" | "bad" | "none"; lines: string[] };

export function qualityDimensions(q: Quality): Dimension[] {
    const m = q.math;
    const mathLines = [`${MATH_STATE[m.state] || "状态未知"}；证据：${m.evidenceLevels.length ? m.evidenceLevels.map((l) => EVIDENCE_LABEL[l] || l).join("、") : "无"}`];
    if (m.state === "manual_unverified") mathLines.push("来源：老师人工编写；人工审核通过也不代表自动验证");
    else {
        if (m.sampledOnly) mathLines.push("只有数值抽样，没有全域证明，讲解中不得写成已证明");
        else if (!m.domainProof && m.state !== "unknown") mathLines.push("只核对了本题数值，没有全域证明");
        mathLines.push("来源：数学服务独立验证，不含人工审校");
    }

    const t = q.teaching;
    const tLines = [TEACHING_STATE[t.state] || "状态未知"];
    if (t.state === "approved" || t.state === "rejected") {
        tLines.push(`清单：通过 ${t.checklist.pass || 0} 项，有问题 ${t.checklist.issue || 0} 项，不适用 ${t.checklist.na || 0} 项；定位意见 ${t.findings} 条${t.selfReview ? "；作者自审" : ""}`);
    }
    if (t.staleReviews > 0) tLines.push(`另有 ${t.staleReviews} 条审核针对旧内容，不适用于当前修订`);
    if (t.edited) tLines.push(`本修订由编辑生成${t.editClasses ? `（${t.editClasses}）` : ""}，需按当前内容重新审核`);
    tLines.push("教学质量只能由老师人工判断，系统不自动打分");

    const p = q.presentation;
    const pLines = [`${PRESENTATION_STATE[p.state] || "状态未知"}${p.blocking.length ? `；阻断 ${p.blocking.length} 项` : ""}${p.warnings.length ? `；提示 ${p.warnings.length} 项` : ""}`, "只说明页面显示，不代表数学验证"];

    return [
        { key: "math", title: "数学", status: m.state === "verified" ? (m.sampledOnly ? "warn" : "ok") : m.state === "unknown" ? "none" : m.state === "failed" ? "bad" : "warn", lines: mathLines },
        { key: "teaching", title: "教学", status: t.state === "approved" || t.state === "inherited" ? "ok" : t.state === "rejected" ? "bad" : "none", lines: tLines },
        { key: "presentation", title: "呈现", status: p.state === "passed" ? "ok" : p.state === "warnings" ? "warn" : p.state === "failed" ? "bad" : "none", lines: pLines },
        { key: "audio", title: "音频", status: "none", lines: [q.audio.state === "not_generated" ? "尚未生成音频" : "音频已生成", "不会自动播放，需要老师点击播放"] },
    ];
}

export const STATUS_TEXT: Record<Dimension["status"], string> = { ok: "通过", warn: "需注意", bad: "未通过", none: "待处理" };
