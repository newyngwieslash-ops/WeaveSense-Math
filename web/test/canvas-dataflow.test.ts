import { describe, expect, it } from "bun:test";

import { buildNodeGenerationContext } from "@/components/canvas/canvas-node-generation";
import {
    evaluateCanvasDataflow,
    evaluateLogicNode,
    evaluateComputeOperation,
    evaluateNumericExpression,
    findBlockedLogicBranchInput,
    isLogicDualInput,
    isLogicDualOutput,
    logicNodeHandleAtY,
    logicNodeHandleY,
    parseDataSourceNode,
} from "@/lib/canvas/dataflow-evaluator";
import { CanvasNodeType, type CanvasConnection, type CanvasNodeData } from "@/types/canvas";

function makeNode(partial: Partial<CanvasNodeData> & { id: string; type: CanvasNodeData["type"] }): CanvasNodeData {
    return {
        title: partial.id,
        position: { x: 0, y: 0 },
        width: 400,
        height: 320,
        ...partial,
    };
}

describe("T03/T04 analytical contracts", () => {
    const run = (logic: unknown, a: any[] = [], b: any[] = []) => evaluateLogicNode(makeNode({ id: "ops", type: CanvasNodeType.Logic, metadata: { logic: logic as never } }), a, b);
    const metrics = ["count", "sum", "avg", "min", "max", "p95"].map((fn) => ({ field: fn === "count" ? "" : "v", fn, as: fn }));
    it("review count always counts rows even with a field", () => {
        expect(run({ operator: "aggregate", metrics: [{ field: "v", fn: "count", as: "n" }] }, [{ v: 0 }, { v: null }, {}]).outputItems).toEqual([{ n: 3 }]);
    });
    it("review join aliases cannot shadow readable nested left or right paths", () => {
        const result = run({ operator: "join", joinKeys: [{ left: "id", right: "id" }] }, [{ id: 1, v: 23, right: { v: 10 } }], [{ id: 1, v: 27.6 }]);
        expect(result.outputItems[0]).toMatchObject({ v: 23, right: { v: 10 }, "right.right.v": 27.6 });
        expect(run({ operator: "compute", expression: "right.v + right.right.v", outputField: "n" }, result.outputItems).outputItems[0]).toMatchObject({ n: 37.6 });
    });
    it("review sort uses Unicode codepoints for TopN and dedupe winner", () => {
        const rows = [{ id: 1, name: "\uE000" }, { id: 1, name: "\u{10000}" }];
        expect(run({ operator: "sort_limit", sortField: "name", dedupeKeys: ["id"], limit: 1 }, rows).outputItems).toEqual([rows[1]]);
    });
    it("review citations dedupe structurally regardless of key order and ignore null", () => {
        const rows = [{ citations: [{ evidence_id: "e", revision: 1 }, null] }, { citations: [null, { revision: 1, evidence_id: "e" }] }, { citations: null }];
        expect(run({ operator: "aggregate", metrics: [{ field: "", fn: "count", as: "n" }] }, rows).outputItems).toEqual([{ n: 3, citations: [{ evidence_id: "e", revision: 1 }] }]);
    });
    it("aggregates multiple dimensions and exact 23/27.6 values, preserving evidence", () => {
        const rows = [23, 27.6, null, "", undefined].map((v, i) => ({ region: "A", substance: "X", v, citations: [{ evidence_id: `e${i}` }], metadata: { evidence: [{ id: `m${i}` }] } }));
        const result = run({ operator: "aggregate", groupBy: ["region", "substance"], metrics }, rows);
        expect(result.error).toBeUndefined();
        expect(result.outputItems).toHaveLength(1);
        expect(result.outputItems[0]).toMatchObject({ region: "A", substance: "X", count: 5, sum: 50.6, avg: 25.3, min: 23, max: 27.6, p95: 27.6, citations: rows.flatMap(r => r.citations), metadata: { evidence: rows.flatMap(r => r.metadata.evidence) } });
        expect(run({ operator: "aggregate", groupBy: [], metrics }, []).outputItems).toEqual([{ count: 0, sum: null, avg: null, min: null, max: null, p95: null }]);
        expect(run({ operator: "aggregate", groupBy: ["region"], metrics }, []).outputItems).toEqual([]);
        expect(run({ operator: "aggregate", groupBy: [], metrics }, [{ v: null }, { v: " " }]).outputItems[0]).toMatchObject({ count: 2, sum: null, avg: null });
        expect(run({ operator: "aggregate", metrics: [{ field: "v", fn: "count", as: "present" }] }, [{ v: 0 }, { v: null }, { v: "" }]).outputItems).toEqual([{ present: 3 }]);
    });
    it("keeps finite large metrics and rejects overflow; normalizes negative zero keys", () => {
        const metric = { field: "v", fn: "sum", as: "s" };
        expect(run({ operator: "aggregate", metrics: [metric] }, [{ v: 1e308 }]).outputItems).toEqual([{ s: 1e308 }]);
        expect(run({ operator: "aggregate", metrics: [metric] }, [{ v: 1e308 }, { v: 1e308 }]).error).toBeTruthy();
        expect(run({ operator: "aggregate", groupBy: ["k"] }, [{ k: 0 }, { k: -0 }]).outputItems).toEqual([{ k: 0, count: 2 }]);
        expect(run({ operator: "join", joinKeys: [{ left: "k", right: "k" }] }, [{ k: -0 }], [{ k: 0 }]).outputItems).toHaveLength(1);
        for (const sortOrder of [null, "", false]) expect(run({ operator: "sort_limit", sortOrder }).error).toBeTruthy();
        for (const sortField of [null, false, 3]) expect(run({ operator: "sort_limit", sortField }).error).toBeTruthy();
    });
    it("uses nearest-rank and type-sensitive non-delimiter group identities", () => {
        expect(run({ operator: "aggregate", groupBy: [], metrics: [{ field: "v", fn: "p95", as: "p" }] }, Array.from({ length: 20 }, (_, i) => ({ v: i + 1 }))).outputItems).toEqual([{ p: 19 }]);
        const rows = [{ a: 1, b: "x" }, { a: "1", b: "x" }, { a: "a · b", b: "c" }, { a: "a", b: "b · c" }];
        expect(run({ operator: "aggregate", groupBy: ["a", "b"], metrics: [{ field: "", fn: "count", as: "n" }] }, rows).outputItems).toEqual(rows.map(r => ({ ...r, n: 1 })));
    });
    it("fails invalid metrics, aliases, keys and numeric data without partial output", () => {
        const base = { operator: "aggregate", groupBy: ["region"], metrics };
        for (const patch of [{ groupBy: "region" }, { groupBy: [""] }, { groupBy: ["region", "region"] }, { metrics: [] }, { metrics: [{ fn: "median", field: "v", as: "m" }] }, { metrics: [{ fn: "sum", field: "", as: "s" }] }, { metrics: [{ fn: "sum", field: "v", as: "region" }] }, { metrics: [{ fn: "sum", field: "v", as: "citations" }] }, { metrics: [metrics[0], metrics[0]] }]) {
            expect(run({ ...base, ...patch }).error).toBeTruthy();
        }
        for (const v of ["bad", false, {}, [], NaN, Infinity, "1e309"]) {
            const result = run(base, [{ region: "A", v: 23 }, { region: "A", v }]);
            expect(result.error).toBeTruthy(); expect(result.outputItems).toEqual([]);
        }
    });
    it("joins composite keys one-to-many, keeps missing unmatched and all collisions addressable", () => {
        const logic = { operator: "join", joinMode: "left", joinKeys: ["region", "substance", "period"].map(k => ({ left: k, right: k })) };
        const left = [{ region: "A", substance: "X", period: "P", v: 23, "right.v": "reserved", citations: ["left"] }, { region: "A", substance: "X" }];
        const right = [{ region: "A", substance: "X", period: "P", v: 27.6, citations: ["right"], metadata: { evidence: ["proof"] } }, { region: "A", substance: "X", period: "P", v: 30 }, { region: "A", substance: "X" }];
        const result = run(logic, left, right);
        expect(result.error).toBeUndefined(); expect(result.outputItems).toHaveLength(3);
        expect(result.outputItems[0]).toMatchObject({ v: 23, "right.v": "reserved", "right.right.v": 27.6, citations: ["left", "right"], metadata: { evidence: ["proof"] } });
        expect(result.outputItems[2]).toEqual(left[1]);
        expect(run({ operator: "compute", expression: "right.right.v", outputField: "copied" }, [result.outputItems[0]]).outputItems[0]).toMatchObject({ copied: 27.6 });
        expect(run({ operator: "filter", fieldPath: "right.right.v", comparator: "gt", compareValue: "25" }, [result.outputItems[0]]).outputItems).toHaveLength(1);
        expect(run({ ...logic, joinMode: "inner" }, left, right).outputItems).toHaveLength(2);
        expect(run({ operator: "join", joinKeys: [{ left: "id", right: "id" }] }, [{ id: 1 }, {}], [{ id: "1" }, {}]).outputItems).toEqual([]);
        for (const patch of [{ joinKeys: [] }, { joinKeys: [{ left: "id", right: "" }] }, { joinMode: "outer" }]) expect(run({ ...logic, ...patch }).error).toBeTruthy();
    });
    it("sorts stably with missing last then deduplicates then limits; missing identities stay distinct", () => {
        const rows = [{ id: "a", score: 23 }, { id: "a", score: 27.6, citations: ["winner"] }, { id: "b", score: 27.6 }, { score: null, tag: 1 }, { score: "", tag: 2 }];
        expect(run({ operator: "sort_limit", sortField: "score", dedupeKeys: ["id"], limit: 3 }, rows).outputItems).toEqual([rows[1], rows[2], rows[3]]);
        expect(run({ operator: "sort_limit", sortField: "score", sortOrder: "asc", dedupeKeys: ["id"] }, rows).outputItems).toEqual([{ ...rows[0], citations: ["winner"] }, rows[2], rows[3], rows[4]]);
        const evidenceRows = [{ id: 1, score: 23, citations: ["a"] }, { id: 1, score: 27.6, citations: ["b"] }];
        expect(run({ operator: "sort_limit", sortField: "score", dedupeKeys: ["id"], limit: 1 }, evidenceRows).outputItems).toEqual([{ id: 1, score: 27.6, citations: ["b", "a"] }]);
        expect(evidenceRows[1].citations).toEqual(["b"]);
        for (const patch of [{ dedupeKeys: "id" }, { dedupeKeys: [""] }, { dedupeKeys: ["id", "id"] }, { sortField: " " }, { sortOrder: "bad" }]) expect(run({ operator: "sort_limit", ...patch }).error).toBeTruthy();
    });
});

