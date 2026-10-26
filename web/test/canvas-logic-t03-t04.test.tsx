import { expect, test } from "bun:test";
import { createElement, type ReactElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { CanvasNodeActionContext } from "../src/components/canvas/canvas-node-action-context";
import { LogicNodeContent } from "../src/components/canvas/nodes/logic-node";
import type { CanvasTheme } from "../src/lib/canvas-theme";
import { CanvasNodeType, type CanvasNodeData } from "../src/types/canvas";
import { parseChartData } from "../src/lib/canvas/chart-data";
import { evaluateLogicNode } from "../src/lib/canvas/dataflow-evaluator";

const theme = { node: { fill: "white", panel: "white", edge: "gray", text: "black", muted: "gray" }, accent: { primary: "blue", onPrimary: "white" } } as CanvasTheme;
test("T03/T04 real editor persists arrays, metric aliases/p95, composite join and dedupe", () => {
    let node: CanvasNodeData = { id: "logic", type: CanvasNodeType.Logic, title: "logic", position: { x: 0, y: 0 }, width: 400, height: 500, metadata: { logic: { operator: "aggregate", groupBy: [], metrics: [{ field: "v", fn: "avg", as: "mean" }] } } };
    function render() {
        let tree: ReactElement = createElement("div");
        function Probe() { tree = LogicNodeContent({ node, theme }); return null; }
        renderToStaticMarkup(createElement(CanvasNodeActionContext.Provider, { value: { updateNode: (_id, update) => { node = update(node); } } }, createElement(Probe)));
        return tree;
    }
    function find(label: string): ReactElement {
        let found: ReactElement | undefined;
        function visit(child: ReactNode) {
            if (Array.isArray(child)) child.forEach(visit);
            else if (child && typeof child === "object" && "props" in child) {
                const props = child.props as { "aria-label"?: string; children?: ReactNode };
                if (props["aria-label"] === label) found = child as ReactElement;
                visit(props.children);
            }
        }
        visit(render()); expect(found).toBeDefined(); return found!;
    }
    const change = (label: string, value: string) => (find(label).props as { onChange: (event: unknown) => void }).onChange({ target: { value } });
    const click = (label: string) => (find(label).props as { onClick: () => void }).onClick();
    change("聚合分组字段", "region,");
    expect((find("聚合分组字段").props as { value: string }).value).toBe("region,");
    change("聚合分组字段", "region,substance,period");
    click("添加聚合指标"); change("聚合函数 2", "p95"); change("聚合目标字段 2", "v"); change("聚合别名 2", "peak");
    expect(node.metadata?.logic).toMatchObject({ groupBy: ["region", "substance", "period"], metrics: [{ field: "v", fn: "avg", as: "mean" }, { field: "v", fn: "p95", as: "peak" }] });
    node = { ...node, metadata: { ...node.metadata, logic: { ...node.metadata!.logic!, operator: "join", joinKeys: [{ left: "region", right: "area" }] } } };
    click("添加关联键"); change("关联左键 2", "substance"); change("关联右键 2", "chemical"); change("关联模式", "left");
    expect(node.metadata?.logic).toMatchObject({ joinMode: "left", joinKeys: [{ left: "region", right: "area" }, { left: "substance", right: "chemical" }] });
    node = { ...node, metadata: { ...node.metadata, logic: { ...node.metadata!.logic!, operator: "sort_limit" } } };
    change("去重字段", "region,substance"); change("截取条数", "3");
    expect(node.metadata?.logic).toMatchObject({ dedupeKeys: ["region", "substance"], limit: 3 });
    change("截取条数", "0"); expect(node.metadata?.logic?.limit).toBe(0);
});

test("review malformed logic metadata renders a repairable error instead of crashing", () => {
    for (const logic of [{ operator: "aggregate", groupBy: "region" }, { operator: "aggregate", metrics: [null] }, { operator: "join", joinKeys: null }]) {
        let node = { id: "bad", type: CanvasNodeType.Logic, title: "bad", position: { x: 0, y: 0 }, width: 400, height: 500, metadata: { logic } } as unknown as CanvasNodeData;
        let tree: ReactElement = createElement("div");
        function Probe() { tree = LogicNodeContent({ node, theme }); return tree; }
        const html = renderToStaticMarkup(createElement(CanvasNodeActionContext.Provider, { value: { updateNode: (_id, update) => { node = update(node); } } }, createElement(Probe)));
        expect(html).toContain("配置格式错误");
        let repair: (() => void) | undefined;
        function visit(child: ReactNode) {
            if (Array.isArray(child)) child.forEach(visit);
            else if (child && typeof child === "object" && "props" in child) {
                const props = child.props as { "aria-label"?: string; onClick?: () => void; children?: ReactNode };
                if (props["aria-label"] === "重置非法算子配置") repair = props.onClick;
                visit(props.children);
            }
        }
        visit(tree); expect(repair).toBeDefined(); repair!();
        expect(evaluateLogicNode(node, [], []).error).toBeUndefined();
    }
});

test("T04 joined literal right fields remain available to chart data", () => {
    const node = { id: "join", type: CanvasNodeType.Logic, metadata: { logic: { operator: "join", joinKeys: [{ left: "id", right: "id" }] } } } as CanvasNodeData;
    const result = evaluateLogicNode(node, [{ id: 1, value: 23 }], [{ id: 1, value: 27.6 }]);
    expect(result.error).toBeUndefined();
    const parsed = parseChartData(JSON.stringify(result.outputItems));
    expect(JSON.stringify(parsed)).toContain('"right.value":27.6');
});
