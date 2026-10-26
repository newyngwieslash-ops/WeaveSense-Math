/** 教师首页（DEV-067）的纯逻辑：待办排序与状态文字。 */
export type HomeLesson = { id: string; title: string; familyId: string; state: string; mine: boolean; canReview: boolean; createdAt: string };
export type HomeClass = { id: string; name: string; students: number; activeAssignments: number; openHelp: number; urgentHelp: number };
export type HomeView = {
    myLessons: HomeLesson[];
    pendingReview: HomeLesson[];
    classes: HomeClass[];
    openHelp: number;
    urgentHelp: number;
    runningJobs: { id: string; kind: string; status: string; createdAt: string }[];
    openIncidents: number;
    reviewMode: string;
    sample: { sample: boolean; familyId: string; title: string; note: string };
    firstRun: boolean;
    demoClass?: { demo: true; label: string; id: string; name: string; students: number } | null;
};

export const LESSON_STATE_LABEL: Record<string, string> = { ready_for_review: "待审核", approved: "已通过", rejected: "已退回" };
export const JOB_STATUS_LABEL: Record<string, string> = { queued: "排队中", running: "运行中", reconciliation_required: "等待对账" };

/** 待办：内容事故与紧急求助优先，其次待我审核，再次普通求助。 */
export function todoLines(v: HomeView): string[] {
    const out: string[] = [];
    if (v.openIncidents > 0) out.push(`${v.openIncidents} 个内容事故未关闭`);
    if (v.urgentHelp > 0) out.push(`${v.urgentHelp} 条内容报错或无法访问的求助`);
    if (v.pendingReview.length > 0) out.push(`${v.pendingReview.length} 份讲解等待你审核`);
    const normal = v.openHelp - v.urgentHelp;
    if (normal > 0) out.push(`${normal} 条学生提问待回复`);
    return out;
}

export function authorLink(ws: string, opts: { lesson?: string; sample?: string }): string {
    const q = new URLSearchParams({ ws });
    if (opts.lesson) q.set("lesson", opts.lesson);
    if (opts.sample) q.set("sample", opts.sample);
    return `/edu/author?${q.toString()}`;
}

export type CatalogItem = { lessonRevisionId: string; problemId: string; title: string; familyId: string; state: string; publicationId?: string; publicationState?: string; mine: boolean; createdAt: string };

/** 检索参数（DEV-081）：空值不发送，检索词至多 60 字。 */
export function catalogQuery(q: string, family: string, state: string): string {
    const p = new URLSearchParams();
    const t = q.trim().slice(0, 60);
    if (t) p.set("q", t);
    if (family) p.set("family", family);
    if (state) p.set("state", state);
    const s = p.toString();
    return s ? `/catalog?${s}` : "/catalog";
}

export function catalogStateLabel(it: CatalogItem): string {
    if (it.publicationState === "published") return "已发布";
    if (it.publicationState === "withdrawn") return "已撤回";
    return LESSON_STATE_LABEL[it.state] || it.state;
}

/** 示例班级（DEV-067）：服务端每个响应都带 demo:true 与标注；前端再次核对，缺少标注时不展示。 */
export type DemoRecord = { id: string; studentName: string; kind: string; item: string; result?: string; hintLevel?: number; message?: string };
export type DemoView = {
    demo: true;
    label: string;
    class: { demo: true; id: string; name: string } | null;
    students?: string[];
    records?: DemoRecord[];
    dashboard?: { students: number; attempted: number; correct: number; hintUsers: number; hintsByLevel: Record<string, number>; results: Record<string, number>; commonErrors: string[]; openHelp: number };
};

export function isLabeledDemo(v: unknown): v is DemoView {
    const d = v as DemoView | null;
    return !!d && d.demo === true && typeof d.label === "string" && d.label.includes("演示数据") && (d.class === null || d.class?.demo === true);
}

export function demoSummaryLines(v: DemoView): string[] {
    const d = v.dashboard;
    if (!v.class || !d) return [];
    return [`${d.students} 名虚构学生，${d.attempted} 人已作答，${d.correct} 人正确`, `${d.hintUsers} 人用过提示`, `${d.openHelp} 条虚构提问待回复`];
}