describe("T02 compute contract", () => {
    const sample = { concentration_ng_l: 50, flow_m3_day: 100000, population: 500000, excretion_factor: 2.3, excretion_factor_source: { version: "fixture-v1", citations: [{ evidence_id: "fixture-not-real-evidence", revision: 1 }] } };
    const comparison = { regionField: "region", substanceField: "substance", periodField: "period", baselinePeriodField: "baseline", valueField: "amount" };
    const config = { operator: "compute" as const, outputField: "change", periodComparison: comparison };
    const row = (period: string | number, baseline: string | number, amount: unknown, region: string | number = "A", substance = "X") => ({ region, substance, period, baseline, amount });
    it("calculates corrected units and preserves explicit factor provenance", () => {
        expect(evaluateNumericExpression(sample, "wbe_consumption_mg_1000p")).toEqual({ value: 23 });
        expect(evaluateNumericExpression(sample, "concentration_ng_l * flow_m3_day * excretion_factor / population")).toEqual({ value: 23 });
        expect(evaluateNumericExpression({ nested: { value: "2.5e1" } }, "-(nested.value + 5) / 2")).toEqual({ value: -15 });
    });
    for (const value of [undefined, null, "", " ", "bad", "0x10", true, [], {}, NaN, Infinity, "Infinity", "1e309"]) {
        it(`rejects invalid numeric field ${String(value)}`, () => {
            expect(evaluateNumericExpression({ x: value }, "x + 1").error).toBeTruthy();
        });
    }
    for (const expression of ["", "x / 0", "x +", "(x", "x)", "x 2", "x..a", "x @ 2", "x * huge * huge", "huge * huge / huge"]) {
        it(`rejects invalid expression ${expression}`, () => {
            expect(evaluateNumericExpression({ x: 2, huge: 1e308 }, expression).error).toBeTruthy();
        });
    }
    it("rejects array/prototype traversal, default output collisions and normalizes zero keys", () => {
        for (const expression of ["readings.length", "readings.0", "inherited"]) expect(evaluateNumericExpression(Object.assign(Object.create({ inherited: 23 }), { readings: [23] }), expression).error).toBeTruthy();
        expect(evaluateNumericExpression({ "right.v": 27.6, right: { v: 23 } }, "right.v").value).toBe(27.6);
        for (const outputField of [undefined, "", " "]) {
            const result = evaluateComputeOperation([{ x: 2, computed_value: 99 }], { operator: "compute", expression: "x * 2", outputField });
            expect(result.error).toBeTruthy(); expect(result.items).toEqual([]);
        }
        expect(evaluateComputeOperation([row(1, 0, 23), row(-0, -1, 20)], config).items[0]).toMatchObject({ change: 15, change_status: "ok" });
        expect(evaluateComputeOperation([row(0, -1, 20), row(-0, -1, 20)], config).error).toBeTruthy();
    });
    it("requires explicit factor and source and returns no fabricated/partial output", () => {
        for (const patch of [{ excretion_factor: undefined }, { excretion_factor_source: undefined }, { excretion_factor_source: " " }, { excretion_factor_source: 1 }]) {
            expect(evaluateNumericExpression({ ...sample, ...patch }, "wbe_consumption_mg_1000p").error).toBeTruthy();
        }
        expect(evaluateNumericExpression(3, "missing").error).toBeTruthy();
        const result = evaluateComputeOperation([{ x: 2 }, { x: "bad" }], { operator: "compute", expression: "x * 2" });
        expect(result.error).toBeTruthy();
        expect(result.items).toEqual([]);
        expect(evaluateComputeOperation([sample], { operator: "compute" }).error).toBeTruthy();
    });
    it("validates versioned factor citations without claiming evidence authenticity", () => {
        const invalid = [null, {}, "source", { version: " ", citations: [{ evidence_id: "fixture", revision: 1 }] },
            { version: "v1", citations: [] }, { version: "v1", citations: [null] },
            ...[0, -1, 1.5, "1", Infinity, 9007199254740992].map((revision) => ({ version: "v1", citations: [{ evidence_id: "fixture", revision }] })),
            { version: "v1", citations: [{ evidence_id: " ", revision: 1 }] }];
        for (const source of invalid) expect(evaluateNumericExpression({ ...sample, excretion_factor_source: source }, "excretion_factor").error).toBeTruthy();
        expect(evaluateNumericExpression({ x: -0.00005 }, "x").value).toBeCloseTo(0);
        expect(evaluateNumericExpression({ x: 1e308 }, "x").value).toBe(1e308);
        expect(evaluateNumericExpression({ x: 2 }, "x + 1")).toEqual({ value: 3 });
    });
    it("matches region, substance and explicit real period, not input order", () => {
        const result = evaluateComputeOperation([
            row("2026-10", "2026-09", 23), row("2026-09", "2026-08", 20),
            row("2026-09", "2026-08", 100, "B"), row("2026-09", "2026-08", 200, "A", "Y"),
            row("2025-10", "2025-09", 10), row("2026-10-yoy", "2025-10", 23),
            row("zero", "none", 0), row("after-zero", "zero", 23),
        ], config);
        expect(result.error).toBeUndefined();
        expect(result.items[0]).toMatchObject({ change: 15, change_status: "ok" });
        expect(result.items[1]).toMatchObject({ change: null, change_status: "missing_baseline" });
        expect(result.items[5]).toMatchObject({ change: 130, change_status: "ok" });
        expect(result.items[7]).toMatchObject({ change: null, change_status: "zero_baseline" });
        expect(evaluateComputeOperation([row(1, "1", 23)], config).items[0]).toMatchObject({ change_status: "missing_baseline" });
        expect(evaluateComputeOperation([row("now", "old", 23, 1), row("old", "none", 10, "1")], config).items[0]).toMatchObject({ change_status: "missing_baseline" });
        const node = makeNode({ id: "period", type: CanvasNodeType.Logic, metadata: { logic: config } });
        expect(evaluateLogicNode(node, [row("now", "old", 23), row("old", "none", 20)], []).outputItems[0]).toMatchObject({ change: 15 });
    });
    it("rejects invalid comparison contracts, missing keys, ambiguous and invalid baselines", () => {
        for (const periodComparison of [null, {}, { ...comparison, periodField: "" }, { ...comparison, valueField: 1 }, { ...comparison, unexpected: "x" }]) {
            const result = evaluateComputeOperation([], { ...config, periodComparison } as never);
            expect(result.error).toBeTruthy();
        }
        for (const items of [
            [row("now", "old", 23), row("old", "none", 20), row("old", "none", 20)],
            [row("now", "old", 23), row("old", "none", Infinity)],
            [{ ...row("now", "old", 23), region: null }],
            [{ ...row("now", "old", 23), baseline: "" }],
            [{ ...row("now", "old", 23), change_status: "old" }],
        ]) {
            const result = evaluateComputeOperation(items, config);
            expect(result.error).toBeTruthy();
            expect(result.items).toEqual([]);
        }
        expect(evaluateComputeOperation([], { ...config, expression: "amount" }).error).toBeTruthy();
    });
});

