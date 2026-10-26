import { FileText } from "lucide-react";
import { createContext, useContext, useState } from "react";

export type MarkdownManagedReport = {
    runId: string;
    nodeId: string;
    body: string;
    claims: unknown[];
    material_scope: { document_ids?: string[]; source_ids?: string[] };
    structured: Record<string, unknown>;
    reportId?: string;
    status: "ready" | "partial" | "succeeded";
    error?: string;
};

// Supplied by the managed-run owner, never inferred from editable Markdown.
// saveReport retries persistence for the frozen run; it must not rerun models.
export const MarkdownReportContext = createContext<{
    reports: Record<string, MarkdownManagedReport | undefined>;
    saveReport?: (runId: string, nodeId: string) => Promise<{ status: "partial" | "succeeded"; report_id?: string; error?: string }>;
}>({ reports: {} });

export function canSaveMarkdownReport(report: MarkdownManagedReport | undefined): boolean {
    return !!report?.runId && !!report.nodeId && !!report.body.trim() && report.claims.length > 0
        && !!(report.material_scope.document_ids?.length || report.material_scope.source_ids?.length)
        && Object.keys(report.structured).length > 0 && report.status !== "succeeded";
}

import { AIMessageMarkdown } from "@/components/ai/ai-message-markdown";
import { useUpstreamNodes } from "@/components/canvas/canvas-node-graph-context";
import { NodeRunSnapshotBar } from "@/components/canvas/nodes/node-run-snapshot-bar";
import { getNodeResourceKind } from "@/lib/canvas/node-registry";
import type { CanvasTheme } from "@/lib/canvas-theme";
import type { CanvasNodeData } from "@/types/canvas";

type MarkdownNodeContentProps = {
    node: CanvasNodeData;
    theme: CanvasTheme;
};

/**
 * Markdown 展示节点。
 *
 * 内容取自身 metadata.content；为空时回落到上游文本素材——这样「AI 写文档 → 连一根线 →
 * 排版好的文档」不需要先把内容搬进本节点。上游取用统一经 CanvasNodeGraphContext，
 * 判定文本素材复用注册表的 resourceKind，不在这里另立一套判断。
 */
export function MarkdownNodeContent({ node, theme }: MarkdownNodeContentProps) {
    const upstream = useUpstreamNodes(node.id);
    const managed = useContext(MarkdownReportContext);
    const report = managed.reports[node.id];
    const [saving, setSaving] = useState(false);
    const [saveResult, setSaveResult] = useState<{ runId: string; status: string; reportId?: string; error?: string }>();
    const currentResult = saveResult?.runId === report?.runId ? saveResult : undefined;
    const reportId = currentResult?.reportId || report?.reportId;
    const saved = !!reportId?.startsWith("rpt_");
    const save = async () => {
        if (!report || !managed.saveReport || !canSaveMarkdownReport(report) || saving) return;
        setSaving(true);
        try {
            const result = await managed.saveReport(report.runId, node.id);
            if (result.status !== "succeeded" || !result.report_id?.startsWith("rpt_")) throw new Error(result.error || "报告保存未完成，可重试保存（不会重跑模型）");
            setSaveResult({ runId: report.runId, status: result.status, reportId: result.report_id });
        } catch (error) {
            setSaveResult({ runId: report.runId, status: "partial", error: error instanceof Error ? error.message : "报告保存失败" });
        } finally { setSaving(false); }
    };
    const own = node.metadata?.content || "";
    const inherited = upstream.find((item) => getNodeResourceKind(item) === "text");
    const source = own || inherited?.metadata?.content || inherited?.metadata?.prompt || "";

    return (
        <div className="flex h-full w-full flex-col" data-canvas-no-zoom style={{ color: theme.node.text }}>
            <div className="flex flex-wrap items-center gap-2 px-4 py-2" onPointerDown={(event) => event.stopPropagation()}>
                <button type="button" className="rounded px-2 py-1 text-xs disabled:opacity-50 focus-visible:outline" disabled={saving || saved || !managed.saveReport || !canSaveMarkdownReport(report)} onClick={() => void save()}>
                    {saving ? "保存中…" : saved ? "报告已保存" : currentResult?.status === "partial" || report?.status === "partial" ? "重试保存报告" : "保存报告"}
                </button>
                <a href="/intelligence/reports" className="text-xs underline focus-visible:outline" target="_blank" rel="noreferrer">{saved ? "打开报告列表" : "已有报告"}</a>
                {!managed.saveReport && <span className="text-xs" style={{ color: theme.node.muted }}>等待受管运行接入</span>}
                {(currentResult?.error || report?.error) && <span role="alert" className="w-full text-xs">部分完成：{currentResult?.error || report?.error}</span>}
            </div>
            <NodeRunSnapshotBar
                nodeId={node.id}
                snapshot={node.metadata?.lastRunSnapshot}
                derived={node.metadata?.derived}
            />
            <div className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden px-4 py-3" data-canvas-wheel-scroll onWheel={(event) => event.stopPropagation()}>
                {source.trim() ? <AIMessageMarkdown>{source}</AIMessageMarkdown> : <div className="flex h-full flex-col items-center justify-center gap-2 text-center" style={{ color: theme.node.muted }}>
                    <FileText className="size-5 opacity-60" />
                    <span style={{ fontSize: "var(--fs-label)" }}>连接文本节点，或双击编辑 Markdown</span>
                </div>}
            </div>
        </div>
    );
}

const FALLBACK_MARKDOWN_THEME = {
    node: { fill: "#181818", panel: "#141414", stroke: "rgba(255,255,255,.12)", edge: "rgba(255,255,255,.18)", text: "#ededed", muted: "#a3a3a3" },
    accent: { primary: "#171717", onPrimary: "#ffffff" },
} as unknown as CanvasTheme;

export function MarkdownNode({
    node,
    theme = FALLBACK_MARKDOWN_THEME,
}: {
    node: CanvasNodeData;
    theme?: CanvasTheme;
    isSelected?: boolean;
    onUpdateNode?: unknown;
    onDelete?: unknown;
}) {
    return <MarkdownNodeContent node={node} theme={theme} />;
}
