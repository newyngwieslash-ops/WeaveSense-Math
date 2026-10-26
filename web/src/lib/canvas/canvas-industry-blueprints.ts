import { createCanvasNode, normalizeConnection } from "@/lib/canvas/canvas-project-domain";
import { layoutCanvasFlow } from "@/lib/canvas/canvas-layout";
import type { SkillInstall } from "@/services/api/intelligence";
import { CanvasNodeType, type CanvasConnection, type CanvasLogicConfig, type CanvasNodeData, type CanvasNodeMetadata, type Position } from "@/types/canvas";

export const INDUSTRY_BLUEPRINTS = [
    { id: "wastewater", label: "污水折算与区域排名" },
    { id: "nps", label: "NPS 情报聚合" },
    { id: "precursor", label: "前体双流关联" },
] as const;
export type IndustryBlueprintId = typeof INDUSTRY_BLUEPRINTS[number]["id"];

export type IndustrySkillReference = {
    skillId: string;
    skillVersion: string;
    skillHash: string;
    source: "installed-library";
    installationId: string;
    workspaceId: string;
};

/** T12/T17 consume this draft only after validating scope, pins, inputs and approval. No execution callback is exposed. */
export type IndustryBlueprintHandoff = {
    blueprintId: IndustryBlueprintId;
    instanceId: string;
    workspaceId?: string;
    nodeIds: string[];
    connectionIds: string[];
    skillReferences: IndustrySkillReference[];
    requiredSkillIds: string[];
    status: "pending-integration";
    dependencies: ["T12", "T17"];
    approvalRequired: true;
    fixture: true;
    publishable: false;
    canRun: false;
    blockers: string[];
};

type BlueprintMarker = {
    blueprintId: IndustryBlueprintId;
    instanceId: string;
    fixture: true;
    publishable: false;
    managedRun: "pending-integration";
};

// The shared Skill editor still uses numeric snapshot versions; industry pins are semver strings.
// Keep this wire contract local until T12/T17 integrate it; never coerce the installed version to a number.
export type IndustryBlueprintMetadata = Omit<CanvasNodeMetadata, "skillVersion" | "workflowParameters"> & {
    skillVersion?: string;
    skillHash?: string;
    xKey?: string;
    series?: string[];
    workflowParameters: {
        industryBlueprint: BlueprintMarker;
        industryHandoff?: IndustryBlueprintHandoff;
    };
};

const WARNING = "示例 fixture，非真实数据/证据，不可发布报告。受管试跑待接入 T12/T17；须验证工作区、固定 Skill 引用、真实证据与审批，禁止本地绕审批运行。";
const PACK_ID = "substance-precursor-intel";
const REQUIRED_SKILLS: Partial<Record<IndustryBlueprintId, string>> = {
    nps: "nps-substitution-screening",
    precursor: "precursor-substance-correlation",
};

