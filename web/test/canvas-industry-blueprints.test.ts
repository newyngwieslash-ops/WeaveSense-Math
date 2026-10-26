import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";
import { CanvasNodeType, type CanvasNodeData } from "@/types/canvas";
import { evaluateCanvasDataflow, evaluateComputeOperation, isLogicDualInput, isLogicDualOutput } from "@/lib/canvas/dataflow-evaluator";
import { createCanvasNode } from "@/lib/canvas/canvas-project-domain";
import type { SkillInstall } from "@/services/api/intelligence";
import type * as BlueprintModule from "@/lib/canvas/canvas-industry-blueprints";

// Missing implementation is an assertion failure in RED, not a module-loader error.
const modulePath = new URL("../src/lib/canvas/canvas-industry-blueprints.ts", import.meta.url);
const blueprints: typeof BlueprintModule | undefined = existsSync(modulePath) ? await import(modulePath.href) : undefined;
const api = () => { expect(blueprints).toBeDefined(); return blueprints!; };
const metadata = (node: CanvasNodeData) => node.metadata as unknown as BlueprintModule.IndustryBlueprintMetadata;
const ids = ["wastewater", "nps", "precursor"] as const;
const source = (path: string) => readFileSync(new URL(`../src/${path}`, import.meta.url), "utf8");
// Test-only installed-library response; never shipped as a production pin.
const installed: SkillInstall[] = ["nps-substitution-screening", "precursor-substance-correlation"].map((skillId, i) => ({
    id: `fixture-install-${i}`, skillId, version: "fixture-version-7", hash: String(i + 1).repeat(64),
    packId: "substance-precursor-intel", updatedAt: "fixture",
}));
const build = (blueprintId: typeof ids[number], extra = {}) => api().appendIndustryBlueprint({ blueprintId, nodes: [], connections: [], workspaceId: "fixture-workspace", installedSkills: installed, ...extra });

