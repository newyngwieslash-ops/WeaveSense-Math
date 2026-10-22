import { describe, expect, test } from "bun:test";
import type { ComponentProps } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { CanvasNode } from "../src/components/canvas/canvas-node";
import { CanvasNodeType } from "../src/types/canvas";

const noop = () => {};
const reference = { id: "text-1", nodeId: "node", kind: "text" as const, label: "文本1", title: "测试编号", active: false };

function render(options: Partial<ComponentProps<typeof CanvasNode>> = {}) {
    return renderToStaticMarkup(
        <CanvasNode
            data={{ id: "node", type: CanvasNodeType.DataSource, title: "数据源", position: { x: 0, y: 0 }, width: 340, height: 240, metadata: {} }}
            scale={1} isSelected={false} isRelated={false} isFocusRelated={false} isConnectionTarget={false} showImageInfo={false}
            onMouseDown={noop} onHoverStart={noop} onHoverEnd={noop} onConnectStart={noop} onResize={noop} onContentChange={noop} onContextMenu={noop}
            {...options}
        />,
    );
}

// Inspect the real component's rendered hit-target contract; browser geometry is checked separately.
function classes(markup: string, tag: RegExp) {
    const opening = markup.match(tag)?.[0];
    expect(opening).toBeDefined();
    return opening!.match(/class="([^"]*)"/)![1].split(/\s+/);
}
const track = /<div[^>]*class="[^"]*right-3 top-3[^>]*>/;
const content = /<div[^>]*class="relative flex h-full w-full items-center justify-center[^>]*>/;
const image = { id: "image", type: CanvasNodeType.Image, title: "图片", position: { x: 0, y: 0 }, width: 340, height: 240, metadata: { content: "data:image/png;base64,AA==", batchRootId: "root" } };

describe("节点状态轨道命中边界", () => {
    test("默认、选中和只读时编号与锁的外轨道均不截获下层按钮", () => {
        for (const options of [{}, { isSelected: true }, { readOnly: true }]) {
            const markup = render({ ...options, resourceLabel: reference });
            expect(classes(markup, track)).toContain("pointer-events-none");
            expect(classes(markup, /<span[^>]*title="测试编号"[^>]*>/)).toContain("pointer-events-none");
        }
        const markup = render({ data: { ...image, metadata: { locked: true } } });
        expect(classes(markup, track)).toContain("pointer-events-none");
        expect(classes(markup, /<span[^>]*title="节点已锁定"[^>]*>/)).toContain("pointer-events-none");
    });

    test("仅可见轨道的数据源与逻辑节点为顶栏留位，其他分支不挤占正文", () => {
        for (const type of [CanvasNodeType.DataSource, CanvasNodeType.Logic]) {
            const data = { ...image, type, metadata: {} };
            expect(classes(render({ data, resourceLabel: reference }), content)).toContain("pt-8");
            expect(classes(render({ data }), content)).not.toContain("pt-8");
            expect(classes(render({ data, resourceLabel: reference, renderLOD: "preview" }), content)).not.toContain("pt-8");
        }
        expect(classes(render({ data: image, resourceLabel: reference }), content)).not.toContain("pt-8");
    });

    test("显示的主图按钮恢复命中，隐藏主图仍穿透，只读不出现按钮", () => {
        const primary = /<button[^>]*aria-label="(?:设置为主图|当前主图)"[^>]*>/;
        expect(classes(render({ data: image, isSelected: true }), primary)).toContain("pointer-events-auto");
        const hidden = classes(render({ data: image }), primary);
        expect(hidden).toContain("pointer-events-none");
        expect(hidden).not.toContain("pointer-events-auto");
        expect(render({ data: image, readOnly: true })).not.toMatch(primary);
    });

    test("图片组切换、图层入口和存储说明独立恢复命中", () => {
        const data = { ...image, metadata: { content: image.metadata.content, storageKey: "local:test-image", isBatchRoot: true, imageLayerGroup: { width: 100, height: 100, layers: [], compositeStatus: "ready" as const } } };
        const markup = render({ data, batchCount: 2 });
        expect(classes(markup, /<button[^>]*aria-label="展开图片组"[^>]*>/)).toContain("pointer-events-auto");
        expect(classes(markup, /<button[^>]*aria-label="管理图层"[^>]*>/)).toContain("pointer-events-auto");
        expect(classes(markup, /<span[^>]*title="[^"]*"[^>]*>本地/)).toContain("pointer-events-auto");
        expect(render({ data, batchCount: 2, readOnly: true })).not.toContain('aria-label="管理图层"');
    });
});
