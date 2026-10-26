import { describe, expect, test } from "bun:test";
import React, { isValidElement, type ReactElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { ChartNode } from "@/components/canvas/nodes/chart-node";
import { DataSourceNode } from "@/components/canvas/nodes/data-source-node";
import { LogicNode } from "@/components/canvas/nodes/logic-node";
import { MarkdownNode } from "@/components/canvas/nodes/markdown-node";
import { CanvasNodeType, type CanvasNodeData } from "@/types/canvas";

type TestElement = {
    textContent: string;
    props: Record<string, unknown>;
};

const internals = (
    React as unknown as {
        __CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE: {
            H: Record<string, unknown>;
        };
    }
).__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE;

let activeRender: (() => void) | null = null;

function render(ui: ReactElement) {
    const states: unknown[] = [];
    const byTestId = new Map<string, TestElement>();

    function walk(node: ReactNode): string {
        if (node == null || typeof node === "boolean") return "";
        if (typeof node === "string" || typeof node === "number") return String(node);
        if (Array.isArray(node)) return node.map((child) => walk(child)).join("");
        if (!isValidElement(node)) return "";
        const element = node as ReactElement<Record<string, unknown>>;
        const type = element.type as unknown;
        try {
            if (typeof type === "function") {
                return walk((type as (props: Record<string, unknown>) => ReactNode)(element.props));
            }
            if (type && typeof type === "object" && "type" in type && typeof (type as { type: unknown }).type === "function") {
                return walk(((type as { type: (props: Record<string, unknown>) => ReactNode }).type)(element.props));
            }
        } catch {
            return "";
        }
        const textContent = walk(element.props?.children as ReactNode);
        const testId = element.props?.["data-testid"];
        if (typeof testId === "string") {
            byTestId.set(testId, { textContent, props: element.props });
        }
        return textContent;
    }

    function evaluate() {
        byTestId.clear();
        let stateCursor = 0;
        function Probe() {
            const origH = internals.H;
            internals.H = new Proxy(origH, {
                get(target, prop, receiver) {
                    if (prop === "useState") {
                        return (initial: unknown) => {
                            const index = stateCursor++;
                            if (!(index in states)) {
                                states[index] = typeof initial === "function" ? (initial as () => unknown)() : initial;
                            }
                            const setState = (next: unknown) => {
                                states[index] =
                                    typeof next === "function"
                                        ? (next as (prev: unknown) => unknown)(states[index])
                                        : next;
                            };
                            return [states[index], setState];
                        };
                    }
                    return Reflect.get(target, prop, receiver);
                },
            });
            try {
                walk(ui);
            } finally {
                internals.H = origH;
            }
            return null;
        }
        renderToStaticMarkup(<Probe />);
    }

    activeRender = evaluate;
    evaluate();

    return {
        queryByTestId(id: string): TestElement | null {
            return byTestId.get(id) ?? null;
        },
        getByTestId(id: string): TestElement {
            const found = byTestId.get(id);
            if (!found) {
                throw new Error(`Unable to find an element by: [data-testid="${id}"]`);
            }
            return found;
        },
    };
}

const fireEvent = {
    click(element: TestElement) {
        const onClick = element.props.onClick;
        if (typeof onClick === "function") {
            onClick({ stopPropagation() {}, preventDefault() {} });
        }
        activeRender?.();
    },
};

function makeNode(type: CanvasNodeType, overrides: Partial<CanvasNodeData> = {}): CanvasNodeData {
    return {
        id: `node-${type}`,
        type,
        title: `测试-${type}`,
        position: { x: 0, y: 0 },
        width: 320,
        height: 240,
        status: "idle",
        metadata: {
            lastRunSnapshot: {
                runId: "run_101",
                executedAt: "2026-10-26T08:30:00Z",
                durationMs: 42,
                inputCount: 12,
                outputCount: 5,
                status: "success",
                sampleItems: [{ region_code: "CN-GD", consumption: 18.4 }],
            },
            derived: {
                runId: "run_101",
                canvasRevision: 7,
                status: "succeeded",
            },
        },
        ...overrides,
    };
}

const noop = () => {};

describe("Canvas node lastRunSnapshot bar", () => {
    test("renders status badge, durationMs, input->output counts and expandable sampleItems across all 4 node types", () => {
        const dsNode = makeNode(CanvasNodeType.DataSource);
        const logicNode = makeNode(CanvasNodeType.Logic);
        const chartNode = makeNode(CanvasNodeType.Chart);
        const mdNode = makeNode(CanvasNodeType.Markdown, {
            metadata: {
                ...makeNode(CanvasNodeType.Markdown).metadata,
                content: "# 研判结论",
            },
        });

        const { getByTestId, queryByTestId } = render(
            <div>
                <DataSourceNode node={dsNode} isSelected={false} onUpdateNode={noop} onDelete={noop} />
                <LogicNode node={logicNode} isSelected={false} onUpdateNode={noop} onDelete={noop} />
                <ChartNode node={chartNode} isSelected={false} onUpdateNode={noop} onDelete={noop} />
                <MarkdownNode node={mdNode} isSelected={false} onUpdateNode={noop} onDelete={noop} />
            </div>,
        );

        for (const type of [CanvasNodeType.DataSource, CanvasNodeType.Logic, CanvasNodeType.Chart, CanvasNodeType.Markdown]) {
            const bar = getByTestId(`node-run-snapshot-node-${type}`);
            expect(bar.textContent).toContain("42ms");
            expect(bar.textContent).toContain("12 → 5");
            expect(bar.textContent).toContain("rev#7");
        }

        expect(queryByTestId("node-run-sample-node-logic")).toBeNull();
        const toggle = getByTestId("node-run-sample-toggle-node-logic");
        fireEvent.click(toggle);
        const sample = getByTestId("node-run-sample-node-logic");
        expect(sample.textContent).toContain("CN-GD");
    });

    test("renders error banner when lastRunSnapshot.status is error", () => {
        const failedNode = makeNode(CanvasNodeType.Logic, {
            metadata: {
                lastRunSnapshot: {
                    runId: "run_err",
                    executedAt: "2026-10-26T08:31:00Z",
                    durationMs: 15,
                    inputCount: 3,
                    outputCount: 0,
                    status: "error",
                    error: "periodComparison 缺少 baseline_period 字段",
                },
            },
        });

        const { getByTestId } = render(
            <LogicNode node={failedNode} isSelected={false} onUpdateNode={noop} onDelete={noop} />,
        );
        const bar = getByTestId("node-run-snapshot-node-logic");
        expect(bar.textContent).toContain("失败");
        expect(bar.textContent).toContain("periodComparison 缺少 baseline_period 字段");
    });
});