describe("T01a explicit logic contract", () => {
    const evaluate = (logic: unknown, items = [{ score: 2 }]) => evaluateLogicNode(makeNode({
        id: "contract", type: CanvasNodeType.Logic,
        metadata: { logic: logic as NonNullable<CanvasNodeData["metadata"]>["logic"] },
    }), items, []);

    for (const operator of ["unknown", "", " ", null, 1, "toString"]) {
        it(`rejects explicit operator ${JSON.stringify(operator)}`, () => {
            const result = evaluate({ operator, fieldPath: "score" });
            expect(result.error).toContain("operator");
            expect(result.outputItems).toEqual([]);
        });
    }
    for (const limit of ["2", null, false, 0, -1, 1.5, NaN, Infinity]) {
        it(`rejects explicit sort limit ${String(limit)}`, () => {
            const result = evaluate({ operator: "sort_limit", limit });
            expect(result.error).toContain("limit");
            expect(result.outputItems).toEqual([]);
        });
    }
    it("preserves omitted defaults and accepts a positive integer limit", () => {
        expect(evaluate({}).error).toBeUndefined();
        expect(evaluate(undefined).error).toBeUndefined();
        expect(evaluateComputeOperation([{ score: 2, computed_value: 1 }], { operator: "compute", expression: "score * 2" }).error).toBeTruthy();
        expect(evaluate({ operator: "sort_limit" }).outputItems).toEqual([{ score: 2 }]);
        expect(evaluate({ operator: "sort_limit", limit: 1 }).outputItems).toHaveLength(1);
    });
    it("rejects compute aliases colliding with any input row without partial output", () => {
        const items = [{ score: 2 }, { score: 3, result: null }];
        const result = evaluateComputeOperation(items, { operator: "compute", expression: "score * 2", outputField: "result" });
        expect(result.error).toContain("outputField");
        expect(result.items).toEqual([]);
        expect(items[1].result).toBeNull();
        expect(evaluate({ operator: "compute", expression: "score * 2", outputField: "score" }).error).toContain("outputField");
        expect(evaluate({ operator: "compute", expression: "score * 2", outputField: "result" }).outputItems).toEqual([{ score: 2, result: 4 }]);
    });
});

