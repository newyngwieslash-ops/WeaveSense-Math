import { memo, useState } from "react";
import type { CanvasNodeMetadata, CanvasNodeRunSnapshot } from "@/types/canvas";

interface NodeRunSnapshotBarProps {
    nodeId: string;
    snapshot?: CanvasNodeRunSnapshot;
    derived?: CanvasNodeMetadata["derived"];
}

const STATUS_META: Record<CanvasNodeRunSnapshot["status"], { label: string; badgeClass: string }> = {
    success: {
        label: "已执行",
        badgeClass: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300 border-emerald-500/30",
    },
    error: {
        label: "失败",
        badgeClass: "bg-red-500/15 text-red-700 dark:text-red-300 border-red-500/30",
    },
    skipped: {
        label: "跳过",
        badgeClass: "bg-amber-500/15 text-amber-700 dark:text-amber-300 border-amber-500/30",
    },
};

export const NodeRunSnapshotBar = memo(function NodeRunSnapshotBar({
    nodeId,
    snapshot,
    derived,
}: NodeRunSnapshotBarProps) {
    const [sampleOpen, setSampleOpen] = useState(false);

    if (!snapshot) {
        return null;
    }

    const meta = STATUS_META[snapshot.status] ?? STATUS_META.success;
    const sampleItems = Array.isArray(snapshot.sampleItems) ? snapshot.sampleItems.slice(0, 5) : [];

    return (
        <div
            data-testid={`node-run-snapshot-${nodeId}`}
            data-canvas-no-zoom
            className="px-3 py-1.5 border-b border-border/60 bg-muted/25 text-[11px] flex flex-col gap-1"
            onPointerDown={e => e.stopPropagation()}
        >
            <div className="flex items-center justify-between gap-2 flex-wrap">
                <div className="flex items-center gap-1.5 min-w-0">
                    <span className={`px-1.5 py-0.5 rounded border text-[10px] font-medium ${meta.badgeClass}`}>
                        {meta.label}
                    </span>
                    <span className="font-mono text-muted-foreground" title="输入条数 → 输出条数">
                        {snapshot.inputCount} → {snapshot.outputCount}
                    </span>
                    {typeof snapshot.durationMs === "number" && (
                        <span className="font-mono text-muted-foreground">
                            {snapshot.durationMs}ms
                        </span>
                    )}
                    {typeof derived?.canvasRevision === "number" && (
                        <span className="font-mono text-[10px] text-muted-foreground/80">
                            rev#{derived.canvasRevision}
                        </span>
                    )}
                </div>

                {sampleItems.length > 0 && (
                    <button
                        type="button"
                        data-testid={`node-run-sample-toggle-${nodeId}`}
                        aria-expanded={sampleOpen}
                        onClick={e => {
                            e.stopPropagation();
                            setSampleOpen(prev => !prev);
                        }}
                        className="text-[10px] text-primary hover:underline cursor-pointer shrink-0"
                    >
                        {sampleOpen ? "收起样本" : `样本(${sampleItems.length})`}
                    </button>
                )}
            </div>

            {snapshot.error && (
                <div className="text-[10px] text-red-600 dark:text-red-400 break-words font-mono bg-red-500/10 rounded px-1.5 py-1">
                    {snapshot.error}
                </div>
            )}

            {sampleOpen && sampleItems.length > 0 && (
                <pre
                    data-testid={`node-run-sample-${nodeId}`}
                    data-canvas-wheel-scroll
                    className="m-0 mt-0.5 max-h-28 overflow-auto rounded bg-background/80 p-1.5 text-[10px] font-mono text-foreground/90 border border-border/50"
                >
                    {JSON.stringify(sampleItems, null, 2)}
                </pre>
            )}
        </div>
    );
});
