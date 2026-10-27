// Student side of a revision notice (DEV-126). The server only sends when the
// assignment replaced an earlier version, whether it was a content correction,
// and how many of the student's own answers stay with that earlier version; it
// never sends the teacher's reason or any incident detail.
export interface StudentRevision {
    revisedAt: string;
    correction: boolean;
    earlierAttempts: number;
}

export function revisionBadge(r?: StudentRevision | null): string {
    if (!r) return "";
    return r.correction ? "已更正" : "已更新";
}

export function revisionNotice(r?: StudentRevision | null, when: (iso: string) => string = (iso) => new Date(iso).toLocaleString()): string {
    if (!r) return "";
    const head = r.correction ? `老师已于 ${when(r.revisedAt)} 更正了这份作业的内容，请以当前版本为准。` : `老师已于 ${when(r.revisedAt)} 更新了这份作业，请以当前版本为准。`;
    const tail = r.earlierAttempts > 0 ? `你在旧版本中的 ${r.earlierAttempts} 次作答已保留，老师可以查看，不计入当前版本。` : "";
    return head + tail;
}