describe("canvas low-code dataflow & logic evaluator", () => {
    it("parses JSON, CSV, Markdown, TXT, and Intel data source formats", () => {
        const jsonNode = makeNode({
            id: "ds-json",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: {
                    format: "json",
                    rawText: JSON.stringify([
                        { id: "a1", title: "赛博朋克雨夜", score: 92 },
                        { id: "a2", title: "古风客栈", score: 75 },
                    ]),
                },
            },
        });
        const jsonParsed = parseDataSourceNode(jsonNode);
        expect(jsonParsed.error).toBeUndefined();
        expect(jsonParsed.items).toHaveLength(2);
        expect(jsonParsed.items[0]).toEqual({ id: "a1", title: "赛博朋克雨夜", score: 92 });

        const csvNode = makeNode({
            id: "ds-csv",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: {
                    format: "csv",
                    rawText: 'id,title,score,featured\nb1,"霓虹街角, 特写",88,true\nb2,废土公路,61,false',
                },
            },
        });
        const csvParsed = parseDataSourceNode(csvNode);
        expect(csvParsed.items).toEqual([
            { id: "b1", title: "霓虹街角, 特写", score: 88, featured: true },
            { id: "b2", title: "废土公路", score: 61, featured: false },
        ]);

        const mdNode = makeNode({
            id: "ds-md",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: {
                    format: "markdown",
                    rawText: "- 镜头一：主角推门而入\n- 镜头二：俯拍长廊追逐",
                },
            },
        });
        const mdParsed = parseDataSourceNode(mdNode);
        expect(mdParsed.items).toEqual([
            { id: "1", text: "镜头一：主角推门而入", title: "镜头一：主角推门而入" },
            { id: "2", text: "镜头二：俯拍长廊追逐", title: "镜头二：俯拍长廊追逐" },
        ]);

        const txtNode = makeNode({
            id: "ds-txt",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: {
                    format: "txt",
                    rawText: "第一条线索\n\n第二条线索",
                },
            },
        });
        expect(parseDataSourceNode(txtNode).items).toEqual(["第一条线索", "第二条线索"]);
    });

    it("evaluates set operations (intersect, union, difference) across input_a and input_b", () => {
        const sourceA = makeNode({
            id: "source-a",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: {
                    format: "json",
                    rawText: JSON.stringify([
                        { id: "k1", name: "角色甲" },
                        { id: "k2", name: "角色乙" },
                    ]),
                },
            },
        });
        const sourceB = makeNode({
            id: "source-b",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: {
                    format: "json",
                    rawText: JSON.stringify([
                        { id: "k2", name: "角色乙（B源）" },
                        { id: "k3", name: "角色丙" },
                    ]),
                },
            },
        });
        const intersectNode = makeNode({
            id: "logic-intersect",
            type: CanvasNodeType.Logic,
            metadata: { logic: { operator: "intersect", matchKey: "id" } },
        });
        const unionNode = makeNode({
            id: "logic-union",
            type: CanvasNodeType.Logic,
            metadata: { logic: { operator: "union", matchKey: "id" } },
        });
        const diffNode = makeNode({
            id: "logic-diff",
            type: CanvasNodeType.Logic,
            metadata: { logic: { operator: "difference", matchKey: "id" } },
        });

        const connections: CanvasConnection[] = [
            { id: "c1", fromNodeId: "source-a", toNodeId: "logic-intersect", toHandleId: "input_a" },
            { id: "c2", fromNodeId: "source-b", toNodeId: "logic-intersect", toHandleId: "input_b" },
            { id: "c3", fromNodeId: "source-a", toNodeId: "logic-union", toHandleId: "input_a" },
            { id: "c4", fromNodeId: "source-b", toNodeId: "logic-union", toHandleId: "input_b" },
            { id: "c5", fromNodeId: "source-a", toNodeId: "logic-diff", toHandleId: "input_a" },
            { id: "c6", fromNodeId: "source-b", toNodeId: "logic-diff", toHandleId: "input_b" },
        ];

        const evalResult = evaluateCanvasDataflow([sourceA, sourceB, intersectNode, unionNode, diffNode], connections);
        expect(evalResult.nodes.get("logic-intersect")?.outputItems).toEqual([{ id: "k2", name: "角色乙" }]);
        expect(evalResult.nodes.get("logic-union")?.outputItems).toEqual([
            { id: "k1", name: "角色甲" },
            { id: "k2", name: "角色乙" },
            { id: "k3", name: "角色丙" },
        ]);
        expect(evalResult.nodes.get("logic-diff")?.outputItems).toEqual([{ id: "k1", name: "角色甲" }]);
    });

    it("splits if_else into True/False ports, extracts nested fields, and gates downstream generation", () => {
        const source = makeNode({
            id: "source-1",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: {
                    format: "json",
                    rawText: JSON.stringify([
                        { id: "1", prompt: "高分机甲战斗镜头", meta: { score: 95 } },
                        { id: "2", prompt: "高分霓虹追逐镜头", meta: { score: 88 } },
                    ]),
                },
            },
        });
        const ifNode = makeNode({
            id: "logic-if",
            title: "高分筛选",
            type: CanvasNodeType.Logic,
            metadata: {
                logic: {
                    operator: "if_else",
                    fieldPath: "meta.score",
                    comparator: "gt",
                    compareValue: "80",
                },
            },
        });
        const extractNode = makeNode({
            id: "logic-extract",
            type: CanvasNodeType.Logic,
            metadata: {
                logic: {
                    operator: "json_extract",
                    fieldPath: "prompt",
                },
            },
        });
        const trueImageNode = makeNode({
            id: "img-true",
            type: CanvasNodeType.Image,
            metadata: { prompt: "请按命中数据生成画面" },
        });
        const falseImageNode = makeNode({
            id: "img-false",
            type: CanvasNodeType.Image,
            metadata: { prompt: "未命中备选分支" },
        });

        const nodes = [source, ifNode, extractNode, trueImageNode, falseImageNode];
        const connections: CanvasConnection[] = [
            { id: "e1", fromNodeId: "source-1", toNodeId: "logic-if" },
            { id: "e2", fromNodeId: "logic-if", fromHandleId: "true", toNodeId: "logic-extract" },
            { id: "e3", fromNodeId: "logic-extract", toNodeId: "img-true" },
            { id: "e4", fromNodeId: "logic-if", fromHandleId: "false", toNodeId: "img-false" },
        ];

        const dataflow = evaluateCanvasDataflow(nodes, connections);
        expect(dataflow.getEdgeItems("logic-if", "true").items).toHaveLength(2);
        expect(dataflow.getEdgeItems("logic-if", "false").items).toHaveLength(0);
        expect(dataflow.getEdgeText("logic-extract")).toBe("高分机甲战斗镜头\n高分霓虹追逐镜头");

        // True 分支激活：下游生成节点自动接收提取出的文本
        const trueContext = buildNodeGenerationContext("img-true", nodes, connections, "请按命中数据生成画面", []);
        expect(trueContext.prompt).toContain("高分机甲战斗镜头\n高分霓虹追逐镜头");
        expect(findBlockedLogicBranchInput("img-true", nodes, connections)).toBeNull();

        // False 分支为空（0 条命中）：门控拦截下游生成
        const blocked = findBlockedLogicBranchInput("img-false", nodes, connections);
        expect(blocked).not.toBeNull();
        expect(blocked?.reason).toContain("False 分支当前未命中");
        expect(() => buildNodeGenerationContext("img-false", nodes, connections, "未命中备选分支", [])).toThrow(
            "逻辑节点「高分筛选」False 分支当前未命中（0 条数据），已跳过下游生成",
        );
    });

    it("detects cyclic connections safely without infinite recursion", () => {
        const logicA = makeNode({
            id: "logic-a",
            type: CanvasNodeType.Logic,
            metadata: { logic: { operator: "filter", comparator: "not_empty" } },
        });
        const logicB = makeNode({
            id: "logic-b",
            type: CanvasNodeType.Logic,
            metadata: { logic: { operator: "filter", comparator: "not_empty" } },
        });
        const connections: CanvasConnection[] = [
            { id: "c-ab", fromNodeId: "logic-a", toNodeId: "logic-b" },
            { id: "c-ba", fromNodeId: "logic-b", toNodeId: "logic-a" },
        ];

        const res = evaluateCanvasDataflow([logicA, logicB], connections);
        expect(res.nodes.get("logic-a")?.cycleDetected).toBe(true);
        expect(res.nodes.get("logic-a")?.error).toContain("循环连线");
    });

    it("computes dual input/output port geometry for logic nodes", () => {
        const intersectNode = makeNode({
            id: "logic-set",
            type: CanvasNodeType.Logic,
            position: { x: 100, y: 200 },
            width: 400,
            height: 300,
            metadata: { logic: { operator: "intersect" } },
        });
        const branchNode = makeNode({
            id: "logic-branch",
            type: CanvasNodeType.Logic,
            position: { x: 100, y: 200 },
            width: 400,
            height: 300,
            metadata: { logic: { operator: "if_else" } },
        });

        expect(isLogicDualInput(intersectNode)).toBe(true);
        expect(isLogicDualOutput(intersectNode)).toBe(false);
        expect(isLogicDualInput(branchNode)).toBe(false);
        expect(isLogicDualOutput(branchNode)).toBe(true);

        const topY = logicNodeHandleY(intersectNode, "input_a")!;
        const bottomY = logicNodeHandleY(intersectNode, "input_b")!;
        expect(topY).toBeCloseTo(200 + 300 * 0.38);
        expect(bottomY).toBeCloseTo(200 + 300 * 0.72);
        expect(logicNodeHandleAtY(intersectNode, "target", topY + 5, 40)).toBe("input_a");
        expect(logicNodeHandleAtY(intersectNode, "target", bottomY - 5, 40)).toBe("input_b");
        expect(logicNodeHandleAtY(branchNode, "source", topY + 4, 40)).toBe("true");
        expect(logicNodeHandleAtY(branchNode, "source", bottomY - 4, 40)).toBe("false");
    });

    it("evaluates data-source -> logic -> skill -> image pipeline and gates empty branch through skill", () => {
        const source = makeNode({
            id: "ds-intel",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: {
                    format: "json",
                    rawText: JSON.stringify([
                        { id: "doc_1", title: "固态电池量产突破", score: 92 },
                    ]),
                },
            },
        });
        const logicNode = makeNode({
            id: "logic-filter",
            title: "高优情报分流",
            type: CanvasNodeType.Logic,
            metadata: {
                logic: {
                    operator: "if_else",
                    fieldPath: "score",
                    comparator: "gt",
                    compareValue: "85",
                },
            },
        });
        const trueSkillNode = makeNode({
            id: "skill-true",
            title: "分镜视觉提炼",
            type: CanvasNodeType.Skill,
            metadata: {
                content: "将输入情报转化为赛博朋克风格实验室特写镜头提示词",
                skillName: "分镜视觉提炼",
                skillDescription: "提炼高张力商业分镜画面",
                skillPrompts: ["将输入情报转化为赛博朋克风格实验室特写镜头提示词"],
            },
        });
        const falseSkillNode = makeNode({
            id: "skill-false",
            title: "次要情报摘要",
            type: CanvasNodeType.Skill,
            metadata: {
                content: "生成简短文字海报背景",
                skillName: "次要情报摘要",
            },
        });
        const trueImage = makeNode({
            id: "img-from-skill-true",
            type: CanvasNodeType.Image,
            metadata: { prompt: "生成主视觉海报" },
        });
        const falseImage = makeNode({
            id: "img-from-skill-false",
            type: CanvasNodeType.Image,
            metadata: { prompt: "生成备选海报" },
        });

        const nodes = [source, logicNode, trueSkillNode, falseSkillNode, trueImage, falseImage];
        const connections: CanvasConnection[] = [
            { id: "c1", fromNodeId: "ds-intel", toNodeId: "logic-filter" },
            { id: "c2", fromNodeId: "logic-filter", fromHandleId: "true", toNodeId: "skill-true" },
            { id: "c3", fromNodeId: "skill-true", toNodeId: "img-from-skill-true" },
            { id: "c4", fromNodeId: "logic-filter", fromHandleId: "false", toNodeId: "skill-false" },
            { id: "c5", fromNodeId: "skill-false", toNodeId: "img-from-skill-false" },
        ];

        const evalResult = evaluateCanvasDataflow(nodes, connections);
        const skillTrueText = evalResult.getEdgeText("skill-true");
        expect(skillTrueText).toContain("【技能：分镜视觉提炼】");
        expect(skillTrueText).toContain("将输入情报转化为赛博朋克风格实验室特写镜头提示词");
        expect(skillTrueText).toContain("固态电池量产突破");

        const trueContext = buildNodeGenerationContext("img-from-skill-true", nodes, connections, "生成主视觉海报", []);
        expect(trueContext.prompt).toContain("【技能：分镜视觉提炼】");
        expect(trueContext.prompt).toContain("固态电池量产突破");

        // False branch (0 items) -> falseSkillNode -> falseImage must be gated
        expect(evalResult.getEdgeItems("skill-false").active).toBe(false);
        const blocked = findBlockedLogicBranchInput("img-from-skill-false", nodes, connections);
        expect(blocked).not.toBeNull();
        expect(blocked?.reason).toContain("False 分支当前未命中");
    });

    it("evaluates substance-precursor analytical DAG: compute -> join -> aggregate -> sort_limit", () => {
        const wbeSource = makeNode({
            id: "ds-wbe",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: {
                    format: "intel",
                    rawText: JSON.stringify([
                        {
                            id: "w1",
                            region: "华南A区",
                            substance: "甲基苯丙胺",
                            concentration_ng_l: 50,
                            flow_m3_day: 100000,
                            excretion_factor: 2.3,
                            excretion_factor_source: { version: "fixture-v1", citations: [{ evidence_id: "fixture-not-real-evidence", revision: 1 }] },
                            population: 500000,
                        },
                        {
                            id: "w2",
                            region: "华南A区",
                            substance: "甲基苯丙胺",
                            concentration_ng_l: 70,
                            flow_m3_day: 100000,
                            excretion_factor: 2.3,
                            excretion_factor_source: { version: "fixture-v1", citations: [{ evidence_id: "fixture-not-real-evidence", revision: 1 }] },
                            population: 500000,
                        },
                        {
                            id: "w3",
                            region: "西南B区",
                            substance: "氯胺酮",
                            concentration_ng_l: 20,
                            flow_m3_day: 80000,
                            excretion_factor: 3.0,
                            excretion_factor_source: { version: "fixture-v1", citations: [{ evidence_id: "fixture-not-real-evidence", revision: 1 }] },
                            population: 400000,
                        },
                    ]),
                },
            },
        });

        const precursorSource = makeNode({
            id: "ds-precursor",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: {
                    format: "intel",
                    rawText: JSON.stringify([
                        { region: "华南A区", precursor: "1-苯基-2-丙酮", anomaly_lots: 4 },
                        { region: "西南B区", precursor: "邻氯苯基环戊基酮", anomaly_lots: 1 },
                    ]),
                },
            },
        });

        const computeNode = makeNode({
            id: "logic-compute",
            type: CanvasNodeType.Logic,
            metadata: {
                logic: {
                    operator: "compute",
                    expression: "wbe_consumption_mg_1000p",
                    outputField: "consumption_mg_1000p",
                },
            },
        });

        const joinNode = makeNode({
            id: "logic-join",
            type: CanvasNodeType.Logic,
            metadata: {
                logic: {
                    operator: "join",
                    joinKeys: [{ left: "region", right: "region" }],
                    joinMode: "inner",
                },
            },
        });

        const aggregateNode = makeNode({
            id: "logic-agg",
            type: CanvasNodeType.Logic,
            metadata: {
                logic: {
                    operator: "aggregate",
                    groupBy: ["region"],
                    metrics: [{ field: "consumption_mg_1000p", fn: "avg", as: "avg_consumption" }, { field: "", fn: "count", as: "count" }],
                },
            },
        });

        const sortLimitNode = makeNode({
            id: "logic-topn",
            type: CanvasNodeType.Logic,
            metadata: {
                logic: {
                    operator: "sort_limit",
                    sortField: "avg_consumption",
                    sortOrder: "desc",
                    limit: 1,
                },
            },
        });

        const nodes = [wbeSource, precursorSource, computeNode, joinNode, aggregateNode, sortLimitNode];
        const connections: CanvasConnection[] = [
            { id: "c1", fromNodeId: "ds-wbe", toNodeId: "logic-compute" },
            { id: "c2", fromNodeId: "logic-compute", toNodeId: "logic-join", toHandleId: "input_a" },
            { id: "c3", fromNodeId: "ds-precursor", toNodeId: "logic-join", toHandleId: "input_b" },
            { id: "c4", fromNodeId: "logic-join", toNodeId: "logic-agg" },
            { id: "c5", fromNodeId: "logic-agg", toNodeId: "logic-topn" },
        ];

        const evalResult = evaluateCanvasDataflow(nodes, connections);
        const computedItems = evalResult.nodes.get("logic-compute")?.outputItems as Array<Record<string, unknown>>;
        expect(computedItems).toHaveLength(3);
        // ng/L * m3/day * correction multiplier / population = mg/1000 people/day
        expect(computedItems[0].consumption_mg_1000p).toBe(23);
        // 70 * 100000 * 2.3 / 500000 = 32.2
        expect(computedItems[1].consumption_mg_1000p).toBe(32.2);
        // 20 * 80000 * 3.0 / 400000 = 12
        expect(computedItems[2].consumption_mg_1000p).toBe(12);

        const joinedItems = evalResult.nodes.get("logic-join")?.outputItems as Array<Record<string, unknown>>;
        expect(joinedItems).toHaveLength(3);
        expect(joinedItems[0].precursor).toBe("1-苯基-2-丙酮");
        expect(joinedItems[0].anomaly_lots).toBe(4);

        const aggItems = evalResult.nodes.get("logic-agg")?.outputItems as Array<Record<string, unknown>>;
        expect(aggItems).toHaveLength(2);
        // 华南A区 avg: (23 + 32.2) / 2 = 27.6
        expect(aggItems[0]).toMatchObject({
            region: "华南A区",
            count: 2,
            avg_consumption: 27.6,
        });

        const topItems = evalResult.nodes.get("logic-topn")?.outputItems as Array<Record<string, unknown>>;
        expect(topItems).toHaveLength(1);
        expect(topItems[0].region).toBe("华南A区");
        expect(topItems[0].avg_consumption).toBe(27.6);
    });
});

