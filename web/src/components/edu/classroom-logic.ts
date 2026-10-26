/**
 * 课堂演示模式（DEV-077）的纯逻辑。投屏时含答案的步骤默认遮住，教师按“揭示”后才显示，
 * 防止讲到中途答案提前出现在大屏上。只用于教师端；学生端的答案开放仍由服务端决定。
 */
export type ClassroomBeat = { id: string; title: string; say: string; board: string[] | null; reveals_answer?: boolean };

export type Shown = { title: string; lines: string[]; caption: string; masked: boolean };

export function shownBeat(b: ClassroomBeat, revealed: ReadonlySet<string>): Shown {
    const masked = !!b.reveals_answer && !revealed.has(b.id);
    if (masked) return { title: b.title, lines: [], caption: "", masked: true };
    return { title: b.title, lines: b.board ?? [], caption: b.say, masked: false };
}

/** 键盘：右箭头、空格、PageDown 下一步；左箭头、PageUp 上一步；A 揭示；Home/End 首尾。 */
export type Key = "next" | "prev" | "reveal" | "first" | "last" | null;
export function keyAction(key: string): Key {
    if (key === "ArrowRight" || key === " " || key === "PageDown") return "next";
    if (key === "ArrowLeft" || key === "PageUp") return "prev";
    if (key === "a" || key === "A") return "reveal";
    if (key === "Home") return "first";
    if (key === "End") return "last";
    return null;
}

export function step(i: number, n: number, k: Key): number {
    if (n <= 0) return 0;
    if (k === "next") return Math.min(i + 1, n - 1);
    if (k === "prev") return Math.max(i - 1, 0);
    if (k === "first") return 0;
    if (k === "last") return n - 1;
    return i;
}
