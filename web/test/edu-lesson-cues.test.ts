import { describe, expect, it } from "bun:test";
import { cueSummary, type EditableBeat } from "../src/components/edu/lesson-edit-logic";

// DEV-071：编辑器只读展示旁白锚点与绑定动作；旧版修订没有锚点时给出生成方式。
describe("旁白锚点摘要", () => {
    const beat: EditableBeat = {
        id: "s1",
        title: "设直线",
        say: "设直线。联立。",
        board: ["y = k(x − 1)", "x² = 4y"],
        cues: [
            {
                anchor: "s1.n0",
                text: "设直线。",
                est_ms: 1000,
                actions: [
                    { kind: "focus_figure", refs: ["F"] },
                    { kind: "show_board_line", line: 0 },
                ],
            },
            { anchor: "s1.n1", text: "联立。", est_ms: 800, actions: [{ kind: "show_board_line", line: 1 }] },
        ],
    };

    it("逐句列出动作并注明按文字顺序与字数估计", () => {
        expect(cueSummary(beat)).toBe("旁白分 2 句（按文字顺序，约 2 秒，字数估计）；第1句：图中标出F，板书第1行；第2句：板书第2行");
    });

    it("旧版修订没有锚点", () => {
        expect(cueSummary({ ...beat, cues: undefined })).toBe("旧版讲解没有旁白锚点，保存任意修改后自动生成");
        expect(cueSummary({ ...beat, cues: null })).toBe("旧版讲解没有旁白锚点，保存任意修改后自动生成");
    });

    it("未知动作与空动作不报错", () => {
        expect(cueSummary({ ...beat, cues: [{ anchor: "s1.n0", text: "设直线。联立。", actions: [{ kind: "zoom" }] }] })).toBe("旁白分 1 句（按文字顺序，约 0 秒，字数估计）；第1句：未知动作");
        expect(cueSummary({ ...beat, cues: [{ anchor: "s1.n0", text: "设直线。联立。", est_ms: 1600, actions: null }] })).toBe("旁白分 1 句（按文字顺序，约 2 秒，字数估计）；第1句");
    });
});
