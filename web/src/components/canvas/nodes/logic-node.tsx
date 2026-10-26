import { AlertTriangle, CheckCircle2, Filter, GitBranch, Layers, Sparkles, XCircle } from "lucide-react";
import { useState } from "react";

import { useCanvasNodeActions } from "@/components/canvas/canvas-node-action-context";
import { useDataflowNodeResult } from "@/components/canvas/canvas-node-graph-context";
import { NodeRunSnapshotBar } from "@/components/canvas/nodes/node-run-snapshot-bar";
import type { CanvasTheme } from "@/lib/canvas-theme";
import {
    formatDataItemsForPrompt,
    LOGIC_COMPARATOR_LABELS,
    readNodeLogicConfig,
    type CanvasDataItem,
} from "@/lib/canvas/dataflow-evaluator";
import type {
    CanvasLogicAggregateFn,
    CanvasLogicComparator,
    CanvasLogicConfig,
    CanvasLogicJoinMode,
    CanvasLogicOperator,
    CanvasLogicSortOrder,
    CanvasNodeData,
} from "@/types/canvas";

type LogicNodeContentProps = {
    node: CanvasNodeData;
    theme: CanvasTheme;
};

const OPERATOR_TABS: Array<{ value: CanvasLogicOperator; shortLabel: string; title: string }> = [
    { value: "if_else", shortLabel: "If / Else", title: "按条件分流到 True / False 两个出口" },
    { value: "filter", shortLabel: "过滤", title: "仅保留满足条件的条目" },
    { value: "compute", shortLabel: "计算", title: "按表达式计算衍生指标（如千人消费量折算）" },
    { value: "aggregate", shortLabel: "聚合", title: "按维度分组统计 count / sum / avg / max / min / p95" },
    { value: "join", shortLabel: "关联 Join", title: "按关联键拼接端口 A 与端口 B 的多源记录" },
    { value: "sort_limit", shortLabel: "Top-N", title: "按指定字段排序并截取前 N 条高风险目标" },
    { value: "intersect", shortLabel: "交集 ∩", title: "取输入 A 与输入 B 的共有项 (A ∩ B)" },
    { value: "union", shortLabel: "并集 ∪", title: "合并输入 A 与输入 B 并去重 (A ∪ B)" },
    { value: "difference", shortLabel: "差集 −", title: "保留输入 A 中不在输入 B 里的项 (A − B)" },
    { value: "json_extract", shortLabel: "提取", title: "按字段路径提取属性值列表" },
];

const COMPARATOR_OPTIONS = Object.entries(LOGIC_COMPARATOR_LABELS) as Array<[CanvasLogicComparator, string]>;

function formatCompactItem(item: CanvasDataItem): string {
    if (typeof item === "string" || typeof item === "number" || typeof item === "boolean") {
        return String(item);
    }
    return JSON.stringify(item);
}

