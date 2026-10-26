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
