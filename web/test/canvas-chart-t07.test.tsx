import { describe, expect, test } from "bun:test";
import { createElement, type ReactElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";

import { CanvasNodeGraphContext, useUpstreamDataflowText, type CanvasNodeGraphContextValue } from "../src/components/canvas/canvas-node-graph-context";
import { ChartNodeContent } from "../src/components/canvas/nodes/chart-node";
import { evaluateCanvasDataflow } from "../src/lib/canvas/dataflow-evaluator";
import { parseChartData } from "../src/lib/canvas/chart-data";
import { getContextResourceNodes } from "../src/lib/canvas/canvas-resource-references";
import type { CanvasTheme } from "../src/lib/canvas-theme";
import { CanvasNodeType, type CanvasConnection, type CanvasNodeData } from "../src/types/canvas";

const theme = { node: { muted: "gray", text: "black", stroke: "gray" } } as CanvasTheme;
const oldRows = '[{"label":"stale","value":999}]';
function node(id: string, type: CanvasNodeType, metadata: CanvasNodeData["metadata"] = {}): CanvasNodeData {
    return { id, type, title: id, position: { x: 0, y: 0 }, width: 320, height: 240, metadata };
}
function source(id: string, rows: unknown[]) {
    return node(id, CanvasNodeType.DataSource, { dataSource: { format: "json", rawText: JSON.stringify(rows) } });
}
function edge(fromNodeId: string, fromHandleId?: string, relation?: CanvasConnection["relation"]): CanvasConnection {
    return { id: `${fromNodeId}-${fromHandleId ?? "out"}-${relation ?? "data"}`, fromNodeId, fromHandleId, toNodeId: "chart", relation };
}
function context(nodes: CanvasNodeData[], connections: CanvasConnection[]): CanvasNodeGraphContextValue & { connections: CanvasConnection[] } {
    return { connections, dataflow: evaluateCanvasDataflow(nodes, connections), getUpstreamNodes: (id) => getContextResourceNodes(id, nodes, connections) };
}
function upstreamText(value: CanvasNodeGraphContextValue): string {
    let result = "";
    function Probe() { result = useUpstreamDataflowText("chart"); return null; }
    renderToStaticMarkup(createElement(CanvasNodeGraphContext.Provider, { value }, createElement(Probe)));
    return result;
}
function chartResult(chart: CanvasNodeData, value: CanvasNodeGraphContextValue = {}) {
    let result: ReactElement = createElement("div");
    function Probe() { result = ChartNodeContent({ node: chart, theme }); return null; }
    renderToStaticMarkup(createElement(CanvasNodeGraphContext.Provider, { value }, createElement(Probe)));
    return result;
}
function chartData(result: ReactElement): Record<string, unknown>[] | undefined {
    const children = (result.props as { children?: ReactNode }).children;
    for (const child of Array.isArray(children) ? children : [children]) {
        if (child && typeof child === "object" && "props" in child) {
            const props = child.props as { data?: Record<string, unknown>[] };
            if (props.data) return props.data;
            const found = chartData(child as ReactElement);
            if (found) return found;
        }
    }
}
function chartAxes(result: ReactElement): { xKey?: string; series: string[] } {
    const keys: string[] = [];
    function visit(element: ReactElement) {
        const props = element.props as { dataKey?: string; children?: ReactNode };
        if (props.dataKey) keys.push(props.dataKey);
        const walk = (child: ReactNode): void => {
            if (Array.isArray(child)) child.forEach(walk);
            else if (child && typeof child === "object" && "props" in child) visit(child as ReactElement);
        };
        walk(props.children);
    }
    visit(result);
    return { xKey: keys[0], series: keys.slice(1) };
}

const chart = node("chart", CanvasNodeType.Chart, { content: oldRows });
const rows = [{ label: "pass", value: 90 }, { label: "reject", value: 20 }];
function branchGraph(handle: string, input = rows) {
    const data = source("source", input);
    const logic = node("branch", CanvasNodeType.Logic, { logic: { operator: "if_else", fieldPath: "value", comparator: "gt", compareValue: "80" } });
    return context([data, logic, chart], [{ id: "input", fromNodeId: "source", toNodeId: "branch" }, edge("branch", handle)]);
}

describe("T07 real graph inputs", () => {
    test("false handle consumes false rows rather than node-level true output", () => {
        expect(JSON.parse(upstreamText(branchGraph("false")))).toEqual([{ label: "reject", value: 20 }]);
    });
    test("merges every incoming data edge, not only the first", () => {
        const value = context([source("a", [rows[0]]), source("b", [rows[1]]), chart], [edge("a"), edge("b")]);
        expect(JSON.parse(upstreamText(value))).toEqual(rows);
    });
    test("merges CSV text and structured upstream without dropping either", () => {
        const csv = node("csv", CanvasNodeType.Text, { content: "label,value\ncsv,30" });
        const value = context([csv, source("json", [rows[0]]), chart], [edge("csv"), edge("json")]);
        expect(chartData(chartResult(chart, value))).toEqual([{ label: "csv", value: 30 }, rows[0]]);
    });
    test("invalid upstream must not disappear behind another valid input", () => {
        const invalid = node("invalid", CanvasNodeType.Text, { content: "not a chart" });
        const value = context([invalid, source("json", [rows[0]]), chart], [edge("invalid"), edge("json")]);
        expect(chartData(chartResult(chart, value))).toBeUndefined();
    });
    test("two handles from one node remain distinct edges", () => {
        const value = branchGraph("false");
        value.connections.push(edge("branch", "true"));
        expect(JSON.parse(upstreamText(value))).toEqual([rows[1], rows[0]]);
    });
    test("batch/storyboard output relations are not data inputs", () => {
        const value = context([source("a", [rows[0]]), source("b", [rows[1]]), chart], [edge("a", undefined, "batch-output"), edge("b", undefined, "storyboard-output")]);
        expect(upstreamText(value)).toBe("");
    });
    test("current data replaces old chart content", () => {
        expect(chartData(chartResult(chart, branchGraph("false")))).toEqual([rows[1]]);
    });
    test("empty current branch clears the old chart", () => {
        const result = chartResult(chart, branchGraph("false", [rows[0]]));
        expect(chartData(result)).toBeUndefined();
        expect(renderToStaticMarkup(result)).toContain("当前上游数据为空");
    });
    test("empty data source clears the old chart", () => {
        expect(chartData(chartResult(chart, context([source("empty", []), chart], [edge("empty")])))).toBeUndefined();
    });
    test("full output beyond five rows reaches the chart", () => {
        const full = Array.from({ length: 9 }, (_, i) => ({ label: `row-${i}`, value: i + 1 }));
        const result = chartData(chartResult(chart, context([source("full", full), chart], [edge("full")])));
        expect(result).toHaveLength(9);
        expect(result?.[8]).toEqual({ label: "row-8", value: 9 });
    });
    test("disconnected chart keeps standalone content", () => {
        expect(chartData(chartResult(chart, context([chart], [])))).toEqual([{ label: "stale", value: 999 }]);
    });
    test("Skill prompt composition must not masquerade as chart output", () => {
        const skill = node("skill", CanvasNodeType.Skill, { skillName: "Analysis", skillPrompts: ["Return JSON rows"] });
        const value = context([source("a", rows), skill, chart], [{ id: "skill-in", fromNodeId: "a", toNodeId: "skill" }, edge("skill")]);
        expect(chartData(chartResult(chart, value))).toBeUndefined();
    });
});

describe("T07 review regressions", () => {
    test("CSV missing column differs from an explicitly empty xKey cell", () => {
        const config = { xKey: "label", series: ["sales"] };
        const explicitEmpty = parseChartData("sales,label\n12,A\n15,", config);
        expect(explicitEmpty.error).toBeUndefined();
        expect(explicitEmpty.chart?.rows).toEqual([{ sales: 12, label: "A" }, { sales: 15, label: "" }]);

        const missing = parseChartData("sales,label\n12,A\n15", config);
        expect(missing.chart).toBeNull();
        expect(missing.error).toContain("图表配置错误");
        expect(missing.error).toContain("label");
    });
    test("large valid output appends every row without argument-limit overflow", () => {
        const input = JSON.stringify(Array.from({ length: 1_000_000 }, (_, index) => ({ label: `row-${index}`, sales: index })));
        let result: ReturnType<typeof parseChartData> | undefined;
        expect(() => { result = parseChartData(input, { xKey: "label", series: ["sales"] }); }).not.toThrow();
        expect(result?.error).toBeUndefined();
        expect(result?.chart?.rows).toHaveLength(1_000_000);
        expect(result?.chart?.rows[0]).toEqual({ label: "row-0", sales: 0 });
        expect(result?.chart?.rows[100_000]).toEqual({ label: "row-100000", sales: 100_000 });
        expect(result?.chart?.rows[999_999]).toEqual({ label: "row-999999", sales: 999_999 });
    });
});

describe("T07 explicit chart fields", () => {
    const content = '[{"label":"A","year":2025,"sales":12,"cost":3},{"label":"B","year":2026,"sales":15,"cost":4}]';
    function configured(config: { xKey?: string; series?: string[] }, text = content) {
        return node("chart", CanvasNodeType.Chart, { content: text, ...config });
    }
    test("explicit numeric xKey and selected series override inference", () => {
        const result = chartResult(configured({ xKey: "year", series: ["cost"] }));
        expect(chartAxes(result)).toEqual({ xKey: "year", series: ["cost"] });
    });
    test("missing explicit xKey reports configuration error", () => {
        expect(renderToStaticMarkup(chartResult(configured({ xKey: "missing" })))).toContain("图表配置错误");
    });
    test("missing explicit series reports configuration error instead of inference", () => {
        expect(renderToStaticMarkup(chartResult(configured({ series: ["missing"] })))).toContain("图表配置错误");
    });
    test("field missing from a later row reports configuration error", () => {
        expect(renderToStaticMarkup(chartResult(configured({ xKey: "label", series: ["sales"] }, '[{"label":"A","sales":12},{"label":"B","cost":4}]')))).toContain("图表配置错误");
    });
    test("non-numeric explicit series reports configuration error", () => {
        expect(renderToStaticMarkup(chartResult(configured({ series: ["label"] })))).toContain("图表配置错误");
    });
});
