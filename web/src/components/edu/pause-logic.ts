/** DEV-125 暂停开关的纯逻辑：标签、校验与影响说明。 */
export type PauseScope = "generation" | "family" | "publication" | "renderer";
export type Pause = {
    id: string;
    scope: PauseScope;
    targetId: string;
    reason: string;
    state: "active" | "lifted";
    createdBy: string;
    createdAt: string;
    liftedBy?: string;
    liftedAt?: string;
    liftNote?: string;
};
export type PauseList = { pauses: Pause[]; platformGenerationPaused: boolean };
export type PauseImpact = { assignments: number; publications: number; students: number; openJobs: number };

export const SCOPE_LABEL: Record<PauseScope, string> = {
    generation: "本机构全部新生成",
    family: "单个题族",
    publication: "单个发布版",
    renderer: "单个渲染服务版本",
};

export const TARGET_HINT: Record<PauseScope, string> = {
    generation: "无需填写",
    family: "题族编号，例如 SOLID-ANGLE-02",
    publication: "发布版编号",
    renderer: "渲染服务版本，例如 preview-svg/1.1.0",
};

/** 返回校验错误；空字符串表示可以预览。 */
export function pauseInputError(scope: PauseScope, target: string, reason: string): string {
    const t = target.trim();
    if (scope !== "generation" && t === "") return "请填写暂停对象";
    if (t.length > 60) return "暂停对象过长";
    if (!/^[\w.@:/-]*$/.test(t)) return "暂停对象含有不支持的字符";
    const r = reason.trim();
    if (r.length < 4 || r.length > 400) return "请写明 4 到 400 字的原因";
    return "";
}

export function impactText(i: PauseImpact): string {
    const parts = [`${i.assignments} 个任务`, `${i.publications} 个发布版`, `${i.students} 名学生`];
    if (i.openJobs > 0) parts.push(`${i.openJobs} 个进行中的生成会继续完成并结算`);
    return `将影响 ${parts.join("，")}。暂停只阻止新的使用，不删除账本、作答、删除标记或审计。`;
}

export function activeFirst(ps: Pause[]): Pause[] {
    return [...ps].sort((a, b) => (a.state === b.state ? b.createdAt.localeCompare(a.createdAt) : a.state === "active" ? -1 : 1));
}