export function appendIndustryBlueprint(input: {
    blueprintId: IndustryBlueprintId;
    nodes: CanvasNodeData[];
    connections: CanvasConnection[];
    workspaceId?: string;
    installedSkills?: readonly SkillInstall[];
    origin?: Position;
}): { nodes: CanvasNodeData[]; connections: CanvasConnection[]; handoff: IndustryBlueprintHandoff } {
    const { blueprintId } = input;
    if (!INDUSTRY_BLUEPRINTS.some(item => item.id === blueprintId)) throw new Error("未知行业蓝图");
    const occupied = new Set([...input.nodes, ...input.connections].map(item => item.id));
    let sequence = 1;
    while ([...occupied].some(id => id.startsWith(`industry-${blueprintId}-${sequence}/`))) sequence++;
    const instanceId = `industry-${blueprintId}-${sequence}`;
    const origin = input.nodes.length
        ? { x: Math.max(...input.nodes.map(node => node.position.x + node.width)) + 120, y: Math.min(...input.nodes.map(node => node.position.y)) }
        : input.origin ?? { x: 0, y: 0 };
    const nodes: CanvasNodeData[] = [];
    const connections: CanvasConnection[] = [];
    const refs: IndustrySkillReference[] = [];
    const marker: BlueprintMarker = { blueprintId, instanceId, fixture: true, publishable: false, managedRun: "pending-integration" };

    function add(key: string, type: CanvasNodeType, title: string, patch: Omit<Partial<IndustryBlueprintMetadata>, "workflowParameters"> = {}) {
        const metadata: IndustryBlueprintMetadata = { ...patch, workflowDescription: WARNING, workflowParameters: { industryBlueprint: { ...marker } } };
        const node = createCanvasNode(type, origin, metadata as unknown as CanvasNodeMetadata);
        node.id = `${instanceId}/node/${key}`;
        node.title = `示例 · ${title}`;
        node.position = { ...origin };
        nodes.push(node);
        return node;
    }
    function source(key: string, title: string, rows: Record<string, unknown>[], format: "json" | "intel" = "json") {
        return add(key, CanvasNodeType.DataSource, title, {
            dataSource: { format, rawText: JSON.stringify(rows.map(row => ({
                ...row, fixture: true, publishable: false,
                metadata: { fixture: true, publishable: false, ...(row.excretion_factor_source ? { excretion_factor_source: row.excretion_factor_source } : {}) },
            })), null, 2), sourceBinding: { kind: "static" } },
        });
    }
    function logic(key: string, title: string, config: CanvasLogicConfig) {
        return add(key, CanvasNodeType.Logic, title, { logic: config });
    }
    function connect(from: CanvasNodeData, to: CanvasNodeData, toHandleId?: "input_a" | "input_b") {
        const direction = normalizeConnection(from.id, to.id, nodes, "source");
        if (!direction) throw new Error("蓝图连线非法");
        connections.push({ id: `${instanceId}/edge/${connections.length + 1}`, ...direction, ...(toHandleId ? { toHandleId } : {}) });
    }
    function skill() {
        const skillId = REQUIRED_SKILLS[blueprintId]!;
        const matches = input.workspaceId?.trim() ? (input.installedSkills ?? []).filter(row => row.skillId === skillId && row.packId === PACK_ID) : [];
        const installation = matches.length === 1 ? matches[0] : undefined;
        const pin = installation && installation.id.trim() && installation.version.trim() && /^[a-fA-F0-9]{64}$/.test(installation.hash)
            ? { skillId: installation.skillId, skillVersion: installation.version, skillHash: installation.hash, source: "installed-library" as const, installationId: installation.id, workspaceId: input.workspaceId! }
            : undefined;
        if (pin) refs.push(pin);
        return add("skill", CanvasNodeType.Skill, `${skillId}${pin ? " · 固定安装版本" : " · 需安装，禁止受管试跑"}`, {
            ...(pin ? { skillId: pin.skillId, skillVersion: pin.skillVersion, skillHash: pin.skillHash } : {}),
            skillName: skillId,
            skillDescription: WARNING,
            skillPrompts: [WARNING, "不得补造证据、因子、列管结论或风险阈值；保留缺口，输出安装版本定义的结构。Join 仅表示键相同，不证明关联或因果。"],
        });
    }
    function outputs(upstream: CanvasNodeData, xKey: string, series: string[], note: string) {
        const chart = add("chart", CanvasNodeType.Chart, "图表（待受管结果）", { chartKind: "bar", xKey, series });
        const markdown = add("markdown", CanvasNodeType.Markdown, "说明草稿 · 不可发布", { content: `# 行业蓝图示例\n\n${WARNING}\n\n${note}` });
        connect(upstream, chart); connect(upstream, markdown);
        return markdown;
    }

    const fixturePeriod = { start: "2026-02-01", end: "2026-02-28", granularity: "month" };
    const fixtureCoverage = (regions: string[]) => ({ covered_regions: regions, uncovered_regions: [], failed_sources: [], truncated_evidence_ids: [], limitations: ["静态 fixture，无真实证据覆盖"] });
    const fixtureCitation = [{ evidence_id: "fixture-not-real-evidence", revision: 1 }];
    // Skill 输入是严格行业根 schema 对象：单行 JSON 源经连线成为该节点的唯一输入对象。
    function skillInput(key: string, title: string, input: Record<string, unknown>) {
        return source(key, title, [input]);
    }

    let report: CanvasNodeData;
    if (blueprintId === "wastewater") {
        const measurements = source("measurements", "污水监测台账（因子仅 fixture）", ["A", "B", "C"].map((region, i) => ({
            region_code: `fixture-region-${region}`, substance_name: "fixture-substance", period: "2026-02", baseline_period: "2026-01", period_role: "current",
            concentration_ng_l: 50 + i * 10, flow_m3_day: 100000, population: 500000,
            excretion_factor: 2.3,
            excretion_factor_source: { version: "fixture-v1", citations: fixtureCitation },
        })));
        const baseline = source("baseline", "基线输入（B 零基线，C 缺失；须与当期因子同版本）", [
            { region_code: "fixture-region-A", substance_name: "fixture-substance", period: "2026-01", baseline_period: "2025-12", period_role: "baseline", consumption: 20, excretion_factor_source: { version: "fixture-v1", citations: fixtureCitation } },
            { region_code: "fixture-region-B", substance_name: "fixture-substance", period: "2026-01", baseline_period: "2025-12", period_role: "baseline", consumption: 0, excretion_factor_source: { version: "fixture-v1", citations: fixtureCitation } },
        ]);
        const compute = logic("compute", "显式污水折算", { operator: "compute", expression: "concentration_ng_l * flow_m3_day * excretion_factor / population", outputField: "consumption" });
        const aggregate = logic("aggregate", "区域/物质/周期/因子版本聚合", {
            operator: "aggregate", groupBy: ["region_code", "substance_name", "period", "baseline_period", "period_role", "excretion_factor_source.version"],
            metrics: [{ field: "consumption", fn: "avg", as: "consumption" }],
        });
        const comparison = logic("comparison", "显式周期对比（缺基线不补零）", {
            operator: "compute", outputField: "change_pct",
            periodComparison: { regionField: "region_code", substanceField: "substance_name", periodField: "period", baselinePeriodField: "baseline_period", valueField: "consumption" },
        });
        const current = logic("current", "仅当期行参与排名（数据驱动，无隐藏日期）", { operator: "filter", fieldPath: "period_role", comparator: "equals", compareValue: "current" });
        const rank = logic("rank", "折算值排名（非风险等级）", { operator: "sort_limit", sortField: "consumption", sortOrder: "desc", limit: 10, dedupeKeys: ["region_code", "substance_name", "period", "excretion_factor_source.version"] });
        connect(measurements, compute); connect(compute, aggregate); connect(aggregate, comparison); connect(baseline, comparison);
        connect(comparison, current); connect(current, rank);
        report = outputs(rank, "region_code", ["consumption"], "折算单位 mg/1000人/日；因子 2.3 仅为显式 fixture，不是默认值。上线须替换全部样例及因子版本/真实引用。聚合按 excretion_factor_source.version 隔离，不同版本不平均；基线必须与当期因子同版本，跨版本对比不可比。change_pct 缺失基线为 null + missing_baseline，零基线为 null + zero_baseline；当期行由数据字段 period_role 标记，不硬编码周期值。图表仅展示折算值，不能把 null 画为 0。区域内均值仅演示聚合，不代表人口加权估计。");
    } else if (blueprintId === "nps") {
        const input = skillInput("input", "NPS 筛查输入对象（六必填，需替换为真实证据）", {
            period: fixturePeriod,
            material_scope: { document_ids: ["fixture-document"] },
            coverage: fixtureCoverage(["fixture-region"]),
            detection_records: [{ substance_name: "fixture-substance", region_code: "fixture-region", detected_at: "2026-02-10", category: "fixture-category", co_use_features: ["fixture-feature"], citations: fixtureCitation }],
            traditional_metrics: [{ region_code: "fixture-region", substance_name: "fixture-substance", period: fixturePeriod, value: 1.2, unit: "fixture-unit", citations: fixtureCitation }],
            control_materials: [{ substance_name: "fixture-substance", value: "fixture-controlled", jurisdiction: "fixture-jurisdiction", effective_from: "2026-01-01", material_source: { version: "fixture-v1", citations: fixtureCitation } }],
        });
        const method = skill();
        const extract = logic("extract", "提取 Skill 结构数组", { operator: "json_extract", fieldPath: "emerging_substances" });
        const aggregate = logic("aggregate", "物质/类别条目聚合（非风险分数）", { operator: "aggregate", groupBy: ["substance_name", "category"], metrics: [{ field: "substance_name", fn: "count", as: "record_count" }] });
        connect(input, method); connect(method, extract); connect(extract, aggregate);
        report = outputs(aggregate, "substance_name", ["record_count"], "安装 nps-substitution-screening 后固定其实际 version/hash。Skill 输入是严格行业根 schema 对象（period/material_scope/coverage/detection_records/traditional_metrics/control_materials 六必填），单行输入源经连线组装，不拿任意行当输入。emerging_substances 按物质与类别统计条目数，不推定来源独立、替代关系或法律状态。Skill 尚未执行。");
    } else {
        const precursor = source("precursor", "前体事件流（真实记录字段，静态 fixture）", [{ region_code: "fixture-region", precursor_name: "fixture-precursor", related_substance: "fixture-substance", period: "2026-02", event_at: "2026-02-03T08:00:00Z", quantity: 120, quantity_unit: "kg", anomaly_type: "fixture-anomaly", citations: fixtureCitation }], "intel");
        const monitoring = source("monitoring", "检测情报流（真实记录字段，静态 fixture）", [{ region_code: "fixture-region", substance_name: "fixture-substance", period: "2026-02", observed_at: "2026-02-10T00:00:00Z", concentration_ng_l: 50, citations: fixtureCitation }], "intel");
        const join = logic("join", "区域/物质/周期复合关联（共现视图，非因果）", { operator: "join", joinMode: "left", joinKeys: [{ left: "region_code", right: "region_code" }, { left: "related_substance", right: "substance_name" }, { left: "period", right: "period" }] });
        const coChart = add("co-chart", CanvasNodeType.Chart, "共现图表（匹配≠因果）", { chartKind: "bar", xKey: "region_code", series: ["concentration_ng_l"] });
        const input = skillInput("input", "前体关联输入对象（六必填含 association_config）", {
            period: fixturePeriod,
            material_scope: { document_ids: ["fixture-document"] },
            coverage: fixtureCoverage(["fixture-region"]),
            monitoring_records: [{ region_code: "fixture-region", substance_name: "fixture-substance", observed_at: "2026-02-10T00:00:00Z", concentration_ng_l: 50, citations: fixtureCitation }],
            precursor_records: [{ region_code: "fixture-region", precursor_name: "fixture-precursor", related_substance: "fixture-substance", event_at: "2026-02-03T08:00:00Z", quantity: 120, quantity_unit: "kg", anomaly_type: "fixture-anomaly", citations: fixtureCitation }],
            association_config: { version: "fixture-v1", max_lag_days: 30, citations: fixtureCitation },
        });
        const method = skill();
        const extract = logic("extract", "提取 Skill 关联数组", { operator: "json_extract", fieldPath: "correlations" });
        const rank = logic("rank", "时间间隔排名（非风险分数）", { operator: "sort_limit", sortField: "lag_days", sortOrder: "desc", limit: 10, dedupeKeys: [] });
        connect(precursor, join, "input_a"); connect(monitoring, join, "input_b"); connect(join, coChart);
        connect(input, method); connect(method, extract); connect(extract, rank);
        report = outputs(rank, "region_code", ["lag_days"], "Join 左表前体（含 event_at/quantity/anomaly_type）、右表检测（含 observed_at），按区域/物质/周期匹配；匹配不是因果或共现证据。Skill 输入是严格行业根 schema 对象（monitoring_records/precursor_records/association_config 等六必填），单行输入源经连线组装。按 lag_days 排序，不制造 risk_score 或风险阈值。Skill 尚未执行。");
    }

    const requiredSkillIds = REQUIRED_SKILLS[blueprintId] ? [REQUIRED_SKILLS[blueprintId]!] : [];
    const handoff: IndustryBlueprintHandoff = {
        blueprintId, instanceId, ...(input.workspaceId?.trim() ? { workspaceId: input.workspaceId } : {}),
        nodeIds: nodes.map(node => node.id), connectionIds: connections.map(edge => edge.id), skillReferences: refs, requiredSkillIds,
        status: "pending-integration", dependencies: ["T12", "T17"], approvalRequired: true, fixture: true, publishable: false, canRun: false,
        blockers: ["managed-run-not-connected", "fixture-not-publishable", ...(requiredSkillIds.length && !refs.length ? ["skill-installation-required"] : [])],
    };
    report.metadata!.workflowParameters = { industryBlueprint: { ...marker }, industryHandoff: handoff };
    const positions = layoutCanvasFlow(nodes, connections);
    return { nodes: [...input.nodes, ...nodes.map(node => ({ ...node, position: positions.get(node.id) ?? node.position }))], connections: [...input.connections, ...connections], handoff };
}
