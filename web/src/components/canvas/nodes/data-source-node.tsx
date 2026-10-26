import { AlertCircle, Code2, Database, FileUp, TableProperties } from "lucide-react";
import { createContext, useContext, useRef, useState, type ChangeEvent } from "react";

import { useCanvasNodeActions } from "@/components/canvas/canvas-node-action-context";
import { useDataflowNodeResult } from "@/components/canvas/canvas-node-graph-context";
import * as NodeRunSnapshotBarModule from "@/components/canvas/nodes/node-run-snapshot-bar";
import type { CanvasTheme } from "@/lib/canvas-theme";
import {
    DATA_SOURCE_FORMAT_LABELS,
    type CanvasDataItem,
} from "@/lib/canvas/dataflow-evaluator";
import { readDataSourceFile } from "@/lib/canvas/data-source-file";
import type { CanvasDataSourceBinding, CanvasDataSourceBindingKind, CanvasDataSourceConfig, CanvasDataSourceFormat, CanvasNodeData } from "@/types/canvas";

export type CanvasApprovedMcpConnection = {
    id: string; name: string; version: number; status: string; allowedTools: string[] | null;
    approvedTools: Record<string, { hash: string; description?: string; inputSchema: unknown }>;
};
export const CanvasSourceBindingContext = createContext<{ connections: CanvasApprovedMcpConnection[]; loading: boolean; error?: string }>({ connections: [], loading: false });
const BINDINGS: [CanvasDataSourceBindingKind, string][] = [["static", "静态数据"], ["intel_query", "情报查询"], ["topic", "专题"], ["monitor", "监控"], ["mcp_tool", "已批准 MCP 工具"]];
const fieldClass = "w-full rounded-[var(--r-sm)] border border-border bg-background px-2 py-1 text-xs focus-visible:outline focus-visible:outline-2 focus-visible:outline-ring";

function bindingObject(text: string, schema = false): Record<string, unknown> {
    if (text.length > 32768) throw new Error("JSON 配置超过 32KB");
    const value: unknown = JSON.parse(text);
    if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("必须为 JSON 对象");
    const visit = (item: unknown, depth: number) => {
        if (depth > 16) throw new Error("JSON 嵌套过深");
        if (!item || typeof item !== "object") return;
        for (const [key, child] of Object.entries(item)) {
            if (["__proto__", "prototype", "constructor"].includes(key)) throw new Error("JSON 包含禁止字段");
            if (schema && ["$ref", "$dynamicRef", "$recursiveRef"].includes(key) && (typeof child !== "string" || !child.startsWith("#"))) throw new Error("输出 Schema 禁止远端引用");
            visit(child, depth + 1);
        }
    };
    visit(value, 0);
    return value as Record<string, unknown>;
}

type DataSourceNodeContentProps = {
    node: CanvasNodeData;
    theme: CanvasTheme;
};

const FORMAT_OPTIONS: CanvasDataSourceFormat[] = ["json", "csv", "txt", "markdown", "intel"];

const FORMAT_PLACEHOLDERS: Record<CanvasDataSourceFormat, string> = {
    json: '[\n  { "id": "1", "title": "条目 A", "score": 92 },\n  { "id": "2", "title": "条目 B", "score": 68 }\n]',
    csv: "id,title,score\n1,条目 A,92\n2,条目 B,68",
    txt: "每行一条文本记录\n第二条文本记录",
    markdown: "# 章节一\n第一部分内容\n\n# 章节二\n第二部分内容",
    intel: '[\n  { "id": "intel-1", "region": "华南A区", "substance": "甲基苯丙胺", "concentration_ng_l": 48.5, "flow_m3_day": 120000, "population": 850000 }\n]',
};

function formatPreviewItem(item: CanvasDataItem): string {
    if (typeof item === "string" || typeof item === "number" || typeof item === "boolean") {
        return String(item);
    }
    return JSON.stringify(item);
}