describe("T07 managed run authoritative output", () => {
    it("uses server computed output for skill nodes instead of the prompt preview", () => {
        const source = makeNode({
            id: "src",
            type: CanvasNodeType.DataSource,
            metadata: { dataSource: { format: "json", rawText: JSON.stringify([{ v: 1 }]) } },
        });
        const skill = makeNode({
            id: "skill",
            type: CanvasNodeType.Skill,
            metadata: {
                skillPrompts: ["本地预览提示词"],
                derived: { runId: "cw_t07", canvasRevision: 3, status: "succeeded" },
                computed: [{ regional_metrics: [{ region_code: "R1" }], citations: [] }],
                skillOutput: { regional_metrics: [{ region_code: "R1" }] },
            },
        });
        const connections: CanvasConnection[] = [
            { id: "c1", fromNodeId: "src", toNodeId: "skill" },
        ];
        const evaluation = evaluateCanvasDataflow([source, skill], connections);
        expect(evaluation.getEdgeItems("skill").items).toEqual([
            { regional_metrics: [{ region_code: "R1" }], citations: [] },
        ]);
    });

    it("clears stale output when the authoritative result is an empty array", () => {
        const source = makeNode({
            id: "src",
            type: CanvasNodeType.DataSource,
            metadata: { dataSource: { format: "json", rawText: JSON.stringify([{ v: 1 }]) } },
        });
        const skill = makeNode({
            id: "skill",
            type: CanvasNodeType.Skill,
            metadata: {
                skillPrompts: ["预览"],
                derived: { runId: "cw_t07", canvasRevision: 3, status: "succeeded" },
                computed: [],
            },
        });
        const connections: CanvasConnection[] = [
            { id: "c1", fromNodeId: "src", toNodeId: "skill" },
        ];
        const evaluation = evaluateCanvasDataflow([source, skill], connections);
        expect(evaluation.getEdgeItems("skill").items).toEqual([]);
        expect(evaluation.getEdgeItems("skill").active).toBe(false);
    });

    it("serves if_else branches from server computedBranches", () => {
        const source = makeNode({
            id: "src",
            type: CanvasNodeType.DataSource,
            metadata: { dataSource: { format: "json", rawText: JSON.stringify([{ risk: "high" }, { risk: "low" }]) } },
        });
        const branch = makeNode({
            id: "branch",
            type: CanvasNodeType.Logic,
            metadata: {
                logic: { operator: "if_else", matchKey: "risk", equals: "high" },
                derived: { runId: "cw_t07", canvasRevision: 3, status: "succeeded" },
                computed: [{ risk: "high" }],
                computedBranches: { true: [{ risk: "high" }], false: [{ risk: "low" }] },
            },
        });
        const connections: CanvasConnection[] = [
            { id: "c1", fromNodeId: "src", toNodeId: "branch" },
        ];
        const evaluation = evaluateCanvasDataflow([source, branch], connections);
        expect(evaluation.getEdgeItems("branch", "true").items).toEqual([{ risk: "high" }]);
        expect(evaluation.getEdgeItems("branch", "false").items).toEqual([{ risk: "low" }]);
    });

    it("keeps local evaluation when the managed run did not succeed", () => {
        const source = makeNode({
            id: "src",
            type: CanvasNodeType.DataSource,
            metadata: {
                dataSource: { format: "json", rawText: JSON.stringify([{ stale: true }]) },
                derived: { runId: "cw_t07", canvasRevision: 3, status: "failed" },
                computed: [{ fresh: true }],
            },
        });
        const evaluation = evaluateCanvasDataflow([source], []);
        expect(evaluation.getEdgeItems("src").items).toEqual([{ stale: true }]);
    });
});
