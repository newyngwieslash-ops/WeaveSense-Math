import { createContext, useContext } from "react";

import {
    evaluateCanvasDataflow,
    formatDataItemsForPrompt,
    type CanvasDataflowEvaluation,
    type DataflowNodeResult,
} from "@/lib/canvas/dataflow-evaluator";
import type { CanvasConnection, CanvasNodeData } from "@/types/canvas";

// 扩展节点（对比/图表/调色/数据源/逻辑算子等）要读自己的上游或 DAG 求值结果才能工作，但节点经 CanvasProjectWorldLayers
// 渲染、CanvasNodeContentProps 里只有 node 本身，没有 nodes/connections。
// 与 canvas-node-action-context 同一个理由：通过 Context 注入，避免改动 world-layers 的透传链。
// 无 Provider 时静默降级为单节点自求值或「没有上游」，节点自行显示空状态而不是崩。
export type CanvasNodeGraphContextValue = {
    getUpstreamNodes?: (nodeId: string) => CanvasNodeData[];
    dataflow?: CanvasDataflowEvaluation;
    connections?: readonly CanvasConnection[];
};

export const CanvasNodeGraphContext = createContext<CanvasNodeGraphContextValue>({});

/** 取该节点的直接上游素材节点；无 Provider 或无上游时返回空数组。 */
export function useUpstreamNodes(nodeId: string) {
    const { getUpstreamNodes } = useContext(CanvasNodeGraphContext);
    return getUpstreamNodes?.(nodeId) ?? [];
}

/** 取该节点在当前画布 DAG 中的实时求值结果；无 Provider 时按单节点计算。 */
export function useDataflowNodeResult(node: CanvasNodeData): DataflowNodeResult {
    const { dataflow } = useContext(CanvasNodeGraphContext);
    const fromGraph = dataflow?.nodes.get(node.id);
    if (fromGraph) return fromGraph;
    return evaluateCanvasDataflow([node], []).nodes.get(node.id)!;
}

/** 连线是否存在与产出是否为空分开表达，避免空分支回退到节点的旧 content。 */
export function useUpstreamDataflowSource(nodeId: string): { connected: boolean; text: string; sources: string[]; error?: string } {
    const { getUpstreamNodes, dataflow, connections } = useContext(CanvasNodeGraphContext);
    if (connections) {
        const incoming = connections.filter((edge) => edge.toNodeId === nodeId && edge.relation !== "batch-output" && edge.relation !== "storyboard-output");
        const outputs = incoming.map((edge) => dataflow?.getEdgeItems(edge.fromNodeId, edge.fromHandleId).items ?? []);
        const error = incoming.map((edge) => dataflow?.nodes.get(edge.fromNodeId)?.error).find(Boolean);
        return { connected: incoming.length > 0, text: formatDataItemsForPrompt(outputs.flat()), sources: outputs.map(formatDataItemsForPrompt), error };
    }
    // 无连线合同的独立展示只读已经解析过端口的素材，不重新按 nodeId 读取 true/outputItems。
    const upstream = getUpstreamNodes?.(nodeId) ?? [];
    const sources = upstream.map((up) => up.metadata?.content || up.metadata?.dataSource?.rawText || up.metadata?.prompt || "");
    return { connected: upstream.length > 0, text: sources.filter((raw) => raw.trim()).join("\n"), sources };
}

export function useUpstreamDataflowText(nodeId: string): string {
    return useUpstreamDataflowSource(nodeId).text;
}