export function DataSourceNodeContent({ node, theme }: DataSourceNodeContentProps) {
    const { updateNode, updateMetadata } = useCanvasNodeActions();
    const result = useDataflowNodeResult(node);
    const fileInputRef = useRef<HTMLInputElement | null>(null);
    const [viewMode, setViewMode] = useState<"edit" | "preview">("edit");
    const [importError, setImportError] = useState<string>();
    const [bindingError, setBindingError] = useState<string>();
    const mcpLibrary = useContext(CanvasSourceBindingContext);

    const ds = node.metadata?.dataSource;
    const format: CanvasDataSourceFormat = ds?.format || result.format || "json";
    const rawText = ds?.rawText ?? node.metadata?.content ?? "";
    const fileName = ds?.fileName || "";

    const patchDataSource = (patch: Partial<CanvasDataSourceConfig>, nextContent?: string) => {
        const mergedConfig: CanvasDataSourceConfig = {
            ...ds,
            format,
            rawText,
            ...(fileName ? { fileName } : {}),
            ...patch,
        };
        const contentValue = nextContent !== undefined ? nextContent : (mergedConfig.rawText ?? rawText);
        if (updateNode) {
            updateNode(node.id, (current) => ({
                ...current,
                metadata: {
                    ...current.metadata,
                    content: contentValue,
                    dataSource: {
                        ...(current.metadata?.dataSource || { format }),
                        ...patch,
                    },
                },
            }));
            return;
        }
        updateMetadata?.(node.id, {
            content: contentValue,
            dataSource: mergedConfig,
        });
    };

    const binding = ds?.sourceBinding ?? { kind: "static" as const };
    const patchBinding = (patch: Partial<CanvasDataSourceBinding>) => {
        patchDataSource({ sourceBinding: { ...binding, ...patch, lastSyncedAt: undefined } });
        setBindingError(undefined);
    };
    const approvedTools = mcpLibrary.connections.filter(connection => connection.status === "approved" && Number.isSafeInteger(connection.version) && connection.version > 0).flatMap(connection =>
        (connection.allowedTools ?? []).flatMap(name => {
            const tool = connection.approvedTools?.[name];
            return tool?.hash ? [{ connection, name, tool, key: JSON.stringify([connection.id, name]) }] : [];
        }));
    const selectedTool = approvedTools.find(item => item.connection.id === binding.refId && item.name === binding.toolName);
    const jsonField = (key: "toolArgs" | "outputSchema", value: string) => {
        try { patchBinding({ [key]: bindingObject(value, key === "outputSchema") }); }
        catch (error) { setBindingError(error instanceof Error ? error.message : "JSON 配置无效"); }
    };

    const handleFileImport = async (event: ChangeEvent<HTMLInputElement>) => {
        const file = event.currentTarget.files?.[0];
        event.currentTarget.value = "";
        if (!file) return;
        try {
            const dataSource = await readDataSourceFile(file);
            patchDataSource(dataSource, dataSource.rawText);
            setImportError(undefined);
        } catch (error) {
            setImportError(error instanceof Error ? error.message : "数据源导入失败");
        }
    };

    return (
        <div
            data-canvas-no-zoom
            data-canvas-wheel-scroll
            className="flex h-full w-full flex-col overflow-hidden p-3 pt-4 text-xs select-none"
            style={{ background: theme.node.fill, color: theme.node.text }}
            onMouseDown={(event) => event.stopPropagation()}
            onPointerDown={(event) => event.stopPropagation()}
            onWheel={(event) => event.stopPropagation()}
        >
            <input
                ref={fileInputRef}
                type="file"
                accept=".json,.jsonl,.csv,.tsv,.txt,.md,.markdown"
                className="hidden"
                onChange={handleFileImport}
            />

            {/* 顶栏：格式切换 + 导入文件 + 视图切换 */}
            <div className="flex flex-wrap items-center justify-between gap-1.5 pb-2" style={{ borderBottom: `1px solid ${theme.node.edge}` }}>
                <div className="flex items-center gap-1" role="tablist" aria-label="数据源格式">
                    <Database className="mr-0.5 size-3.5 shrink-0" style={{ color: theme.accent.primary }} />
                    {FORMAT_OPTIONS.map((item) => {
                        const active = format === item;
                        return (
                            <button
                                key={item}
                                type="button"
                                aria-pressed={active}
                                className="rounded-[var(--r-sm)] px-1.5 py-0.5 text-[var(--fs-tiny)] font-medium transition-colors"
                                style={{
                                    background: active ? theme.accent.primary : theme.node.panel,
                                    color: active ? theme.accent.onPrimary : theme.node.muted,
                                }}
                                onClick={() => patchDataSource({ format: item })}
                            >
                                {DATA_SOURCE_FORMAT_LABELS[item]}
                            </button>
                        );
                    })}
                </div>

                <div className="flex items-center gap-1">
                    <button
                        type="button"
                        className="inline-flex items-center gap-1 rounded-[var(--r-sm)] border px-1.5 py-0.5 text-[var(--fs-tiny)] font-medium transition-opacity hover:opacity-90"
                        style={{ background: theme.node.panel, borderColor: theme.node.edge, color: theme.node.text }}
                        onClick={() => fileInputRef.current?.click()}
                        title="导入本地 JSON / CSV / TXT / Markdown 文件"
                    >
                        <FileUp className="size-3" />
                        导入
                    </button>
                    <button
                        type="button"
                        aria-pressed={viewMode === "preview"}
                        className="inline-flex items-center gap-1 rounded-[var(--r-sm)] border px-1.5 py-0.5 text-[var(--fs-tiny)] font-medium transition-colors"
                        style={{
                            background: viewMode === "preview" ? `${theme.accent.primary}22` : theme.node.panel,
                            borderColor: viewMode === "preview" ? theme.accent.primary : theme.node.edge,
                            color: viewMode === "preview" ? theme.accent.primary : theme.node.muted,
                        }}
                        onClick={() => setViewMode((current) => (current === "edit" ? "preview" : "edit"))}
                    >
                        {viewMode === "edit" ? <TableProperties className="size-3" /> : <Code2 className="size-3" />}
                        {viewMode === "edit" ? `预览 (${result.outputItems.length})` : "源码"}
                    </button>
                </div>
            </div>

            {NodeRunSnapshotBarModule.NodeRunSnapshotBar ? (
                <NodeRunSnapshotBarModule.NodeRunSnapshotBar
                    nodeId={node.id}
                    snapshot={node.metadata?.lastRunSnapshot}
                    derived={node.metadata?.derived}
                />
            ) : null}

            <details className="mt-2 max-h-[60%] shrink-0 overflow-y-auto" open={binding.kind !== "static"}>
                <summary className="cursor-pointer focus-visible:outline focus-visible:outline-2">绑定配置 · {BINDINGS.find(([kind]) => kind === binding.kind)?.[1]}</summary>
                <div className="space-y-2 py-2">
                    <label>数据源绑定类型<select aria-label="数据源绑定类型" className={fieldClass} value={binding.kind} onChange={event => { patchDataSource({ sourceBinding: { kind: event.target.value as CanvasDataSourceBindingKind } }); setBindingError(undefined); }}>
                        {BINDINGS.map(([kind, label]) => <option key={kind} value={kind}>{label}</option>)}
                    </select></label>
                    {binding.kind === "intel_query" ? <>
                        <label>情报查询<input aria-label="情报查询" className={fieldClass} value={binding.query ?? ""} onChange={event => patchBinding({ query: event.target.value })} /></label>
                        <label>来源 ID（逗号分隔，可空）<input aria-label="情报来源 ID" className={fieldClass} value={binding.sourceIds?.join(",") ?? ""} onChange={event => patchBinding({ sourceIds: [...new Set(event.target.value.split(",").map(id => id.trim()).filter(Boolean))] })} /></label>
                    </> : null}
                    {binding.kind === "topic" || binding.kind === "monitor" ? <label>绑定对象 ID<input aria-label="绑定对象 ID" className={fieldClass} value={binding.refId ?? ""} onChange={event => patchBinding({ refId: event.target.value })} /></label> : null}
                    {binding.kind === "monitor" ? <p role="status">监控命中接口未接入，服务端会明确失败，不使用配置或预览冒充命中。</p> : null}
                    {binding.kind !== "static" ? <>
                        <label>查询时间窗口<select aria-label="查询时间窗口" className={fieldClass} value={binding.timeWindow?.kind ?? ""} onChange={event => patchBinding({ timeWindow: event.target.value ? { kind: event.target.value as "7d" | "30d" | "custom", timezone: binding.timeWindow?.timezone || "UTC" } : undefined })}>
                            <option value="">不指定窗口</option><option value="7d">最近 7 天</option><option value="30d">最近 30 天</option><option value="custom">自定义</option>
                        </select></label>
                        {binding.timeWindow ? <label>时区<input aria-label="查询时区" className={fieldClass} value={binding.timeWindow.timezone} onChange={event => patchBinding({ timeWindow: { ...binding.timeWindow!, timezone: event.target.value } })} /></label> : null}
                        {binding.timeWindow?.kind === "custom" ? <div className="grid grid-cols-2 gap-1">{(["from", "to"] as const).map(key => <label key={key}>{key === "from" ? "起始时间" : "截止时间"}<input aria-label={`查询${key}`} className={fieldClass} placeholder="ISO 日期或 RFC3339" value={binding.timeWindow?.[key] ?? ""} onChange={event => patchBinding({ timeWindow: { ...binding.timeWindow!, [key]: event.target.value } })} /></label>)}</div> : null}
                    </> : null}
                    {binding.kind === "mcp_tool" ? <>
                        <label>已批准 MCP 工具<select aria-label="已批准 MCP 工具" className={fieldClass} value={selectedTool?.key ?? ""} disabled={mcpLibrary.loading} onChange={event => {
                            const chosen = approvedTools.find(item => item.key === event.target.value);
                            if (!chosen) return;
                            patchDataSource({ sourceBinding: { kind: "mcp_tool", refId: chosen.connection.id, toolName: chosen.name, toolVersion: chosen.connection.version, schemaHash: chosen.tool.hash, toolArgs: {} } });
                        }}><option value="">选择运行栏工作区后读取已批准工具</option>{approvedTools.map(item => <option key={item.key} value={item.key}>{item.connection.name} / {item.name} · v{item.connection.version}</option>)}</select></label>
                        {binding.toolName ? <p>固定版本 {binding.toolVersion} · Schema hash {binding.schemaHash}</p> : null}
                        {selectedTool && (selectedTool.connection.version !== binding.toolVersion || selectedTool.tool.hash !== binding.schemaHash) ? <p role="alert">已批准版本发生变化，请重新选择工具并重新审批。</p> : null}
                        {selectedTool ? <details><summary>输入 Schema（已批准）</summary><pre className="whitespace-pre-wrap break-all select-text">{JSON.stringify(selectedTool.tool.inputSchema, null, 2)}</pre></details> : null}
                        <label>MCP 参数 JSON<textarea aria-label="MCP 参数 JSON" className={fieldClass} key={`args-${binding.refId}-${binding.toolName}`} defaultValue={JSON.stringify(binding.toolArgs ?? {}, null, 2)} onBlur={event => jsonField("toolArgs", event.target.value)} /></label>
                        <label>预期输出 Schema（用户合同，随本次审批冻结）<textarea aria-label="MCP 输出 JSON Schema" className={fieldClass} key={`schema-${binding.refId}-${binding.toolName}`} defaultValue={binding.outputSchema ? JSON.stringify(binding.outputSchema, null, 2) : ""} placeholder='{"type":"array","items":{"type":"object"}}' onBlur={event => jsonField("outputSchema", event.target.value)} /></label>
                        {binding.timeWindow ? <div className="grid grid-cols-2 gap-1">{(["from", "to"] as const).map(key => <label key={key}>窗口{key}参数名<input aria-label={`MCP 窗口${key}参数`} className={fieldClass} value={binding.windowArguments?.[key] ?? ""} onChange={event => patchBinding({ windowArguments: { from: binding.windowArguments?.from ?? "", to: binding.windowArguments?.to ?? "", [key]: event.target.value } })} /></label>)}</div> : null}
                        <p>参数与 Schema 在失焦时校验保存；远端引用禁止。运行仍需真实审批。</p>
                        {mcpLibrary.error ? <p role="alert">{mcpLibrary.error}</p> : null}
                    </> : null}
                    {bindingError ? <p role="alert">{bindingError}；未保存无效配置</p> : null}
                </div>
            </details>
            <p className="mt-1 text-[var(--fs-tiny)]" role="status">{binding.kind === "static" ? "本地静态预览，非服务端运行结果" : "本地缓存预览，非最新服务端结果；刷新绑定须在顶栏申请受管运行"}</p>

            {/* 错误提示 */}
            {importError || result.error ? (
                <div
                    className="mt-2 flex items-center gap-1.5 rounded-[var(--r-sm)] px-2.5 py-1.5 text-[var(--fs-tiny)]"
                    style={{ background: "color-mix(in srgb, var(--status-error, #ef4444) 14%, transparent)", color: "var(--status-error, #ef4444)" }}
                >
                    <AlertCircle className="size-3.5 shrink-0" />
                    <span className="truncate">{importError || result.error}</span>
                </div>
            ) : null}

            {/* 主体区：源码编辑 或 结构化预览 */}
            <div className="mt-2 min-h-0 flex-1 overflow-hidden rounded-[var(--r-md)] border" style={{ background: theme.node.panel, borderColor: theme.node.edge }}>
                {viewMode === "edit" ? (
                    <textarea
                        aria-label="数据源内容"
                        className="thin-scrollbar h-full w-full resize-none border-none bg-transparent p-2.5 font-mono text-xs leading-5 outline-none select-text"
                        style={{ color: theme.node.text }}
                        placeholder={FORMAT_PLACEHOLDERS[format]}
                        value={rawText}
                        onChange={(event) => patchDataSource({ rawText: event.target.value }, event.target.value)}
                    />
                ) : (
                    <div className="thin-scrollbar flex h-full w-full flex-col gap-1.5 overflow-y-auto p-2">
                        {result.outputItems.length ? (
                            result.outputItems.slice(0, 50).map((item, idx) => (
                                <div
                                    key={idx}
                                    className="flex items-start gap-2 rounded-[var(--r-sm)] px-2 py-1.5 font-mono text-[var(--fs-tiny)]"
                                    style={{ background: theme.node.fill, border: `1px solid ${theme.node.edge}` }}
                                >
                                    <span className="shrink-0 font-semibold opacity-50">#{idx + 1}</span>
                                    <span className="min-w-0 flex-1 break-all select-text">{formatPreviewItem(item)}</span>
                                </div>
                            ))
                        ) : (
                            <div className="flex h-full items-center justify-center text-center text-[var(--fs-label)]" style={{ color: theme.node.muted }}>
                                暂无解析出的条目
                            </div>
                        )}
                    </div>
                )}
            </div>

            {/* 底栏：条数统计与字段摘要 */}
            <div className="mt-2 flex items-center justify-between gap-2 text-[var(--fs-tiny)]" style={{ color: theme.node.muted }}>
                <div className="flex min-w-0 items-center gap-1.5">
                    <span
                        className="shrink-0 rounded-[var(--r-sm)] px-1.5 py-0.5 font-semibold tabular-nums"
                        style={{ background: `${theme.accent.primary}1f`, color: theme.accent.primary }}
                    >
                        {result.outputItems.length} 条记录
                    </span>
                    {result.detectedKeys.length ? (
                        <span className="truncate" title={`字段: ${result.detectedKeys.join(", ")}`}>
                            字段: {result.detectedKeys.slice(0, 5).join(", ")}
                        </span>
                    ) : null}
                </div>
                {ds?.sourceBinding?.kind && ds.sourceBinding.kind !== "static" ? (
                    <span className="max-w-[140px] truncate rounded px-1.5 py-0.5 font-mono opacity-85" style={{ background: theme.node.panel }} title={`情报源绑定: ${ds.sourceBinding.kind} ${ds.sourceBinding.refId || ds.sourceBinding.query || ""}`}>
                        绑定: {ds.sourceBinding.kind}
                    </span>
                ) : fileName ? (
                    <span className="max-w-[120px] truncate opacity-75" title={fileName}>
                        {fileName}
                    </span>
                ) : null}
            </div>
        </div>
    );
}

const FALLBACK_DATA_SOURCE_THEME = {
    node: { fill: "#181818", panel: "#141414", stroke: "rgba(255,255,255,.12)", edge: "rgba(255,255,255,.18)", text: "#ededed", muted: "#a3a3a3" },
    accent: { primary: "#171717", onPrimary: "#ffffff" },
} as unknown as CanvasTheme;

export function DataSourceNode({
    node,
    theme = FALLBACK_DATA_SOURCE_THEME,
}: {
    node: CanvasNodeData;
    theme?: CanvasTheme;
    isSelected?: boolean;
    onUpdateNode?: unknown;
    onDelete?: unknown;
}) {
    return <DataSourceNodeContent node={node} theme={theme} />;
}