describe("T20 industry blueprints", () => {
    for (const id of ids) {
        test(`${id}: connected acyclic graph uses registered node kinds and real ports`, () => {
            const graph = build(id);
            const byId = new Map(graph.nodes.map(node => [node.id, node]));
            expect(byId.size).toBe(graph.nodes.length);
            const indegree = new Map(graph.nodes.map(node => [node.id, 0]));
            for (const edge of graph.connections) {
                const from = byId.get(edge.fromNodeId)!;
                const to = byId.get(edge.toNodeId)!;
                expect(from).toBeDefined(); expect(to).toBeDefined();
                expect(edge.fromHandleId === undefined || (isLogicDualOutput(from) && ["true", "false"].includes(edge.fromHandleId))).toBe(true);
                expect(isLogicDualInput(to) ? ["input_a", "input_b"].includes(edge.toHandleId!) : edge.toHandleId === undefined).toBe(true);
                indegree.set(to.id, indegree.get(to.id)! + 1);
                expect(from.position.x + from.width).toBeLessThan(to.position.x);
            }
            const queue = [...indegree].filter(([, degree]) => degree === 0).map(([id]) => id);
            let visited = 0;
            while (queue.length) {
                const nodeId = queue.shift()!; visited++;
                for (const edge of graph.connections.filter(edge => edge.fromNodeId === nodeId)) {
                    const degree = indegree.get(edge.toNodeId)! - 1;
                    indegree.set(edge.toNodeId, degree);
                    if (!degree) queue.push(edge.toNodeId);
                }
            }
            expect(visited).toBe(graph.nodes.length);
            for (const node of graph.nodes) {
                expect(Object.values(CanvasNodeType)).toContain(node.type);
                expect(graph.connections.some(edge => edge.fromNodeId === node.id || edge.toNodeId === node.id)).toBe(true);
                const m = metadata(node);
                expect(m.workflowParameters.industryBlueprint).toMatchObject({ fixture: true, publishable: false, blueprintId: id });
                expect(node.title).toContain("示例");
                if (node.type === CanvasNodeType.Chart) { expect(m.xKey).toBeTruthy(); expect(m.series?.length).toBeGreaterThan(0); expect(m.series?.every(key => typeof key === "string")).toBe(true); }
                if (m.logic?.operator === "aggregate") { expect(Array.isArray(m.logic.groupBy)).toBe(true); expect(m.logic.metrics?.length).toBeGreaterThan(0); for (const metric of m.logic.metrics!) { expect(metric.field).toBeTruthy(); expect(metric.as).toBeTruthy(); expect(["count", "sum", "avg", "min", "max"]).toContain(metric.fn); } }
                if (m.logic?.operator === "sort_limit") { expect(m.logic.sortField).toBeTruthy(); expect(m.logic.sortOrder).toBe("desc"); expect(m.logic.limit).toBeGreaterThan(0); }
                if (node.type === CanvasNodeType.DataSource) {
                    expect(m.dataSource?.sourceBinding?.kind).toBe("static");
                    const rows = JSON.parse(m.dataSource!.rawText!);
                    expect(rows.length).toBeGreaterThan(0);
                    for (const row of rows) expect(row).toMatchObject({ fixture: true, publishable: false });
                }
            }
        });
    }

    test("wastewater: explicit factor and provenance, baseline input, null/status contract and ranked outputs", () => {
        const graph = build("wastewater");
        const formula = graph.nodes.find(n => metadata(n).logic?.expression)!;
        expect(metadata(formula).logic?.expression).toBe("concentration_ng_l * flow_m3_day * excretion_factor / population");
        const input = graph.nodes.find(n => graph.connections.some(e => e.fromNodeId === n.id && e.toNodeId === formula.id))!;
        const rows = JSON.parse(metadata(input).dataSource!.rawText!);
        expect(rows[0].excretion_factor_source).toEqual({ version: "fixture-v1", citations: [{ evidence_id: "fixture-not-real-evidence", revision: 1 }] });
        const comparison = graph.nodes.find(n => metadata(n).logic?.periodComparison)!;
        expect(metadata(comparison).logic?.periodComparison).toEqual({ regionField: "region_code", substanceField: "substance_name", periodField: "period", baselinePeriodField: "baseline_period", valueField: "consumption" });
        expect(metadata(comparison).logic?.expression).toBeUndefined();
        const config = metadata(comparison).logic!;
        const result = evaluateComputeOperation([
            { region_code: "A", substance_name: "X", period: "current", baseline_period: "previous", consumption: 4 },
            { region_code: "B", substance_name: "X", period: "current", baseline_period: "previous", consumption: 4 },
            { region_code: "B", substance_name: "X", period: "previous", baseline_period: "earlier", consumption: 0 },
        ], { operator: "compute", outputField: config.outputField, periodComparison: config.periodComparison });
        expect(result.error).toBeUndefined();
        expect(result.items[0]).toMatchObject({ change_pct: null, change_pct_status: "missing_baseline" });
        expect(result.items[1]).toMatchObject({ change_pct: null, change_pct_status: "zero_baseline" });
        expect(graph.nodes.some(n => n.title.includes("基线") && n.type === CanvasNodeType.DataSource)).toBe(true);
        const ranking = graph.nodes.find(n => metadata(n).logic?.operator === "sort_limit")!;
        for (const type of [CanvasNodeType.Chart, CanvasNodeType.Markdown]) expect(graph.connections.some(e => e.fromNodeId === ranking.id && graph.nodes.find(n => n.id === e.toNodeId)?.type === type)).toBe(true);
    });

    test("Skill graphs extract real output arrays; precursor joins composite keys on both ports", () => {
        for (const [id, field] of [["nps", "emerging_substances"], ["precursor", "correlations"]] as const) {
            const graph = build(id);
            const skill = graph.nodes.find(n => n.type === CanvasNodeType.Skill)!;
            const extractor = graph.nodes.find(n => metadata(n).logic?.operator === "json_extract")!;
            expect(metadata(extractor).logic?.fieldPath).toBe(field);
            expect(graph.connections.some(e => e.fromNodeId === skill.id && e.toNodeId === extractor.id)).toBe(true);
        }
        const graph = build("precursor");
        const join = graph.nodes.find(n => metadata(n).logic?.operator === "join")!;
        expect(metadata(join).logic).toMatchObject({ joinMode: "left", joinKeys: [{ left: "region_code", right: "region_code" }, { left: "related_substance", right: "substance_name" }, { left: "period", right: "period" }] });
        expect(graph.connections.filter(e => e.toNodeId === join.id).map(e => e.toHandleId).sort()).toEqual(["input_a", "input_b"]);
    });

    test("append retains existing objects/edges and positions; repeated insertions have stable collision-free IDs", () => {
        const existing = createCanvasNode(CanvasNodeType.Text, { x: 500, y: 600 }, { content: "user data" });
        const second = createCanvasNode(CanvasNodeType.Markdown, { x: 900, y: 600 });
        const edge = { id: "user-edge", fromNodeId: existing.id, toNodeId: second.id };
        const before = JSON.stringify([existing, second, edge]);
        const first = build("wastewater", { nodes: [existing, second], connections: [edge] });
        const next = build("wastewater", { nodes: first.nodes, connections: first.connections });
        expect(next.nodes[0]).toBe(existing); expect(next.nodes[1]).toBe(second); expect(next.connections[0]).toBe(edge);
        expect(JSON.stringify([existing, second, edge])).toBe(before);
        expect(new Set([...next.nodes, ...next.connections].map(item => item.id)).size).toBe(next.nodes.length + next.connections.length);
        expect(next.nodes.slice(0, first.nodes.length).map(n => n.id)).toEqual(first.nodes.map(n => n.id));
        expect(first.nodes.slice(2).every(n => n.position.x >= second.position.x + second.width + 120)).toBe(true);
    });

    test("pins come only from exact installed records, not catalog/default hashes", () => {
        const graph = build("nps");
        const node = graph.nodes.find(n => n.type === CanvasNodeType.Skill)!;
        expect(metadata(node)).toMatchObject({ skillId: installed[0].skillId, skillVersion: installed[0].version, skillHash: installed[0].hash });
        expect(graph.handoff.skillReferences[0]).toMatchObject({ source: "installed-library", installationId: installed[0].id, workspaceId: "fixture-workspace" });
        const changed = installed.map(s => ({ ...s, version: "next", hash: "a".repeat(64) }));
        build("nps", { installedSkills: changed });
        expect(metadata(node).skillVersion).toBe("fixture-version-7");
        for (const overrides of [{ installedSkills: [] }, { workspaceId: "" }, { installedSkills: [{ ...installed[0], hash: "" }] }, { installedSkills: [{ ...installed[0], packId: "wrong" }] }]) {
            const missing = build("nps", overrides);
            const skill = missing.nodes.find(n => n.type === CanvasNodeType.Skill)!;
            expect(metadata(skill).skillHash).toBeUndefined();
            expect(metadata(skill).skillVersion).toBeUndefined();
            expect(skill.title).toContain("需安装");
            expect(missing.handoff.blockers).toContain("skill-installation-required");
        }
    });

    test("wastewater fixture evaluates with explicit dedupe keys and retains factor provenance through aggregate", () => {
        const graph = build("wastewater");
        const rank = graph.nodes.find(n => metadata(n).logic?.operator === "sort_limit")!;
        expect(metadata(rank).logic?.dedupeKeys).toEqual(["region_code", "substance_name", "period", "excretion_factor_source.version"]);
        const result = evaluateCanvasDataflow(graph.nodes, graph.connections);
        for (const node of graph.nodes.filter(n => n.type === CanvasNodeType.Logic)) expect(result.nodes.get(node.id)?.error).toBeUndefined();
        const rows = result.nodes.get(rank.id)!.outputItems as Record<string, unknown>[];
        expect(rows.map(row => row.region_code)).toEqual(["fixture-region-C", "fixture-region-B", "fixture-region-A"]);
        expect(rows[0]).toMatchObject({ consumption: 32.2, change_pct: null, change_pct_status: "missing_baseline", period_role: "current" });
        expect(rows[1]).toMatchObject({ change_pct: null, change_pct_status: "zero_baseline" });
        expect(rows[2]).toMatchObject({ change_pct: 15, change_pct_status: "ok" });
        expect(rows[0].metadata).toMatchObject({ fixture: true, publishable: false, excretion_factor_source: { version: "fixture-v1", citations: [{ evidence_id: "fixture-not-real-evidence", revision: 1 }] } });
    });

    test("handoff never runs locally and blocks fixture publication and unintegrated managed execution", () => {
        for (const id of ids) {
            const graph = build(id);
            expect(graph.handoff).toMatchObject({ status: "pending-integration", dependencies: ["T12", "T17"], approvalRequired: true, canRun: false, publishable: false, fixture: true });
            expect(graph.handoff.nodeIds).toEqual(graph.nodes.map(n => n.id));
            expect(graph.handoff.blockers).toContain("managed-run-not-connected");
            expect(graph.handoff.blockers).toContain("fixture-not-publishable");
        }
    });

    test("real project exposes blueprint empty-state commands, append menu and installed workspace selector", () => {
        const page = source("pages/canvas/project.tsx");
        expect(page).toContain("appendIndustryBlueprint(");
        expect(page).toContain("...industryBlueprintCommands");
        expect(page).toContain("行业蓝图（示例）");
        expect(page).toContain("listIndustrySkills(industryWorkspaceId)");
        expect(page).toContain("受管试跑待接入 T12/T17");
        const module = existsSync(modulePath) ? readFileSync(modulePath, "utf8") : "";
        expect(module).toContain("createCanvasNode("); expect(module).toContain("normalizeConnection("); expect(module).toContain("layoutCanvasFlow(");
        expect(module).not.toMatch(/callTool\(|fetch\(|http\.post|runWorkflow\(/);
    });
});

describe("T20 blueprint input/version/period defects", () => {
    const skillInputRows = (graph: ReturnType<typeof build>) => {
        const skill = graph.nodes.find(n => n.type === CanvasNodeType.Skill)!;
        const upstream = graph.nodes.find(n => graph.connections.some(e => e.fromNodeId === n.id && e.toNodeId === skill.id))!;
        return { skill, upstream, rows: JSON.parse(metadata(upstream).dataSource!.rawText!) as Record<string, any>[] };
    };

    test("nps skill input object carries all six required fields with full record shapes", () => {
        const { rows } = skillInputRows(build("nps"));
        expect(rows).toHaveLength(1);
        const input = rows[0];
        for (const key of ["period", "material_scope", "coverage", "detection_records", "traditional_metrics", "control_materials"]) {
            expect(input[key]).toBeDefined();
        }
        expect(input.period).toMatchObject({ start: expect.any(String), end: expect.any(String), granularity: expect.any(String) });
        expect(input.material_scope.document_ids?.length || input.material_scope.source_ids?.length).toBeTruthy();
        for (const key of ["covered_regions", "uncovered_regions", "failed_sources", "truncated_evidence_ids", "limitations"]) {
            expect(Array.isArray(input.coverage[key])).toBe(true);
        }
        expect(input.detection_records.length).toBeGreaterThan(0);
        for (const record of input.detection_records) {
            for (const key of ["substance_name", "region_code", "detected_at", "category", "co_use_features", "citations"]) expect(record[key]).toBeDefined();
        }
        for (const metric of input.traditional_metrics) {
            for (const key of ["region_code", "substance_name", "period", "value", "unit", "citations"]) expect(metric[key]).toBeDefined();
        }
        for (const material of input.control_materials) {
            for (const key of ["substance_name", "value", "jurisdiction", "effective_from", "material_source"]) expect(material[key]).toBeDefined();
        }
    });

    test("precursor skill input carries event/observed times and association_config", () => {
        const { rows } = skillInputRows(build("precursor"));
        expect(rows).toHaveLength(1);
        const input = rows[0];
        for (const key of ["period", "material_scope", "coverage", "monitoring_records", "precursor_records", "association_config"]) {
            expect(input[key]).toBeDefined();
        }
        expect(input.monitoring_records.length).toBeGreaterThan(0);
        for (const record of input.monitoring_records) {
            for (const key of ["region_code", "substance_name", "observed_at", "citations"]) expect(record[key]).toBeDefined();
        }
        for (const record of input.precursor_records) {
            for (const key of ["region_code", "precursor_name", "related_substance", "event_at", "quantity", "quantity_unit", "anomaly_type", "citations"]) expect(record[key]).toBeDefined();
        }
        expect(input.association_config).toMatchObject({ version: expect.any(String), max_lag_days: expect.any(Number), citations: expect.any(Array) });
        // 双流 join 的两条输入流本身也携带真实记录字段（事件/观测时间），不是扁平占位。
        const graph = build("precursor");
        const join = graph.nodes.find(n => metadata(n).logic?.operator === "join")!;
        for (const handle of ["input_a", "input_b"] as const) {
            const feeder = graph.nodes.find(n => graph.connections.some(e => e.fromNodeId === n.id && e.toNodeId === join.id && e.toHandleId === handle))!;
            const feedRows = JSON.parse(metadata(feeder).dataSource!.rawText!);
            expect(feedRows.length).toBeGreaterThan(0);
        }
        const leftFeeder = graph.nodes.find(n => graph.connections.some(e => e.fromNodeId === n.id && e.toNodeId === join.id && e.toHandleId === "input_a"))!;
        expect(JSON.parse(metadata(leftFeeder).dataSource!.rawText!)[0].event_at).toBeDefined();
        const rightFeeder = graph.nodes.find(n => graph.connections.some(e => e.fromNodeId === n.id && e.toNodeId === join.id && e.toHandleId === "input_b"))!;
        expect(JSON.parse(metadata(rightFeeder).dataSource!.rawText!)[0].observed_at).toBeDefined();
    });

    test("wastewater aggregate isolates factor versions and drops marker group keys", () => {
        const graph = build("wastewater");
        const aggregate = graph.nodes.find(n => metadata(n).logic?.operator === "aggregate")!;
        const groupBy = metadata(aggregate).logic!.groupBy!;
        expect(groupBy).toContain("excretion_factor_source.version");
        expect(groupBy).not.toContain("fixture");
        expect(groupBy).not.toContain("publishable");
        // 混版本输入不得平均成一个 ok 变化率：无同版本基线的行必须保持 missing_baseline。
        const measurements = graph.nodes.find(n => metadata(n).dataSource && JSON.parse(metadata(n).dataSource!.rawText!)[0].concentration_ng_l !== undefined)!;
        const cloned = graph.nodes.map(node => node === measurements
            ? { ...node, metadata: { ...node.metadata!, dataSource: { ...metadata(node).dataSource!, rawText: JSON.stringify([
                { region_code: "fixture-region-A", substance_name: "fixture-substance", period: "2026-02", baseline_period: "2026-01", period_role: "current", concentration_ng_l: 50, flow_m3_day: 100000, population: 500000, excretion_factor: 2.3, excretion_factor_source: { version: "fixture-v1", citations: [{ evidence_id: "fixture-not-real-evidence", revision: 1 }] } },
                { region_code: "fixture-region-A", substance_name: "fixture-substance", period: "2026-02", baseline_period: "2026-01", period_role: "current", concentration_ng_l: 50, flow_m3_day: 100000, population: 500000, excretion_factor: 4.6, excretion_factor_source: { version: "fixture-v2", citations: [{ evidence_id: "fixture-not-real-evidence", revision: 1 }] } },
            ]) } } }
            : node);
        const result = evaluateCanvasDataflow(cloned, graph.connections);
        // 版本在聚合处隔离：两行独立保留（23 与 46），不平均成 34.5 的假 ok。
        const aggregateNode = graph.nodes.find(n => metadata(n).logic?.operator === "aggregate")!;
        const aggregated = result.nodes.get(aggregateNode.id)!.outputItems as Record<string, unknown>[];
        expect(aggregated.map(row => row["excretion_factor_source.version"]).sort()).toEqual(["fixture-v1", "fixture-v2"]);
        expect(aggregated.find(row => row["excretion_factor_source.version"] === "fixture-v1")).toMatchObject({ consumption: 23 });
        expect(aggregated.find(row => row["excretion_factor_source.version"] === "fixture-v2")).toMatchObject({ consumption: 46 });
        // 混版本同周期进入对比节点时 fail-closed（重复基线错误），绝不给出平均后的 ok 变化率。
        const comparisonNode = graph.nodes.find(n => metadata(n).logic?.periodComparison)!;
        expect(result.nodes.get(comparisonNode.id)?.error).toContain("重复区域/物质/周期基线");
    });

    test("no hardcoded current period; ranking uses a data-driven period role", () => {
        for (const id of ids) {
            for (const node of build(id).nodes) {
                const logic = metadata(node).logic;
                if (logic) expect(JSON.stringify(logic)).not.toContain("2026-");
            }
        }
        const graph = build("wastewater");
        const current = graph.nodes.find(n => metadata(n).logic?.operator === "filter")!;
        expect(metadata(current).logic).toMatchObject({ fieldPath: "period_role", comparator: "equals", compareValue: "current" });
        // 换期后（period=2026-07）排名仍只包含当期行：直接用求值器验证数据驱动行为。
        const result = evaluateCanvasDataflow(graph.nodes, graph.connections);
        const rank = graph.nodes.find(n => metadata(n).logic?.operator === "sort_limit")!;
        for (const row of result.nodes.get(rank.id)!.outputItems as Record<string, unknown>[]) {
            expect(row.period_role).toBe("current");
        }
    });
});
