import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import { isValidElement, type ReactElement, type ReactNode } from "react";
import * as SourceModule from "@/components/canvas/nodes/data-source-node";
import { CanvasNodeActionContext } from "@/components/canvas/canvas-node-action-context";
import { CanvasNodeType, type CanvasDataSourceBinding, type CanvasNodeData } from "@/types/canvas";
import { canvasThemes } from "@/lib/canvas-theme";

const approved = [{ id: "connection", name: "Approved lookup", version: 3, status: "approved", allowedTools: ["lookup"], approvedTools: { lookup: { hash: "approved-schema-hash", inputSchema: { type: "object", required: ["query"] }, description: "Lookup" } } }];
function elements(node: ReactNode): ReactElement<Record<string, any>>[] {
    if (Array.isArray(node)) return node.flatMap(elements);
    if (!isValidElement(node)) return [];
    const element = node as ReactElement<Record<string, any>>;
    return [element, ...elements(element.props.children)];
}
function harness(binding: CanvasDataSourceBinding = { kind: "static" }) {
    let node: CanvasNodeData = { id: "data", type: CanvasNodeType.DataSource, title: "data", position: { x: 0, y: 0 }, width: 400, height: 500, metadata: { dataSource: { format: "json", rawText: '[{"keep":true}]', sourceBinding: binding } } };
    const render = () => {
        let tree: ReactNode;
        function Capture() { tree = SourceModule.DataSourceNodeContent({ node, theme: canvasThemes.light }); return tree; }
        const Provider = (SourceModule as any).CanvasSourceBindingContext?.Provider;
        const capture = <CanvasNodeActionContext.Provider value={{ updateNode: (_id, fn) => { node = fn(node); } }}><Capture /></CanvasNodeActionContext.Provider>;
        const html = renderToStaticMarkup(Provider ? <Provider value={{ connections: approved, loading: false }}>{capture}</Provider> : capture);
        return { html, field: (label: string) => { const field = elements(tree).find(el => el.props["aria-label"] === label); expect(field).toBeDefined(); return field!; } };
    };
    return { render, node: () => node };
}

describe("T18 real source binding component", () => {
    test("five bindings are editable without discarding static source data", () => {
        const h = harness();
        for (const kind of ["static", "intel_query", "topic", "monitor", "mcp_tool"]) {
            h.render().field("数据源绑定类型").props.onChange({ target: { value: kind } });
            expect(h.node().metadata?.dataSource?.sourceBinding?.kind).toBe(kind);
            expect(h.node().metadata?.dataSource?.rawText).toBe('[{"keep":true}]');
        }
        expect(h.render().html).toContain("服务端");
    });
    test("query window persists an object and switching kind drops incompatible identifiers", () => {
        const h = harness({ kind: "intel_query", query: "original" });
        h.render().field("情报查询").props.onChange({ target: { value: "new query" } });
        h.render().field("查询时间窗口").props.onChange({ target: { value: "7d" } });
        expect(h.node().metadata?.dataSource?.sourceBinding).toMatchObject({ kind: "intel_query", query: "new query", timeWindow: { kind: "7d", timezone: "UTC" } });
        h.render().field("数据源绑定类型").props.onChange({ target: { value: "topic" } });
        h.render().field("绑定对象 ID").props.onChange({ target: { value: "topic-1" } });
        expect(h.node().metadata?.dataSource?.sourceBinding).toEqual({ kind: "topic", refId: "topic-1" });
    });
    test("MCP pins only approved version/schema and writes canonical argument fields", () => {
        const h = harness({ kind: "mcp_tool" });
        h.render().field("已批准 MCP 工具").props.onChange({ target: { value: JSON.stringify(["connection", "lookup"]) } });
        h.render().field("MCP 参数 JSON").props.onBlur({ target: { value: '{"query":"abc"}' } });
        h.render().field("MCP 输出 JSON Schema").props.onBlur({ target: { value: '{"type":"array","items":{"type":"object"}}' } });
        expect(h.node().metadata?.dataSource?.sourceBinding).toMatchObject({ refId: "connection", toolName: "lookup", toolVersion: 3, schemaHash: "approved-schema-hash", toolArgs: { query: "abc" }, outputSchema: { type: "array", items: { type: "object" } } });
        const saved = JSON.stringify(h.node().metadata?.dataSource?.sourceBinding);
        for (const key of ['"tool":', '"arguments":', '"schemaVersion":']) expect(saved).not.toContain(key);
        expect(h.render().html).toContain("输入 Schema");
        const before = saved;
        h.render().field("MCP 参数 JSON").props.onBlur({ target: { value: "[]" } });
        expect(JSON.stringify(h.node().metadata?.dataSource?.sourceBinding)).toBe(before);
    });
    test("remote source preview is explicitly a local cache, not fresh server output", () => {
        const h = harness({ kind: "monitor", refId: "monitor-1" });
        expect(h.render().html).toContain("本地缓存预览");
        expect(h.render().html).toContain("监控命中接口未接入");
    });
});
