import { expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { readSourceText } from "./helpers/module-group-source";
import { CanvasConnectionCreateMenu } from "../src/components/canvas/canvas-workspace-overlays";
import { CanvasNodeType } from "../src/types/canvas";

const component = readSourceText(new URL("../src/components/canvas/canvas-workspace-overlays.tsx", import.meta.url));
const css = readSourceText(new URL("../src/styles/globals.css", import.meta.url));

test("connection menu omits the header and provides descriptions for every option", () => {
    expect(component).not.toContain("引用当前节点");
    expect(component).not.toContain(">创建下一步</span>");
    expect(component).not.toContain("ChevronRight");
    expect(component).not.toContain("absolute inset-x-8 top-0 h-px");
    const options = component.match(/<ConnectionCreateOption\b[\s\S]*?\n\s*\/>/g) || [];
    expect(options).toHaveLength(11);
    for (const option of options) expect(option).toMatch(/description="[^"]+"/);
    expect(options.map((option) => ({
        title: option.match(/title="([^"]+)"/)?.[1],
        description: option.match(/description="([^"]+)"/)?.[1],
        type: option.match(/onCreate\(CanvasNodeType\.(\w+)\)/)?.[1],
        disabledType: option.match(/getDisabledReason\(CanvasNodeType\.(\w+)\)/)?.[1],
    }))).toEqual([
        { title: "文本生成", description: "引用当前内容，生成或改写文本", type: "Text", disabledType: "Text" },
        { title: "分镜脚本", description: "根据剧情拆解镜头，编排分镜脚本", type: "Script", disabledType: "Script" },
        { title: "批量创作表", description: "汇总多张图片，批量执行换装或创意生图", type: "BatchTable", disabledType: "BatchTable" },
        { title: "图片生成", description: "结合提示词和参考图，生成新的画面", type: "Image", disabledType: "Image" },
        { title: "生成配置", description: "选择模型，或使用已启用的工作流插件", type: "Config", disabledType: "Config" },
        { title: "绘图", description: "以参考图片为底图，自由绘制和标注", type: "Drawing", disabledType: "Drawing" },
        { title: "视频生成", description: "结合提示词与参考素材，生成动态视频", type: "Video", disabledType: "Video" },
        { title: "音频参考", description: "连接文本或角色卡，创建音频生成节点", type: "Audio", disabledType: "Audio" },
        { title: "转换", description: "本地处理图片或视频", type: "MediaConversion", disabledType: "MediaConversion" },
        { title: "数据源", description: "接入 JSON、CSV、Markdown、TXT 或情报条目", type: "DataSource", disabledType: "DataSource" },
        { title: "逻辑与集合", description: "If/Else 分支、交集、并集、差集与过滤", type: "Logic", disabledType: "Logic" },
    ]);
    expect(component).toContain('title="批量创作表"');
    expect(component).toContain("onCreate(CanvasNodeType.BatchTable)");
});

test("real connection menu keeps conditional drawing and disabled reasons", () => {
    for (const canCreateDrawing of [true, false]) {
        const html = renderToStaticMarkup(createElement(CanvasConnectionCreateMenu, {
            pending: { position: { x: 20, y: 20 }, connection: { nodeId: "source", side: "right" } },
            viewport: { x: 0, y: 0, k: 1 }, viewportSize: { width: 1280, height: 800 },
            containerRef: { current: null }, canCreateDrawing,
            getDisabledReason: (type) => type === CanvasNodeType.DataSource ? "测试禁用" : "",
            onCreate: () => { throw new Error("SSR must not create nodes"); }, onClose: () => {},
        }));
        const labels = [...html.matchAll(/data-create-option="([^"]+)"/g)].map((match) => match[1]);
        const expected = ["文本生成", "分镜脚本", "批量创作表", "图片生成", "生成配置", ...(canCreateDrawing ? ["绘图"] : []), "视频生成", "音频参考", "转换", "数据源", "逻辑与集合"];
        expect(labels).toEqual(expected);
        expect(labels).toHaveLength(canCreateDrawing ? 11 : 10);
        const disabled = html.match(/<button\b[^>]*data-create-option="数据源"[^>]*>/)?.[0] || "";
        expect(disabled).toContain('aria-disabled="true"');
        expect(disabled).toContain('aria-description="测试禁用"');
    }
});

test("add-node submenu uses a four-column medium-size grid", () => {
    const menu = readSourceText(new URL("../src/components/canvas/canvas-create-menu.tsx", import.meta.url));
    const context = readSourceText(new URL("../src/components/canvas/canvas-context-menu.tsx", import.meta.url));
    const styles = readSourceText(new URL("../src/styles/globals.css", import.meta.url));
    expect(menu).toContain('variant === "node" ? "grid-cols-4"');
    expect(menu).toContain("size-6 shrink-0");
    expect(context).toContain("w-[360px]");
    expect(styles).toContain("--canvas-create-node-height: var(--space-16)");
});

test("only the active option expands; stationary pointer retargeting cannot switch rows", () => {
    const descriptionRule = css.match(/\.canvas-connection-create-description \{([^}]+)\}/)?.[1] || "";
    expect(descriptionRule).toContain("grid-template-rows: 0fr");
    expect(descriptionRule).not.toContain("min-height");
    const option = component.slice(component.indexOf("function ConnectionCreateOption("), component.indexOf("function clamp("));
    expect(option).not.toContain("motion.button");
    expect(css).toContain('.canvas-connection-create-option[data-expanded="true"]');
    expect(component).toContain("previous?.x === event.clientX && previous.y === event.clientY");
    expect(component).not.toContain("SpotlightSurface");
    expect(css).toContain('.canvas-connection-create-option[data-motion="reduced"]');
    expect(component).toContain("aria-disabled={Boolean(disabledReason)}");
    expect(component).toContain("if (!disabledReason) onClick()");
    expect(component).toContain('event.key === "Escape"');
});