export function LogicNodeContent({ node, theme }: LogicNodeContentProps) {
    const { updateNode, updateMetadata } = useCanvasNodeActions();
    const result = useDataflowNodeResult(node);
    const config = readNodeLogicConfig(node);
    const [previewBranch, setPreviewBranch] = useState<"true" | "false">("true");

    const {
        operator,
        fieldPath = "",
        comparator = "contains",
        compareValue = "",
        matchKey = "",
        expression = "",
        outputField = "",
        groupBy = [],
        metrics = [],
        joinMode = "inner",
        joinKeys = [],
        dedupeKeys = [],
        sortField = "",
        sortOrder = "desc",
        limit = 10,
    } = config;
    const fieldList = (value: string) => value === "" ? [] : value.split(",").map(key => key.trim());
    const inputClass = "h-6 min-w-0 rounded-[var(--r-sm)] border px-2 text-[var(--fs-tiny)] outline-none";
    const inputStyle = { background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text };
    const isSetOp = operator === "intersect" || operator === "union" || operator === "difference";
    const isConditionOp = operator === "if_else" || operator === "filter";
    const hideCompareValue = comparator === "is_empty" || comparator === "not_empty";

    const patchLogic = (patch: Partial<CanvasLogicConfig>) => {
        const nextLogic: CanvasLogicConfig = {
            ...config,
            ...patch,
        };
        const previewText = formatDataItemsForPrompt(result.outputItems);
        if (updateNode) {
            updateNode(node.id, (current) => ({
                ...current,
                metadata: {
                    ...current.metadata,
                    content: previewText,
                    logic: {
                        ...readNodeLogicConfig(current),
                        ...patch,
                    },
                },
            }));
            return;
        }
        updateMetadata?.(node.id, {
            content: previewText,
            logic: nextLogic,
        });
    };

    const stringArray = (value: unknown): boolean => Array.isArray(value) && value.every(item => typeof item === "string");
    const invalidShape = !stringArray(groupBy) || !stringArray(dedupeKeys)
        || !Array.isArray(metrics) || metrics.some(metric => !metric || typeof metric.field !== "string" || typeof metric.fn !== "string" || typeof metric.as !== "string")
        || !Array.isArray(joinKeys) || joinKeys.some(pair => !pair || typeof pair.left !== "string" || typeof pair.right !== "string");
    if (invalidShape) return (
        <div data-canvas-no-zoom data-canvas-wheel-scroll className="flex h-full flex-col gap-2 overflow-auto p-3" style={{ background: theme.node.fill, color: theme.node.text }} onPointerDown={event => event.stopPropagation()}>
            <div role="alert">配置格式错误：分组、指标、关联键和去重字段必须使用当前数组合同。请重置后重新编辑。</div>
            <button type="button" aria-label="重置非法算子配置" onClick={() => patchLogic({ groupBy: [], metrics: [{ field: "", fn: "count", as: "count" }], joinKeys: [{ left: "id", right: "id" }], dedupeKeys: [] })}>重置非法算子配置</button>
        </div>
    );

    const activePreviewItems =
        operator === "if_else"
            ? previewBranch === "false"
                ? result.falseItems
                : result.trueItems
            : result.outputItems;

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
            {/* 顶栏：算子切换 */}
            <div className="flex flex-wrap items-center gap-1 pb-2" style={{ borderBottom: `1px solid ${theme.node.edge}` }} role="tablist" aria-label="逻辑算子类型">
                <GitBranch className="mr-0.5 size-3.5 shrink-0" style={{ color: theme.accent.primary }} />
                {OPERATOR_TABS.map((tab) => {
                    const active = operator === tab.value;
                    return (
                        <button
                            key={tab.value}
                            type="button"
                            aria-pressed={active}
                            title={tab.title}
                            className="rounded-[var(--r-sm)] px-1.5 py-0.5 text-[var(--fs-tiny)] font-medium transition-colors"
                            style={{
                                background: active ? theme.accent.primary : theme.node.panel,
                                color: active ? theme.accent.onPrimary : theme.node.muted,
                            }}
                            onClick={() => patchLogic({ operator: tab.value })}
                        >
                            {tab.shortLabel}
                        </button>
                    );
                })}
            </div>

            <NodeRunSnapshotBar
                nodeId={node.id}
                snapshot={node.metadata?.lastRunSnapshot}
                derived={node.metadata?.derived}
            />

            {/* 环路或错误提示 */}
            {result.error ? (
                <div
                    className="mt-2 flex items-center gap-1.5 rounded-[var(--r-sm)] px-2.5 py-1.5 text-[var(--fs-tiny)]"
                    style={{ background: "color-mix(in srgb, var(--status-error, #ef4444) 14%, transparent)", color: "var(--status-error, #ef4444)" }}
                >
                    <AlertTriangle className="size-3.5 shrink-0" />
                    <span className="truncate">{result.error}</span>
                </div>
            ) : null}

            {/* 规则表单区 */}
            <div className="mt-2 flex flex-col gap-1.5 rounded-[var(--r-md)] border p-2" style={{ background: theme.node.panel, borderColor: theme.node.edge }}>
                {isSetOp ? (
                    <>
                        <div className="flex items-center justify-between gap-2 text-[var(--fs-tiny)]">
                            <span className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-medium" style={{ background: theme.node.fill, color: theme.node.text }}>
                                <Layers className="size-3" style={{ color: theme.accent.primary }} />
                                端口 A: {result.inputA.length} 条
                            </span>
                            <span className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 font-medium" style={{ background: theme.node.fill, color: theme.node.text }}>
                                <Layers className="size-3" style={{ color: theme.accent.primary }} />
                                端口 B: {result.inputB.length} 条
                            </span>
                        </div>
                        <div className="flex items-center gap-1.5">
                            <label className="shrink-0 text-[var(--fs-tiny)]" style={{ color: theme.node.muted }}>
                                匹配键
                            </label>
                            <input
                                type="text"
                                aria-label="集合匹配键"
                                placeholder="留空按整行比对，或填字段如 id / title"
                                value={matchKey}
                                onChange={(event) => patchLogic({ matchKey: event.target.value })}
                                className="h-6 min-w-0 flex-1 rounded-[var(--r-sm)] border px-2 font-mono text-[var(--fs-tiny)] outline-none"
                                style={{ background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text }}
                            />
                        </div>
                    </>
                ) : null}

                {isConditionOp ? (
                    <div className="grid grid-cols-12 gap-1.5">
                        <input
                            type="text"
                            aria-label="条件字段路径"
                            placeholder="字段 (如 score 或留空)"
                            value={fieldPath}
                            onChange={(event) => patchLogic({ fieldPath: event.target.value })}
                            className="col-span-4 h-6 rounded-[var(--r-sm)] border px-2 font-mono text-[var(--fs-tiny)] outline-none"
                            style={{ background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text }}
                        />
                        <select
                            aria-label="条件比较符"
                            value={comparator}
                            onChange={(event) => patchLogic({ comparator: event.target.value as CanvasLogicComparator })}
                            className={`${hideCompareValue ? "col-span-8" : "col-span-4"} h-6 rounded-[var(--r-sm)] border px-1.5 text-[var(--fs-tiny)] outline-none`}
                            style={{ background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text }}
                        >
                            {COMPARATOR_OPTIONS.map(([val, label]) => (
                                <option key={val} value={val}>
                                    {label}
                                </option>
                            ))}
                        </select>
                        {!hideCompareValue ? (
                            <input
                                type="text"
                                aria-label="条件比较值"
                                placeholder="比较值"
                                value={compareValue}
                                onChange={(event) => patchLogic({ compareValue: event.target.value })}
                                className="col-span-4 h-6 rounded-[var(--r-sm)] border px-2 font-mono text-[var(--fs-tiny)] outline-none"
                                style={{ background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text }}
                            />
                        ) : null}
                    </div>
                ) : null}

                {operator === "json_extract" ? (
                    <div className="flex items-center gap-1.5">
                        <label className="shrink-0 text-[var(--fs-tiny)]" style={{ color: theme.node.muted }}>
                            提取路径
                        </label>
                        <input
                            type="text"
                            aria-label="提取字段路径"
                            placeholder="字段路径，如 title 或 meta.author"
                            value={fieldPath}
                            onChange={(event) => patchLogic({ fieldPath: event.target.value })}
                            className="h-6 min-w-0 flex-1 rounded-[var(--r-sm)] border px-2 font-mono text-[var(--fs-tiny)] outline-none"
                            style={{ background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text }}
                        />
                    </div>
                ) : null}

                {operator === "compute" ? (
                    <div className="grid grid-cols-12 gap-1.5">
                        <input
                            type="text"
                            aria-label="计算表达式"
                            placeholder="公式 (如 wbe_consumption_mg_1000p 或 a * b / 1000)"
                            value={expression}
                            onChange={(event) => patchLogic({ expression: event.target.value })}
                            className="col-span-8 h-6 rounded-[var(--r-sm)] border px-2 font-mono text-[var(--fs-tiny)] outline-none"
                            style={{ background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text }}
                        />
                        <input
                            type="text"
                            aria-label="计算输出字段"
                            placeholder="输出字段 (如 consumption_mg_1000p)"
                            value={outputField}
                            onChange={(event) => patchLogic({ outputField: event.target.value })}
                            className="col-span-4 h-6 rounded-[var(--r-sm)] border px-2 font-mono text-[var(--fs-tiny)] outline-none"
                            style={{ background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text }}
                        />
                    </div>
                ) : null}

                {operator === "aggregate" ? (
                    <div className="flex flex-col gap-1.5">
                        <input aria-label="聚合分组字段" placeholder="多分组字段，逗号分隔；留空全局" value={groupBy.join(",")} onChange={event => patchLogic({ groupBy: fieldList(event.target.value) })} className={inputClass} style={inputStyle} />
                        {metrics.map((metric, index) => (
                            <div key={index} className="grid grid-cols-4 gap-1">
                                <input aria-label={`聚合目标字段 ${index + 1}`} placeholder="字段；count可空" value={metric.field} onChange={event => patchLogic({ metrics: metrics.map((m, i) => i === index ? { ...m, field: event.target.value } : m) })} className={inputClass} style={inputStyle} />
                                <select aria-label={`聚合函数 ${index + 1}`} value={metric.fn} onChange={event => patchLogic({ metrics: metrics.map((m, i) => i === index ? { ...m, fn: event.target.value as CanvasLogicAggregateFn } : m) })} className={inputClass} style={inputStyle}>
                                    {["count", "sum", "avg", "max", "min", "p95"].map(fn => <option key={fn} value={fn}>{fn}</option>)}
                                </select>
                                <input aria-label={`聚合别名 ${index + 1}`} placeholder="输出别名" value={metric.as} onChange={event => patchLogic({ metrics: metrics.map((m, i) => i === index ? { ...m, as: event.target.value } : m) })} className={inputClass} style={inputStyle} />
                                <button type="button" aria-label={`删除聚合指标 ${index + 1}`} onClick={() => patchLogic({ metrics: metrics.filter((_, i) => i !== index) })}>删除</button>
                            </div>
                        ))}
                        <button type="button" aria-label="添加聚合指标" onClick={() => patchLogic({ metrics: [...metrics, { field: "", fn: "count", as: `metric_${metrics.length + 1}` }] })}>添加指标</button>
                    </div>
                ) : null}

                {operator === "join" ? (
                    <div className="flex flex-col gap-1.5">
                        {joinKeys.map((pair, index) => (
                            <div key={index} className="grid grid-cols-3 gap-1">
                                <input aria-label={`关联左键 ${index + 1}`} placeholder="左字段" value={pair.left} onChange={event => patchLogic({ joinKeys: joinKeys.map((p, i) => i === index ? { ...p, left: event.target.value } : p) })} className={inputClass} style={inputStyle} />
                                <input aria-label={`关联右键 ${index + 1}`} placeholder="右字段" value={pair.right} onChange={event => patchLogic({ joinKeys: joinKeys.map((p, i) => i === index ? { ...p, right: event.target.value } : p) })} className={inputClass} style={inputStyle} />
                                <button type="button" aria-label={`删除关联键 ${index + 1}`} onClick={() => patchLogic({ joinKeys: joinKeys.filter((_, i) => i !== index) })}>删除</button>
                            </div>
                        ))}
                        <button type="button" aria-label="添加关联键" onClick={() => patchLogic({ joinKeys: [...joinKeys, { left: "", right: "" }] })}>添加关联键</button>
                        <select aria-label="关联模式" value={joinMode} onChange={event => patchLogic({ joinMode: event.target.value as CanvasLogicJoinMode })} className={inputClass} style={inputStyle}>
                            <option value="inner">Inner Join</option><option value="left">Left Join</option>
                        </select>
                    </div>
                ) : null}

                {operator === "sort_limit" ? (
                    <div className="grid grid-cols-12 gap-1.5">
                        <input
                            type="text"
                            aria-label="排序字段"
                            placeholder="排序字段 (如 value)"
                            value={sortField || fieldPath}
                            onChange={(event) => patchLogic({ sortField: event.target.value, fieldPath: event.target.value })}
                            className="col-span-5 h-6 rounded-[var(--r-sm)] border px-2 font-mono text-[var(--fs-tiny)] outline-none"
                            style={{ background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text }}
                        />
                        <select
                            aria-label="排序方向"
                            value={sortOrder}
                            onChange={(event) => patchLogic({ sortOrder: event.target.value as CanvasLogicSortOrder })}
                            className="col-span-4 h-6 rounded-[var(--r-sm)] border px-1.5 text-[var(--fs-tiny)] outline-none"
                            style={{ background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text }}
                        >
                            <option value="desc">降序 (Top)</option>
                            <option value="asc">升序</option>
                        </select>
                        <input
                            type="number"
                            min={1}
                            max={500}
                            aria-label="截取条数"
                            placeholder="Top N"
                            value={limit}
                            onChange={(event) => patchLogic({ limit: Number(event.target.value) })}
                            className="col-span-3 h-6 rounded-[var(--r-sm)] border px-1.5 font-mono text-[var(--fs-tiny)] outline-none"
                            style={{ background: theme.node.fill, borderColor: theme.node.edge, color: theme.node.text }}
                        />
                    </div>
                ) : null}

                {operator === "sort_limit" ? <input aria-label="去重字段" placeholder="排序后去重字段，逗号分隔；留空不去重" value={dedupeKeys.join(",")} onChange={event => patchLogic({ dedupeKeys: fieldList(event.target.value) })} className={inputClass} style={inputStyle} /> : null}

                {/* 上游检测到的字段快捷点选 */}
                {result.detectedKeys.length > 0 ? (
                    <div className="flex flex-wrap items-center gap-1 pt-0.5">
                        <span className="text-[10px] opacity-55">可用字段:</span>
                        {result.detectedKeys.slice(0, 6).map((key) => {
                            const selectedKey = isSetOp ? matchKey === key : operator === "join" ? joinKeys.some(pair => pair.left === key) : operator === "aggregate" ? groupBy.includes(key) : fieldPath === key;
                            return (
                                <button
                                    key={key}
                                    type="button"
                                    className="rounded px-1.5 py-0.2 font-mono text-[10px] transition-opacity hover:opacity-100"
                                    style={{
                                        background: selectedKey ? `${theme.accent.primary}28` : theme.node.fill,
                                        color: selectedKey ? theme.accent.primary : theme.node.muted,
                                        border: `1px solid ${selectedKey ? theme.accent.primary : theme.node.edge}`,
                                    }}
                                    onClick={() => isSetOp ? patchLogic({ matchKey: key }) : operator === "join" ? patchLogic({ joinKeys: [...joinKeys, { left: key, right: key }] }) : operator === "aggregate" ? patchLogic({ groupBy: groupBy.includes(key) ? groupBy.filter(field => field !== key) : [...groupBy, key] }) : patchLogic({ fieldPath: key })}
                                >
                                    {key}
                                </button>
                            );
                        })}
                    </div>
                ) : null}
            </div>

            {/* 输出预览与分支状态栏 */}
            <div className="mt-2 flex items-center justify-between gap-1.5 text-[var(--fs-tiny)]">
                {operator === "if_else" ? (
                    <div className="flex w-full items-center justify-between gap-1.5">
                        <button
                            type="button"
                            aria-pressed={previewBranch === "true"}
                            onClick={() => setPreviewBranch("true")}
                            className="inline-flex flex-1 items-center justify-between rounded-[var(--r-sm)] border px-2 py-1 font-medium transition-colors"
                            style={{
                                background: result.trueItems.length > 0 ? "rgba(34, 197, 94, 0.14)" : theme.node.panel,
                                borderColor: previewBranch === "true" ? "#22c55e" : theme.node.edge,
                                color: result.trueItems.length > 0 ? "#22c55e" : theme.node.muted,
                            }}
                        >
                            <span className="inline-flex items-center gap-1">
                                <CheckCircle2 className="size-3" />
                                True 出口
                            </span>
                            <span className="font-mono font-semibold">{result.trueItems.length} 条</span>
                        </button>
                        <button
                            type="button"
                            aria-pressed={previewBranch === "false"}
                            onClick={() => setPreviewBranch("false")}
                            className="inline-flex flex-1 items-center justify-between rounded-[var(--r-sm)] border px-2 py-1 font-medium transition-colors"
                            style={{
                                background: result.falseItems.length > 0 ? "rgba(245, 158, 11, 0.14)" : theme.node.panel,
                                borderColor: previewBranch === "false" ? "#f59e0b" : theme.node.edge,
                                color: result.falseItems.length > 0 ? "#f59e0b" : theme.node.muted,
                            }}
                        >
                            <span className="inline-flex items-center gap-1">
                                <XCircle className="size-3" />
                                False 出口
                            </span>
                            <span className="font-mono font-semibold">{result.falseItems.length} 条</span>
                        </button>
                    </div>
                ) : (
                    <div className="flex w-full items-center justify-between">
                        <span className="inline-flex items-center gap-1 font-medium" style={{ color: theme.node.muted }}>
                            <Filter className="size-3" />
                            输入 {result.inputA.length + result.inputB.length} 条
                        </span>
                        <span
                            className="inline-flex items-center gap-1 rounded-[var(--r-sm)] px-1.5 py-0.5 font-semibold tabular-nums"
                            style={{ background: `${theme.accent.primary}1f`, color: theme.accent.primary }}
                        >
                            <Sparkles className="size-3" />
                            输出 {result.outputItems.length} 条
                        </span>
                    </div>
                )}
            </div>

            {/* 实时求值结果列表 */}
            <div
                className="thin-scrollbar mt-1.5 min-h-0 flex-1 overflow-y-auto rounded-[var(--r-md)] border p-1.5"
                style={{ background: theme.node.panel, borderColor: theme.node.edge }}
            >
                {activePreviewItems.length > 0 ? (
                    <div className="flex flex-col gap-1">
                        {activePreviewItems.slice(0, 30).map((item, idx) => (
                            <div
                                key={idx}
                                className="flex items-start gap-1.5 rounded-[var(--r-sm)] px-2 py-1 font-mono text-[var(--fs-tiny)]"
                                style={{ background: theme.node.fill, border: `1px solid ${theme.node.edge}` }}
                            >
                                <span className="shrink-0 opacity-45">#{idx + 1}</span>
                                <span className="min-w-0 flex-1 break-all select-text">{formatCompactItem(item)}</span>
                            </div>
                        ))}
                    </div>
                ) : (
                    <div className="flex h-full items-center justify-center text-center text-[var(--fs-tiny)]" style={{ color: theme.node.muted }}>
                        {result.inputA.length + result.inputB.length === 0
                            ? isSetOp
                                ? "请连接左侧端口 A 与端口 B 的上游数据源"
                                : "请连接左侧上游数据源或文本节点"
                            : operator === "if_else"
                              ? `${previewBranch === "true" ? "True" : "False"} 分支当前无命中数据（下游将自动跳过）`
                              : "当前条件无匹配结果"}
                    </div>
                )}
            </div>
        </div>
    );
}

const FALLBACK_LOGIC_THEME = {
    node: { fill: "#181818", panel: "#141414", stroke: "rgba(255,255,255,.12)", edge: "rgba(255,255,255,.18)", text: "#ededed", muted: "#a3a3a3" },
    accent: { primary: "#171717", onPrimary: "#ffffff" },
} as unknown as CanvasTheme;

export function LogicNode({
    node,
    theme = FALLBACK_LOGIC_THEME,
}: {
    node: CanvasNodeData;
    theme?: CanvasTheme;
    isSelected?: boolean;
    onUpdateNode?: unknown;
    onDelete?: unknown;
}) {
    return <LogicNodeContent node={node} theme={theme} />;
}
