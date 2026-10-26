/** 审核清单与定位退回（DEV-076）。与服务端 reviewItems、findingTarget 保持一致。 */
export const REVIEW_ITEMS = ["problem", "conditions", "method", "visuals", "hints", "self_test", "answer_policy"] as const;
export type ReviewItem = (typeof REVIEW_ITEMS)[number];
export type Verdict = "pass" | "issue" | "na";
export type Checklist = Partial<Record<ReviewItem, Verdict>>;
export type Finding = { target: string; item: ReviewItem; note: string };

export const ITEM_LABEL: Record<ReviewItem, string> = {
    problem: "题意",
    conditions: "条件",
    method: "方法",
    visuals: "图、文、音一致",
    hints: "提示",
    self_test: "自测",
    answer_policy: "答案策略",
};
export const VERDICT_LABEL: Record<Verdict, string> = { pass: "通过", issue: "有问题", na: "不适用" };
export const WHOLE_TARGETS: Record<string, string> = {
    lesson: "整份讲解",
    problem: "题面",
    conditions: "条件",
    method: "方法",
    answer: "答案",
    practice: "练习",
    layout: "版式",
};

export function targetLabel(target: string, beats: { id: string; title: string }[] = []): string {
    if (target.startsWith("beat:")) {
        const id = target.slice(5);
        const i = beats.findIndex((b) => b.id === id);
        return i >= 0 ? `第 ${i + 1} 步 ${beats[i].title}` : `步骤 ${id}`;
    }
    if (target.startsWith("hint:")) return `提示 ${target.slice(5)}`;
    return WHOLE_TARGETS[target] || target;
}

export function checklistComplete(c: Checklist): boolean {
    return REVIEW_ITEMS.every((k) => !!c[k]);
}

export function openIssues(c: Checklist): ReviewItem[] {
    return REVIEW_ITEMS.filter((k) => c[k] === "issue");
}

/** 通过：清单完整、无“有问题”、无定位意见、无阻断错误。 */
export function canApprove(c: Checklist, findings: Finding[], blockingErrors: string): boolean {
    return !blockingErrors && checklistComplete(c) && openIssues(c).length === 0 && findings.length === 0;
}

/** 退回：至少一条定位意见、一个“有问题”项或一段意见。 */
export function canReject(c: Checklist, findings: Finding[], comment: string): boolean {
    return findings.length > 0 || openIssues(c).length > 0 || comment.trim() !== "";
}

export function parseFindings(raw?: string): Finding[] {
    if (!raw) return [];
    try {
        const v = JSON.parse(raw) as Finding[];
        return Array.isArray(v) ? v : [];
    } catch {
        return [];
    }
}
