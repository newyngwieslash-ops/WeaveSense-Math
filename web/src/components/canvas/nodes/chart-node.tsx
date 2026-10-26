import { ChartColumn } from "lucide-react";
import { Bar, BarChart, CartesianGrid, Legend, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";

import { useUpstreamDataflowSource } from "@/components/canvas/canvas-node-graph-context";
import { NodeRunSnapshotBar } from "@/components/canvas/nodes/node-run-snapshot-bar";
import { parseChartData, type ChartFieldConfig } from "@/lib/canvas/chart-data";
export { parseChartSource } from "@/lib/canvas/chart-data";
import type { CanvasTheme } from "@/lib/canvas-theme";
import type { CanvasNodeData } from "@/types/canvas";

type ChartNodeContentProps = {
    node: CanvasNodeData;
    theme: CanvasTheme;
};

const SERIES_COLORS = ["#f59e0b", "#22c55e", "#06b6d4", "#a855f7", "#ef4444"];

/**
 * 图表节点：把上游数据流（数据源 / 逻辑算子 / 技能 / 文本节点的 JSON 数组或 CSV）渲染成柱状图/折线图。
 * metadata.chartKind 为 "line" 时画折线，缺省柱状。
 */
export function ChartNodeContent({ node, theme }: ChartNodeContentProps) {
    const upstream = useUpstreamDataflowSource(node.id);
    const source = upstream.connected ? upstream.text : node.metadata?.content || "";
    const { chart: parsed, error } = parseChartData(upstream.connected ? upstream.sources : source, (node.metadata ?? {}) as ChartFieldConfig);
    const asLine = node.metadata?.chartKind === "line";

    if (!parsed || upstream.error) {
        return (
            <div className="flex h-full w-full flex-col" style={{ color: theme.node.muted }}>
                <NodeRunSnapshotBar
                    nodeId={node.id}
                    snapshot={node.metadata?.lastRunSnapshot}
                    derived={node.metadata?.derived}
                />
                <div className="flex h-full w-full flex-col items-center justify-center gap-2 px-4 text-center">
                    <ChartColumn className="size-5 opacity-60" />
                    <span style={{ fontSize: "var(--fs-label)" }}>
                        {upstream.error ? `上游数据错误：${upstream.error}` : error || (source.trim() ? "无法解析为图表数据（需要 JSON 数组或 CSV）" : upstream.connected ? "当前上游数据为空" : "连接数据源、逻辑聚合或输出 JSON/CSV 的节点")}
                    </span>
                </div>
            </div>
        );
    }

    return (
        <div className="flex h-full w-full flex-col" data-canvas-no-zoom style={{ color: theme.node.text }} onWheel={(event) => event.stopPropagation()} onMouseDown={(event) => event.stopPropagation()}>
            <NodeRunSnapshotBar
                nodeId={node.id}
                snapshot={node.metadata?.lastRunSnapshot}
                derived={node.metadata?.derived}
            />
            <div className="min-h-0 flex-1 w-full px-2 py-2">
                <ResponsiveContainer width="100%" height="100%">
                    {asLine ? (
                        <LineChart data={parsed.rows}>
                            <CartesianGrid stroke={theme.node.stroke} strokeDasharray="3 3" />
                            <XAxis dataKey={parsed.xKey} tick={{ fill: theme.node.muted, fontSize: 11 }} />
                            <YAxis tick={{ fill: theme.node.muted, fontSize: 11 }} />
                            <Tooltip />
                            {parsed.series.length > 1 ? <Legend /> : null}
                            {parsed.series.map((key, index) => (
                                <Line key={key} type="monotone" dataKey={key} stroke={SERIES_COLORS[index % SERIES_COLORS.length]} dot={false} />
                            ))}
                        </LineChart>
                    ) : (
                        <BarChart data={parsed.rows}>
                            <CartesianGrid stroke={theme.node.stroke} strokeDasharray="3 3" />
                            <XAxis dataKey={parsed.xKey} tick={{ fill: theme.node.muted, fontSize: 11 }} />
                            <YAxis tick={{ fill: theme.node.muted, fontSize: 11 }} />
                            <Tooltip />
                            {parsed.series.length > 1 ? <Legend /> : null}
                            {parsed.series.map((key, index) => (
                                <Bar key={key} dataKey={key} fill={SERIES_COLORS[index % SERIES_COLORS.length]} />
                            ))}
                        </BarChart>
                    )}
                </ResponsiveContainer>
            </div>
        </div>
    );
}

const FALLBACK_CHART_THEME = {
    node: { fill: "#181818", panel: "#141414", stroke: "rgba(255,255,255,.12)", edge: "rgba(255,255,255,.18)", text: "#ededed", muted: "#a3a3a3" },
    accent: { primary: "#171717", onPrimary: "#ffffff" },
} as unknown as CanvasTheme;

export function ChartNode({
    node,
    theme = FALLBACK_CHART_THEME,
}: {
    node: CanvasNodeData;
    theme?: CanvasTheme;
    isSelected?: boolean;
    onUpdateNode?: unknown;
    onDelete?: unknown;
}) {
    return <ChartNodeContent node={node} theme={theme} />;
}
