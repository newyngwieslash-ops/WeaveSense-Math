import { expect, test } from "bun:test";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MarkdownNodeContent, MarkdownReportContext, type MarkdownManagedReport } from "../src/components/canvas/nodes/markdown-node";
import { CanvasNodeType, type CanvasNodeData } from "../src/types/canvas";
import type { CanvasTheme } from "../src/lib/canvas-theme";
const node: CanvasNodeData = { id: "md", type: CanvasNodeType.Markdown, title: "Markdown", position: { x: 0, y: 0 }, width: 320, height: 300, metadata: { content: "# Draft" } };
const theme = { node: { muted: "gray", text: "black" } } as CanvasTheme;
const report: MarkdownManagedReport = { runId: "run16", nodeId: "md", body: "body", claims: [{ text: "evidence" }], material_scope: { source_ids: ["source"] }, structured: { rows: [] }, status: "partial" };
test("T16 only a managed structured report enables save-only retry", () => {
 const render = (value: MarkdownManagedReport) => renderToStaticMarkup(createElement(MarkdownReportContext.Provider, { value: { reports: { md: value }, saveReport: async () => ({ status: "succeeded", report_id: "rpt_16" }) } }, createElement(MarkdownNodeContent, { node, theme })));
 expect(render(report)).toContain("重试保存报告"); expect(render(report)).not.toContain("disabled=\"\"");
 expect(render({ ...report, claims: [] })).toContain("disabled=\"\"");
 expect(render({ ...report, reportId: "rpt_16", status: "succeeded" })).toContain("报告已保存");
});
test("T16 markdown plain text cannot save; real reports route remains available", () => {
 const html = renderToStaticMarkup(createElement(MarkdownNodeContent, { node, theme }));
 expect(html).toContain("保存报告"); expect(html).toContain("disabled"); expect(html).toContain('/intelligence/reports');
});
