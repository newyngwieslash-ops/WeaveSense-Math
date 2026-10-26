import {
    CanvasNodeType,
    type CanvasConnection,
    type CanvasDataSourceConfig,
    type CanvasDataSourceFormat,
    type CanvasLogicComparator,
    type CanvasLogicConfig,
    type CanvasLogicOperator,
    type CanvasNodeData,
} from "@/types/canvas";

export type CanvasDataItem = Record<string, unknown> | string | number | boolean;

export type DataflowNodeResult = {
    nodeId: string;
    format?: CanvasDataSourceFormat;
    operator?: CanvasLogicOperator;
    inputA: CanvasDataItem[];
    inputB: CanvasDataItem[];
    outputItems: CanvasDataItem[];
    trueItems: CanvasDataItem[];
    falseItems: CanvasDataItem[];
    detectedKeys: string[];
    error?: string;
    cycleDetected?: boolean;
};

export type CanvasDataflowEvaluation = {
    nodes: Map<string, DataflowNodeResult>;
    getEdgeItems: (
        fromNodeId: string,
        fromHandleId?: string,
    ) => {
        items: CanvasDataItem[];
        active: boolean;
        isConditionalBranch: boolean;
        branchLabel?: string;
    };
    getEdgeText: (fromNodeId: string, fromHandleId?: string) => string;
};

export const LOGIC_PORT_TOP_RATIO = 0.38;
export const LOGIC_PORT_BOTTOM_RATIO = 0.72;

export const DATA_SOURCE_FORMAT_LABELS: Record<CanvasDataSourceFormat, string> = {
    json: "JSON",
    csv: "CSV",
    txt: "TXT",
    markdown: "Markdown",
    intel: "情报集",
};

export const LOGIC_OPERATOR_LABELS: Record<CanvasLogicOperator, string> = {
    if_else: "If / Else 分支",
    intersect: "交集 (A ∩ B)",
    union: "并集 (A ∪ B)",
    difference: "差集 (A − B)",
    filter: "条件过滤",
    json_extract: "字段提取",
    compute: "指标计算",
    aggregate: "分组聚合",
    join: "关联拼接 (Join)",
    sort_limit: "排序与 Top-N",
};

export const LOGIC_COMPARATOR_LABELS: Record<CanvasLogicComparator, string> = {
    contains: "包含",
    not_contains: "不包含",
    equals: "等于 (=)",
    not_equals: "不等于 (≠)",
    gt: "大于 (>)",
    lt: "小于 (<)",
    not_empty: "非空",
    is_empty: "为空",
    regex: "正则匹配",
};

export function defaultDataSourceConfig(): CanvasDataSourceConfig {
    return {
        format: "json",
        rawText: '[\n  { "id": "1", "title": "示例条目 A", "score": 90 },\n  { "id": "2", "title": "示例条目 B", "score": 65 }\n]',
    };
}

export function defaultLogicConfig(): CanvasLogicConfig {
    return {
        operator: "if_else",
        fieldPath: "score",
        comparator: "gt",
        compareValue: "80",
        matchKey: "id",
    };
}

export function readNodeLogicConfig(node: CanvasNodeData): CanvasLogicConfig {
    const raw = node.metadata?.logic;
    return {
        operator: raw?.operator || "if_else",
        fieldPath: raw?.fieldPath ?? "",
        comparator: raw?.comparator || "contains",
        compareValue: raw?.compareValue ?? "",
        matchKey: raw?.matchKey ?? "",
        expression: raw?.expression ?? "",
        ...(raw && Object.prototype.hasOwnProperty.call(raw, "periodComparison") ? { periodComparison: raw.periodComparison } : {}),
        outputField: raw?.outputField ?? "",
        groupBy: raw?.groupBy === undefined ? [] : raw.groupBy,
        metrics: raw?.metrics === undefined ? [{ field: "", fn: "count", as: "count" }] : raw.metrics,
        joinMode: raw?.joinMode === undefined ? "inner" : raw.joinMode,
        joinKeys: raw?.joinKeys === undefined ? [{ left: "id", right: "id" }] : raw.joinKeys,
        dedupeKeys: raw?.dedupeKeys === undefined ? [] : raw.dedupeKeys,
        sortField: raw?.sortField === undefined ? "" : raw.sortField,
        sortOrder: raw?.sortOrder === undefined ? "desc" : raw.sortOrder,
        limit: typeof raw?.limit === "number" ? raw.limit : 10,
    };
}

export function isLogicDualInput(node: CanvasNodeData): boolean {
    if (node.type !== CanvasNodeType.Logic) return false;
    const op = readNodeLogicConfig(node).operator;
    return op === "intersect" || op === "union" || op === "difference" || op === "join";
}

export function isLogicDualOutput(node: CanvasNodeData): boolean {
    if (node.type !== CanvasNodeType.Logic) return false;
    return readNodeLogicConfig(node).operator === "if_else";
}

export function logicNodeHandleY(node: CanvasNodeData, handleId?: string): number | undefined {
    if (node.type !== CanvasNodeType.Logic) return undefined;
    if (handleId === "input_a" || handleId === "true") {
        return node.position.y + node.height * LOGIC_PORT_TOP_RATIO;
    }
    if (handleId === "input_b" || handleId === "false") {
        return node.position.y + node.height * LOGIC_PORT_BOTTOM_RATIO;
    }
    return undefined;
}

export function logicNodeHandleAtY(node: CanvasNodeData, side: "source" | "target", worldY: number, hitRadius = 48): string | undefined {
    if (node.type !== CanvasNodeType.Logic) return undefined;
    const isDual = side === "target" ? isLogicDualInput(node) : isLogicDualOutput(node);
    if (!isDual) return undefined;
    const topY = node.position.y + node.height * LOGIC_PORT_TOP_RATIO;
    const bottomY = node.position.y + node.height * LOGIC_PORT_BOTTOM_RATIO;
    const distTop = Math.abs(worldY - topY);
    const distBottom = Math.abs(worldY - bottomY);
    if (Math.min(distTop, distBottom) > hitRadius) return undefined;
    if (side === "target") {
        return distTop <= distBottom ? "input_a" : "input_b";
    }
    return distTop <= distBottom ? "true" : "false";
}

export function detectDataSourceFormatFromFilename(fileName: string, content = ""): CanvasDataSourceFormat {
    const lower = fileName.trim().toLowerCase();
    if (lower.endsWith(".json") || lower.endsWith(".jsonl")) return "json";
    if (lower.endsWith(".csv") || lower.endsWith(".tsv")) return "csv";
    if (lower.endsWith(".md") || lower.endsWith(".markdown")) return "markdown";
    if (lower.endsWith(".txt")) return "txt";
    const trimmed = content.trim();
    if ((trimmed.startsWith("[") && trimmed.endsWith("]")) || (trimmed.startsWith("{") && trimmed.endsWith("}"))) {
        return "json";
    }
    return "txt";
}

function normalizeScalarValue(raw: string): string | number | boolean {
    if (raw !== raw.trim()) return raw;
    if (raw === "true") return true;
    if (raw === "false") return false;
    if (/^-?(?:0|[1-9]\d*)(?:\.\d+)?$/.test(raw)) {
        const value = Number(raw);
        if (Number.isFinite(value) && (!Number.isInteger(value) || Number.isSafeInteger(value))) return value;
    }
    return raw;
}

export function parseCsvItems(text: string, delimiter: "," | "	" = ","): CanvasDataItem[] {
    const source = text.replace(/^\uFEFF/, "").replace(/\r\n/g, "\n");
    const records: { cells: string[]; line: number }[] = [];
    let cells: string[] = [], cell = "", quoted = false, closedQuote = false, touched = false, line = 1, startLine = 1;
    const fail = () => { throw new Error(`CSV/TSV 解析失败：第 ${line} 行引号不合法`); };
    const finishCell = () => { cells.push(cell); cell = ""; closedQuote = false; };
    const finishRow = () => {
        if (touched || cells.length || cell) { finishCell(); records.push({ cells, line: startLine }); }
        cells = []; cell = ""; touched = false; closedQuote = false; startLine = line + 1;
    };
    for (let i = 0; i < source.length; i++) {
        const ch = source[i];
        if (quoted) {
            if (ch === '"') {
                if (source[i + 1] === '"') { cell += '"'; i++; }
                else { quoted = false; closedQuote = true; }
            } else { cell += ch; if (ch === "\n") line++; }
            continue;
        }
        if (ch === delimiter) { finishCell(); touched = true; continue; }
        if (ch === "\n") { finishRow(); line++; continue; }
        if (closedQuote) fail();
        if (ch === '"') { if (cell) fail(); quoted = true; touched = true; }
        else { cell += ch; touched = true; }
    }
    if (quoted) fail();
    finishRow();
    if (!records.length) return [];
    const headers = records[0].cells.map(header => header.trim());
    if (headers.some(header => !header) || new Set(headers).size !== headers.length) throw new Error(`CSV/TSV 第 ${records[0].line} 行表头为空或重复`);
    return records.slice(1).map(record => {
        if (record.cells.length !== headers.length) throw new Error(`CSV/TSV 第 ${record.line} 行列数不匹配`);
        return Object.fromEntries(headers.map((header, index) => [header, normalizeScalarValue(record.cells[index])]));
    });
}

export function parseMarkdownItems(text: string): CanvasDataItem[] {
    const trimmed = text.trim();
    if (!trimmed) return [];
    const lines = trimmed.split(/\r?\n/);
    const bulletItems = lines
        .map((line) => {
            const match = /^(?:[-*+]|\d+\.)\s+(.+)$/.exec(line.trim());
            return match ? match[1].trim() : null;
        })
        .filter((item): item is string => Boolean(item));
    if (bulletItems.length >= 2) {
        return bulletItems.map((item, idx) => ({ id: String(idx + 1), text: item, title: item }));
    }

    const sections: CanvasDataItem[] = [];
    let currentTitle = "";
    let currentLines: string[] = [];
    const flushSection = () => {
        const body = currentLines.join("\n").trim();
        if (currentTitle || body) {
            sections.push({
                id: String(sections.length + 1),
                title: currentTitle || body.slice(0, 40),
                content: body || currentTitle,
            });
        }
    };
    for (const line of lines) {
        const headingMatch = /^#{1,4}\s+(.+)$/.exec(line.trim());
        if (headingMatch) {
            flushSection();
            currentTitle = headingMatch[1].trim();
            currentLines = [];
        } else {
            currentLines.push(line);
        }
    }
    flushSection();
    if (sections.length > 1 || currentTitle) return sections;

    return trimmed
        .split(/\r?\n\s*\r?\n/)
        .map((block) => block.trim())
        .filter(Boolean);
}

export function parseJsonItems(text: string): { items: CanvasDataItem[]; error?: string } {
    const trimmed = text.trim();
    if (!trimmed) return { items: [] };
    try {
        const parsed: unknown = JSON.parse(trimmed);
        assertFiniteJson(parsed);
        if (Array.isArray(parsed)) {
            return { items: parsed.map(toCanvasDataItem) };
        }
        if (parsed && typeof parsed === "object") {
            const record = parsed as Record<string, unknown>;
            for (const key of ["items", "rows", "data", "records", "list", "documents", "results"]) {
                if (Array.isArray(record[key])) {
                    return { items: (record[key] as unknown[]).map(toCanvasDataItem) };
                }
            }
            return { items: [record] };
        }
        return { items: [], error: "JSON 必须是对象或数组" };
    } catch (err) {
        return {
            items: [],
            error: err instanceof Error ? `JSON 解析失败：${err.message}` : "JSON 格式不合法",
        };
    }
}

function assertFiniteJson(value: unknown): void {
    if (typeof value === "number" && !Number.isFinite(value)) throw new Error("JSON 数值必须有限");
    if (value && typeof value === "object") for (const child of Object.values(value)) assertFiniteJson(child);
}

function toCanvasDataItem(value: unknown): CanvasDataItem {
    if (value === null || value === undefined) return "";
    if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return value;
    if (Array.isArray(value)) return { items: value, text: JSON.stringify(value) };
    return value as Record<string, unknown>;
}

export function parseDataSourceNode(node: CanvasNodeData): { items: CanvasDataItem[]; format: CanvasDataSourceFormat; error?: string } {
    const ds = node.metadata?.dataSource;
    const sourceText = ds?.rawText ?? node.metadata?.content ?? node.metadata?.prompt ?? "";
    const raw = sourceText.trim();
    const format: CanvasDataSourceFormat = ds?.format || detectDataSourceFormatFromFilename(ds?.fileName || "", raw);
    if (!raw) return { items: [], format };

    if (format === "json" && ds?.fileName?.trim().toLowerCase().endsWith(".jsonl")) {
        const items: CanvasDataItem[] = [];
        for (const [index, line] of sourceText.split(/\r?\n/).entries()) {
            if (!line.trim()) continue;
            try {
                const parsed: unknown = JSON.parse(line.trim());
                assertFiniteJson(parsed);
                items.push(toCanvasDataItem(parsed));
            } catch {
                return { items: [], format, error: `JSONL 解析失败：第 ${index + 1} 行不是有效 JSON` };
            }
        }
        return { items, format };
    }

    if (format === "json" || format === "intel") {
        const res = parseJsonItems(raw);
        return { items: res.items, format, error: res.error };
    }
    if (format === "csv") {
        const delimiter = ds?.fileName?.trim().toLowerCase().endsWith(".tsv") ? "	" : ",";
        try { return { items: parseCsvItems(sourceText, delimiter), format }; }
        catch (error) { return { items: [], format, error: error instanceof Error ? error.message : "CSV/TSV 解析失败" }; }
    }
    if (format === "markdown") {
        return { items: parseMarkdownItems(raw), format };
    }
    const lines = raw
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter(Boolean);
    return { items: lines, format };
}

export function formatSkillNodePrompt(node: CanvasNodeData): string {
    const skill = node.metadata?.skillSnapshot;
    if (skill) {
        return [
            `【技能：${skill.name}】`,
            skill.description ? `用途：${skill.description}` : "",
            `执行模板：\n${skill.template}`,
            skill.outputContract ? `输出约束：\n${skill.outputContract}` : "",
            "请严格执行该技能，只输出结果，不要输出解释性套话。",
        ]
            .filter(Boolean)
            .join("\n\n");
    }
    const name = (node.metadata?.skillName || "").trim();
    const description = (node.metadata?.skillDescription || "").trim();
    const prompts = (node.metadata?.skillPrompts || []).map((p) => String(p).trim()).filter(Boolean);
    const content = (node.metadata?.content || node.metadata?.prompt || "").trim();
    const body = content || prompts.join("\n");
    if (!name && !description) return body;
    return [
        `【技能：${name || node.title || "Skill 节点"}】`,
        description ? `用途：${description}` : "",
        body ? `执行模板：\n${body}` : "",
    ]
        .filter(Boolean)
        .join("\n\n");
}

export function extractNodeItems(node: CanvasNodeData): { items: CanvasDataItem[]; format?: CanvasDataSourceFormat; error?: string } {
    if (node.type === CanvasNodeType.DataSource) {
        return parseDataSourceNode(node);
    }
    if (node.type === CanvasNodeType.Skill) {
        const formatted = formatSkillNodePrompt(node);
        return { items: formatted ? [formatted] : [], format: "txt" };
    }
    if (node.type === CanvasNodeType.Script) {
        const rows = node.metadata?.storyboard?.rows || [];
        return {
            items: rows.map((row) => ({
                id: row.id,
                shotNumber: row.shotNumber,
                durationSeconds: row.durationSeconds,
                plotDescription: row.plotDescription,
                dialogue: row.dialogue,
                imageGenerationPrompt: row.imageGenerationPrompt,
                videoMotionPrompt: row.videoMotionPrompt,
            })),
        };
    }
    if (node.type === CanvasNodeType.BatchTable) {
        const rows = node.metadata?.batchTable?.rows || [];
        return {
            items: rows.map((row, idx) => ({
                id: row.id || String(idx + 1),
                prompt: row.prompt,
                enabled: row.enabled,
                ...(row.cells || {}),
            })),
        };
    }
    if (node.type === CanvasNodeType.Image || node.type === CanvasNodeType.Video || node.type === CanvasNodeType.Audio) {
        const url = node.metadata?.content || "";
        const storageKey = node.metadata?.storageKey || "";
        if (!url && !storageKey) return { items: [] };
        return {
            items: [
                {
                    id: node.id,
                    type: node.type,
                    title: node.title || node.id,
                    url,
                    storageKey,
                },
            ],
        };
    }
    if (node.type === CanvasNodeType.Markdown) {
        const raw = (node.metadata?.content || "").trim();
        return { items: parseMarkdownItems(raw), format: "markdown" };
    }
    const raw = (node.metadata?.content || node.metadata?.prompt || node.metadata?.skillSnapshot?.template || "").trim();
    if (!raw) return { items: [] };
    if ((raw.startsWith("[") && raw.endsWith("]")) || (raw.startsWith("{") && raw.endsWith("}"))) {
        const jsonRes = parseJsonItems(raw);
        if (!jsonRes.error && jsonRes.items.length > 0) {
            return { items: jsonRes.items, format: "json" };
        }
    }
    const lines = raw
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter(Boolean);
    return { items: lines.length > 1 ? lines : [raw], format: "txt" };
}

export function readDataItemField(item: CanvasDataItem, fieldPath?: string): unknown {
    const cleanPath = (fieldPath || "").trim();
    if (!cleanPath) {
        if (item && typeof item === "object") {
            const rec = item as Record<string, unknown>;
            for (const key of ["text", "content", "title", "name", "prompt", "value", "id"]) {
                if (rec[key] !== undefined && rec[key] !== null && String(rec[key]).trim() !== "") {
                    return rec[key];
                }
            }
            return JSON.stringify(item);
        }
        return item;
    }
    if (!item || typeof item !== "object") {
        return item;
    }
    const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === "object" && !Array.isArray(value) && (Object.getPrototypeOf(value) === Object.prototype || Object.getPrototypeOf(value) === null);
    if (!isRecord(item)) return undefined;
    if (Object.prototype.hasOwnProperty.call(item, cleanPath)) return item[cleanPath];
    const segments = cleanPath.split(".").map((s) => s.trim()).filter(Boolean);
    let cursor: unknown = item;
    for (const seg of segments) {
        if (!isRecord(cursor) || !Object.prototype.hasOwnProperty.call(cursor, seg)) return undefined;
        cursor = cursor[seg];
    }
    return cursor;
}

function itemScalarString(value: unknown): string {
    if (value === null || value === undefined) return "";
    if (typeof value === "string") return value;
    if (typeof value === "number" || typeof value === "boolean") return String(value);
    return JSON.stringify(value);
}

export function itemIdentityKey(item: CanvasDataItem, matchKey?: string): string {
    const cleanKey = (matchKey || "").trim();
    if (cleanKey && item && typeof item === "object") {
        const extracted = readDataItemField(item, cleanKey);
        return itemScalarString(extracted).trim();
    }
    if (item && typeof item === "object") {
        const rec = item as Record<string, unknown>;
        const sortedEntries = Object.keys(rec)
            .sort()
            .map((k) => [k, rec[k]]);
        return JSON.stringify(sortedEntries);
    }
    return String(item).trim();
}

export function evaluateLogicCondition(
    item: CanvasDataItem,
    comparator: CanvasLogicComparator = "contains",
    fieldPath?: string,
    compareValue = "",
): { matched: boolean; error?: string } {
    const rawValue = readDataItemField(item, fieldPath);
    const actualStr = itemScalarString(rawValue).trim();
    const targetStr = compareValue.trim();

    if (comparator === "is_empty") {
        const empty =
            rawValue === null ||
            rawValue === undefined ||
            actualStr === "" ||
            (Array.isArray(rawValue) && rawValue.length === 0);
        return { matched: empty };
    }
    if (comparator === "not_empty") {
        const empty =
            rawValue === null ||
            rawValue === undefined ||
            actualStr === "" ||
            (Array.isArray(rawValue) && rawValue.length === 0);
        return { matched: !empty };
    }
    if (comparator === "contains") {
        if (Array.isArray(rawValue)) {
            return { matched: rawValue.some((entry) => itemScalarString(entry).toLowerCase().includes(targetStr.toLowerCase())) };
        }
        return { matched: actualStr.toLowerCase().includes(targetStr.toLowerCase()) };
    }
    if (comparator === "not_contains") {
        const res = evaluateLogicCondition(item, "contains", fieldPath, compareValue);
        return { matched: !res.matched };
    }
    if (comparator === "equals") {
        const actualNum = Number(actualStr);
        const targetNum = Number(targetStr);
        if (actualStr !== "" && targetStr !== "" && Number.isFinite(actualNum) && Number.isFinite(targetNum)) {
            return { matched: actualNum === targetNum };
        }
        return { matched: actualStr === targetStr };
    }
    if (comparator === "not_equals") {
        const res = evaluateLogicCondition(item, "equals", fieldPath, compareValue);
        return { matched: !res.matched };
    }
    if (comparator === "gt" || comparator === "lt") {
        const actualNum = Number(actualStr);
        const targetNum = Number(targetStr);
        if (Number.isFinite(actualNum) && Number.isFinite(targetNum) && actualStr !== "" && targetStr !== "") {
            return { matched: comparator === "gt" ? actualNum > targetNum : actualNum < targetNum };
        }
        return { matched: comparator === "gt" ? actualStr > targetStr : actualStr < targetStr };
    }
    if (comparator === "regex") {
        if (!targetStr) return { matched: Boolean(actualStr) };
        try {
            const re = new RegExp(targetStr, "i");
            return { matched: re.test(actualStr) };
        } catch (err) {
            return {
                matched: false,
                error: err instanceof Error ? `正则表达式无效：${err.message}` : "正则表达式无效",
            };
        }
    }
    return { matched: false };
}

export function evaluateSetOperation(
    operator: "intersect" | "union" | "difference",
    itemsA: CanvasDataItem[],
    itemsB: CanvasDataItem[],
    matchKey?: string,
): CanvasDataItem[] {
    const keysB = new Set(itemsB.map((item) => itemIdentityKey(item, matchKey)).filter(Boolean));
    const seen = new Set<string>();
    const result: CanvasDataItem[] = [];

    if (operator === "intersect") {
        for (const item of itemsA) {
            const key = itemIdentityKey(item, matchKey);
            if (!key || !keysB.has(key) || seen.has(key)) continue;
            seen.add(key);
            result.push(item);
        }
        return result;
    }

    if (operator === "difference") {
        for (const item of itemsA) {
            const key = itemIdentityKey(item, matchKey);
            if (!key || keysB.has(key) || seen.has(key)) continue;
            seen.add(key);
            result.push(item);
        }
        return result;
    }

    // union: A ∪ B
    for (const item of [...itemsA, ...itemsB]) {
        const key = itemIdentityKey(item, matchKey);
        if (!key || seen.has(key)) continue;
        seen.add(key);
        result.push(item);
    }
    return result;
}

export function collectDetectedKeys(items: CanvasDataItem[]): string[] {
    const keys = new Set<string>();
    for (const item of items.slice(0, 25)) {
        if (item && typeof item === "object" && !Array.isArray(item)) {
            for (const key of Object.keys(item)) {
                keys.add(key);
                if (keys.size >= 16) return [...keys];
            }
        }
    }
    return [...keys];
}

type ArithToken =
    | { kind: "num"; value: number }
    | { kind: "ident"; name: string }
    | { kind: "op"; op: "+" | "-" | "*" | "/" | "(" | ")" };

function tokenizeArithmetic(expr: string): ArithToken[] | null {
    const tokens: ArithToken[] = [];
    let i = 0;
    while (i < expr.length) {
        const ch = expr[i];
        if (/\s/.test(ch)) {
            i += 1;
            continue;
        }
        if (ch === "+" || ch === "-" || ch === "*" || ch === "/" || ch === "(" || ch === ")") {
            tokens.push({ kind: "op", op: ch });
            i += 1;
            continue;
        }
        if (/\d/.test(ch) || (ch === "." && /\d/.test(expr[i + 1] || ""))) {
            let start = i;
            i += 1;
            while (i < expr.length && /[\d.]/.test(expr[i])) i += 1;
            const num = Number(expr.slice(start, i));
            if (!Number.isFinite(num)) return null;
            tokens.push({ kind: "num", value: num });
            continue;
        }
        if (/[a-zA-Z_\u4e00-\u9fa5]/.test(ch)) {
            let start = i;
            i += 1;
            while (i < expr.length && /[a-zA-Z0-9_.\u4e00-\u9fa5]/.test(expr[i])) i += 1;
            tokens.push({ kind: "ident", name: expr.slice(start, i) });
            continue;
        }
        return null;
    }
    return tokens;
}

function computeNumber(raw: unknown, field: string): number {
    if (typeof raw !== "number" && (typeof raw !== "string" || !/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(raw.trim()))) {
        throw new Error(`缺失或非法数值字段：${field}`);
    }
    const value = Number(raw);
    if (!Number.isFinite(value)) throw new Error(`非有限数值：${field}`);
    return value;
}

function computeField(item: CanvasDataItem, field: string): unknown {
    if (!field || field.split(".").some((part) => !part.trim()) || !item || typeof item !== "object") {
        throw new Error(`缺失或非法字段：${field}`);
    }
    return readDataItemField(item, field);
}

function validateFactorSource(item: CanvasDataItem): void {
    const source = computeField(item, "excretion_factor_source") as Record<string, unknown> | undefined;
    const citations = source?.citations;
    if (!source || typeof source.version !== "string" || !source.version.trim() || !Array.isArray(citations) || !citations.length || citations.some((citation) =>
        !citation || typeof citation.evidence_id !== "string" || !citation.evidence_id.trim() || !Number.isSafeInteger(citation.revision) || citation.revision <= 0)) {
        throw new Error("excretion_factor_source 必须提供版本及有效证据引用");
    }
}

function finiteCompute(value: number): number {
    if (!Number.isFinite(value)) throw new Error("计算结果非有限");
    return value;
}

function roundCompute(value: number): number {
    finiteCompute(value);
    // Avoid overflowing a finite large result merely to round decimal places.
    return Math.abs(value) <= Number.MAX_VALUE / 10000 ? Math.round(value * 10000) / 10000 : value;
}

export function evaluateNumericExpression(item: CanvasDataItem, expression: string): { value: number; error?: string } {
    const cleanExpr = expression.trim();
    const resolvedExpr =
        cleanExpr === "wbe_consumption_mg_1000p"
            ? "concentration_ng_l * flow_m3_day * excretion_factor / population"
            : cleanExpr;
    const tokens = tokenizeArithmetic(resolvedExpr);
    if (!tokens || tokens.length === 0) {
        return { value: 0, error: `计算表达式语法无效：${cleanExpr}` };
    }
    let pos = 0;

    const parsePrimary = (): number => {
        const tok = tokens[pos];
        if (!tok) throw new Error("表达式意外结束");
        if (tok.kind === "op" && tok.op === "+") {
            pos += 1;
            return parsePrimary();
        }
        if (tok.kind === "op" && tok.op === "-") {
            pos += 1;
            return -parsePrimary();
        }
        if (tok.kind === "op" && tok.op === "(") {
            pos += 1;
            const inner = parseAddSub();
            const close = tokens[pos];
            if (!close || close.kind !== "op" || close.op !== ")") {
                throw new Error("缺少右括号");
            }
            pos += 1;
            return inner;
        }
        if (tok.kind === "num") {
            pos += 1;
            return tok.value;
        }
        if (tok.kind === "ident") {
            pos += 1;
            if (tok.name === "excretion_factor") validateFactorSource(item);
            return computeNumber(computeField(item, tok.name), tok.name);
        }
        throw new Error("无效的计算因子");
    };

    const parseMulDiv = (): number => {
        let left = parsePrimary();
        while (pos < tokens.length) {
            const tok = tokens[pos];
            if (tok.kind === "op" && (tok.op === "*" || tok.op === "/")) {
                pos += 1;
                const right = parsePrimary();
                if (tok.op === "/" && right === 0) throw new Error("除零");
                left = finiteCompute(tok.op === "*" ? left * right : left / right);
            } else {
                break;
            }
        }
        return left;
    };

    const parseAddSub = (): number => {
        let left = parseMulDiv();
        while (pos < tokens.length) {
            const tok = tokens[pos];
            if (tok.kind === "op" && (tok.op === "+" || tok.op === "-")) {
                pos += 1;
                const right = parseMulDiv();
                left = finiteCompute(tok.op === "+" ? left + right : left - right);
            } else {
                break;
            }
        }
        return left;
    };

    try {
        const val = parseAddSub();
        if (pos < tokens.length) {
            return { value: 0, error: `计算表达式存在多余符号：${cleanExpr}` };
        }
        return { value: roundCompute(val) };
    } catch (err) {
        return {
            value: 0,
            error: err instanceof Error ? `计算表达式错误：${err.message}` : "计算表达式错误",
        };
    }
}

export function evaluateComputeOperation(
    items: CanvasDataItem[],
    config: CanvasLogicConfig,
): { items: CanvasDataItem[]; error?: string } {
    const outKey = (config.outputField || "computed_value").trim() || "computed_value";
    if (items.some((item) => item && typeof item === "object"
        ? Object.prototype.hasOwnProperty.call(item, outKey)
        : outKey === "value")) {
        return { items: [], error: `compute outputField 重复：${outKey}` };
    }
    if (Object.prototype.hasOwnProperty.call(config, "periodComparison")) {
        return evaluatePeriodComparison(items, config, outKey);
    }
    const expr = (config.expression || config.fieldPath || "").trim();
    if (!expr) return { items: [], error: "compute expression 必须明确指定" };
    const outputItems: CanvasDataItem[] = [];
    for (const item of items) {
        const { value, error } = evaluateNumericExpression(item, expr);
        if (error) return { items: [], error };
        if (item && typeof item === "object" && !Array.isArray(item)) {
            outputItems.push({
                ...(item as Record<string, unknown>),
                [outKey]: value,
            });
        } else {
            outputItems.push({
                value: item,
                [outKey]: value,
            });
        }
    }
    return { items: outputItems };
}

function evaluatePeriodComparison(items: CanvasDataItem[], config: CanvasLogicConfig, outKey: string): { items: CanvasDataItem[]; error?: string } {
    try {
        const comparison = config.periodComparison;
        const fields = ["regionField", "substanceField", "periodField", "baselinePeriodField", "valueField"] as const;
        if (!comparison || typeof comparison !== "object" || Array.isArray(comparison) || Object.keys(comparison).length !== fields.length ||
            fields.some((key) => typeof comparison[key] !== "string" || !comparison[key].trim() || comparison[key].split(".").some((part) => !part.trim())) ||
            config.expression?.trim() || config.fieldPath?.trim()) throw new Error("periodComparison 配置非法或与表达式冲突");
        const statusKey = `${outKey}_status`;
        const key = (item: CanvasDataItem, periodField: string) => JSON.stringify([comparison.regionField, comparison.substanceField, periodField].map((field) => {
            const value = computeField(item, field);
            if ((typeof value !== "string" || !value.trim()) && (typeof value !== "number" || !Number.isFinite(value))) throw new Error(`缺失或非法关联字段：${field}`);
            return value;
        }));
        const index = new Map<string, number>();
        for (const item of items) {
            if (Object.prototype.hasOwnProperty.call(item, outKey) || Object.prototype.hasOwnProperty.call(item, statusKey)) throw new Error("compute outputField 重复");
            const identity = key(item, comparison.periodField);
            if (index.has(identity)) throw new Error("periodComparison 重复区域/物质/周期基线");
            index.set(identity, computeNumber(computeField(item, comparison.valueField), comparison.valueField));
        }
        const output = items.map((item) => {
            const baseline = index.get(key(item, comparison.baselinePeriodField));
            const current = computeNumber(computeField(item, comparison.valueField), comparison.valueField);
            const status = baseline === undefined ? "missing_baseline" : baseline === 0 ? "zero_baseline" : "ok";
            const value = baseline === undefined || baseline === 0 ? null : roundCompute(finiteCompute(finiteCompute(current - baseline) / baseline) * 100);
            return { ...(item as Record<string, unknown>), [outKey]: value, [statusKey]: status };
        });
        return { items: output };
    } catch (error) {
        return { items: [], error: error instanceof Error ? error.message : "periodComparison 计算错误" };
    }
}

const PROVENANCE_KEYS = ["citations", "evidence", "metadata"];
const missingAnalyticValue = (value: unknown) => value == null || (typeof value === "string" && !value.trim());

function analyticKeys(raw: unknown, name: string): string[] {
    if (!Array.isArray(raw) || raw.some(key => typeof key !== "string" || !key.trim() || key !== key.trim()) || new Set(raw).size !== raw.length) {
        throw new Error(`${name} 必须是非空且不重复的字段名数组`);
    }
    return raw;
}

function analyticField(item: CanvasDataItem, key: string): unknown {
    return item && typeof item === "object" ? readDataItemField(item, key) : undefined;
}

function analyticIdentity(item: CanvasDataItem, keys: string[], requireComplete: boolean): string | undefined {
    const values = keys.map(key => analyticField(item, key));
    if (requireComplete && values.some(missingAnalyticValue)) return undefined;
    for (const value of values) {
        if (value != null && !["string", "number", "boolean"].includes(typeof value)) throw new Error("分组/关联/去重键必须是标量");
        if (typeof value === "number" && !Number.isFinite(value)) throw new Error("键必须是有限数值");
    }
    return JSON.stringify(values.map(value => [value == null ? "missing" : typeof value, value ?? null]));
}

function compareCodepoints(left: string, right: string): number {
    const a = Array.from(left, char => char.codePointAt(0)!), b = Array.from(right, char => char.codePointAt(0)!);
    for (let i = 0; i < Math.min(a.length, b.length); i++) if (a[i] !== b[i]) return a[i] - b[i];
    return a.length - b.length;
}

function canonicalProvenance(value: unknown): string {
    if (Array.isArray(value)) return `[${value.map(canonicalProvenance).join(",")}]`;
    if (value && typeof value === "object") return `{${Object.keys(value).sort(compareCodepoints).map(key => `${JSON.stringify(key)}:${canonicalProvenance((value as Record<string, unknown>)[key])}`).join(",")}}`;
    return JSON.stringify(value) ?? "null";
}

function cleanProvenance(value: unknown): unknown {
    if (Array.isArray(value)) {
        const seen = new Set<string>();
        return value.filter(item => item != null).map(cleanProvenance).filter(item => { const key = canonicalProvenance(item); if (seen.has(key)) return false; seen.add(key); return true; });
    }
    if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([key, child]) => [key, cleanProvenance(child)]));
    return value;
}

// Preserve provenance without interpreting evidence authenticity. Scalars with conflicts become arrays.
function mergeProvenanceValue(left: unknown, right: unknown): unknown {
    left = cleanProvenance(left); right = cleanProvenance(right);
    if (left == null) return right;
    if (right == null) return left;
    if (canonicalProvenance(left) === canonicalProvenance(right)) return left;
    if (left && right && typeof left === "object" && typeof right === "object" && !Array.isArray(left) && !Array.isArray(right)) {
        const merged = { ...left } as Record<string, unknown>;
        for (const [key, value] of Object.entries(right)) Object.defineProperty(merged, key, { value: mergeProvenanceValue(merged[key], value), enumerable: true, writable: true, configurable: true });
        return merged;
    }
    const values = [...(Array.isArray(left) ? left : [left]), ...(Array.isArray(right) ? right : [right])];
    const seen = new Set<string>();
    return values.filter(value => { const key = canonicalProvenance(value); if (seen.has(key)) return false; seen.add(key); return true; });
}

function mergeProvenance(rows: CanvasDataItem[]): Record<string, unknown> {
    const merged: Record<string, unknown> = {};
    for (const row of rows) if (row && typeof row === "object") {
        for (const key of PROVENANCE_KEYS) if (Object.prototype.hasOwnProperty.call(row, key) && row[key] != null) merged[key] = mergeProvenanceValue(merged[key], row[key]);
    }
    return merged;
}

export function evaluateAggregateOperation(items: CanvasDataItem[], config: CanvasLogicConfig): CanvasDataItem[] {
    const groupFields = analyticKeys(config.groupBy === undefined ? [] : config.groupBy, "groupBy");
    const metrics = config.metrics === undefined ? [{ field: "", fn: "count", as: "count" }] : config.metrics;
    const aliases = new Set([...groupFields, ...PROVENANCE_KEYS]);
    if (groupFields.some(key => PROVENANCE_KEYS.includes(key))) throw new Error("groupBy 与证据字段冲突");
    if (!Array.isArray(metrics) || !metrics.length) throw new Error("metrics 不能为空");
    for (const metric of metrics) {
        if (!metric || !["count", "sum", "avg", "max", "min", "p95"].includes(metric.fn) || typeof metric.field !== "string" || metric.field !== metric.field.trim() || (metric.fn !== "count" && !metric.field) || typeof metric.as !== "string" || !metric.as.trim() || metric.as !== metric.as.trim() || aliases.has(metric.as)) throw new Error("metrics 函数、字段或别名非法/重复");
        aliases.add(metric.as);
    }
    const groups = new Map<string, CanvasDataItem[]>();
    if (!groupFields.length) groups.set("[]", []);
    for (const item of items) {
        const key = analyticIdentity(item, groupFields, false)!;
        const bucket = groups.get(key) ?? [];
        bucket.push(item); groups.set(key, bucket);
    }
    return [...groups.values()].map(rows => {
        const entries: [string, unknown][] = groupFields.map(key => [key, analyticField(rows[0], key) ?? null]);
        for (const metric of metrics) {
            const present = metric.field ? rows.map(row => analyticField(row, metric.field)).filter(value => !missingAnalyticValue(value)) : rows;
            let value: number | null = rows.length;
            if (metric.fn !== "count") {
                const numbers = present.map(raw => computeNumber(raw, metric.field)).sort((a, b) => a - b);
                value = null;
                if (numbers.length) {
                    if (metric.fn === "min") value = numbers[0];
                    else if (metric.fn === "max") value = numbers[numbers.length - 1];
                    else if (metric.fn === "p95") value = numbers[Math.ceil(numbers.length * 0.95) - 1];
                    else {
                        value = numbers.reduce((sum, n) => { const next = sum + n; if (!Number.isFinite(next)) throw new Error("聚合结果非有限数值"); return next; }, 0);
                        if (metric.fn === "avg") value /= numbers.length;
                    }
                    value = roundCompute(value);
                }
            }
            entries.push([metric.as, value]);
        }
        return { ...Object.fromEntries(entries), ...mergeProvenance(rows) };
    });
}

function analyticPaths(record: Record<string, unknown>, prefix = ""): string[] {
    const paths: string[] = [];
    for (const [key, value] of Object.entries(record)) {
        const path = prefix + key; paths.push(path);
        if (value && typeof value === "object" && !Array.isArray(value)) paths.push(...analyticPaths(value as Record<string, unknown>, path + "."));
    }
    return paths;
}

export function evaluateJoinOperation(
    itemsA: CanvasDataItem[],
    itemsB: CanvasDataItem[],
    config: CanvasLogicConfig,
): CanvasDataItem[] {
    const pairs = config.joinKeys === undefined ? [{ left: "id", right: "id" }] : config.joinKeys;
    const joinMode = config.joinMode === undefined ? "inner" : config.joinMode;
    if (!["inner", "left"].includes(joinMode) || !Array.isArray(pairs) || !pairs.length || pairs.some(pair => !pair)) throw new Error("joinMode/joinKeys 非法");
    const leftKeys = analyticKeys(pairs.map(pair => pair.left), "joinKeys.left");
    const rightKeys = analyticKeys(pairs.map(pair => pair.right), "joinKeys.right");
    const record = (item: CanvasDataItem, side: string): Record<string, unknown> => item && typeof item === "object" ? item : { [side + "Value"]: item };
    const leftFields = new Set(itemsA.flatMap(item => analyticPaths(record(item, "left"))));
    const rightFields = [...new Set(itemsB.flatMap(item => Object.keys(record(item, "right"))))].sort(compareCodepoints);
    const used = new Set([...leftFields, ...itemsB.flatMap(item => analyticPaths(record(item, "right")))]);
    const names = new Map(rightFields.map(key => {
        let name = key;
        if (leftFields.has(key)) { do { name = `right.${name}`; } while (used.has(name)); }
        used.add(name); return [key, name];
    }));
    const rightIndex = new Map<string, Record<string, unknown>[]>();
    for (const item of itemsB) {
        const key = analyticIdentity(item, rightKeys, true);
        if (key === undefined) continue;
        const bucket = rightIndex.get(key) ?? []; bucket.push(record(item, "right")); rightIndex.set(key, bucket);
    }
    const joined: CanvasDataItem[] = [];
    for (const item of itemsA) {
        const key = analyticIdentity(item, leftKeys, true);
        const left = record(item, "left");
        const matches = key === undefined ? undefined : rightIndex.get(key);
        if (matches?.length) {
            for (const right of matches) joined.push({ ...left, ...Object.fromEntries(Object.entries(right).map(([key, value]) => [names.get(key)!, value])), ...mergeProvenance([left, right]) });
        } else if (joinMode === "left") joined.push({ ...left });
    }
    return joined;
}

export function evaluateSortLimitOperation(items: CanvasDataItem[], config: CanvasLogicConfig): CanvasDataItem[] {
    if (config.sortField !== undefined && typeof config.sortField !== "string") throw new Error("sortField 非法");
    const sortField = config.sortField || config.fieldPath || "value";
    if (typeof sortField !== "string" || !sortField.trim() || sortField !== sortField.trim()) throw new Error("sortField 非法");
    const sortOrder = config.sortOrder === undefined ? "desc" : config.sortOrder;
    const limit = config.limit === undefined ? 10 : config.limit;
    if (!["asc", "desc"].includes(sortOrder) || !Number.isSafeInteger(limit) || limit <= 0) throw new Error("sortOrder/limit 非法");
    const keys = analyticKeys(config.dedupeKeys === undefined ? [] : config.dedupeKeys, "dedupeKeys");
    const multiplier = sortOrder === "asc" ? 1 : -1;
    const sortValues = items.map(item => {
        const value = readDataItemField(item, sortField);
        if (!missingAnalyticValue(value) && typeof value !== "string" && typeof value !== "number" && typeof value !== "boolean") throw new Error("排序值必须是标量");
        if (typeof value === "number" && !Number.isFinite(value)) throw new Error("排序值必须是有限数值");
        return value;
    });
    const sorted = items.map((item, index) => ({ item, index })).sort((a, b) => {
        const rawA = sortValues[a.index], rawB = sortValues[b.index];
        const missingA = missingAnalyticValue(rawA), missingB = missingAnalyticValue(rawB);
        if (missingA || missingB) return missingA === missingB ? a.index - b.index : missingA ? 1 : -1;
        const numeric = (raw: unknown) => (typeof raw === "number" || typeof raw === "string" && /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(raw.trim())) && Number.isFinite(Number(raw));
        const numericA = numeric(rawA), numericB = numeric(rawB);
        let order: number;
        if (numericA && numericB) order = Number(rawA) < Number(rawB) ? -1 : Number(rawA) > Number(rawB) ? 1 : 0;
        else if (numericA !== numericB) order = numericA ? -1 : 1;
        else { const sa = `${typeof rawA}:${String(rawA)}`, sb = `${typeof rawB}:${String(rawB)}`; order = compareCodepoints(sa, sb); }
        return order * multiplier || a.index - b.index;
    });
    const seen = new Map<string, number>();
    const output: CanvasDataItem[] = [];
    for (const { item } of sorted) {
        const key = keys.length ? analyticIdentity(item, keys, true) : undefined;
        const previous = key === undefined ? undefined : seen.get(key);
        if (previous !== undefined) {
            const winner = output[previous];
            if (winner && typeof winner === "object") output[previous] = { ...winner, ...mergeProvenance([winner, item]) };
        } else {
            if (key !== undefined) seen.set(key, output.length);
            output.push(item);
        }
    }
    return output.slice(0, limit);
}

export function evaluateLogicNode(node: CanvasNodeData, inputA: CanvasDataItem[], inputB: CanvasDataItem[]): DataflowNodeResult {
    const raw = node.metadata?.logic;
    let contractError: string | undefined;
    if (raw && Object.prototype.hasOwnProperty.call(raw, "operator") &&
        (typeof raw.operator !== "string" || !Object.prototype.hasOwnProperty.call(LOGIC_OPERATOR_LABELS, raw.operator))) {
        contractError = "未知或非法 operator";
    } else if (raw?.operator === "sort_limit" && Object.prototype.hasOwnProperty.call(raw, "limit") &&
        (typeof raw.limit !== "number" || !Number.isSafeInteger(raw.limit) || raw.limit <= 0)) {
        contractError = "sort_limit limit 必须是正整数";
    }
    if (contractError) {
        return { nodeId: node.id, inputA, inputB, outputItems: [], trueItems: [], falseItems: [], detectedKeys: collectDetectedKeys([...inputA, ...inputB]), error: contractError };
    }
    const config = readNodeLogicConfig(node);
    const { operator, fieldPath, comparator = "contains", compareValue = "", matchKey } = config;
    const allInputs = [...inputA, ...inputB];
    const detectedKeys = collectDetectedKeys(allInputs);

    if (operator === "intersect" || operator === "union" || operator === "difference") {
        const outputItems = evaluateSetOperation(operator, inputA, inputB, matchKey);
        return {
            nodeId: node.id,
            operator,
            inputA,
            inputB,
            outputItems,
            trueItems: outputItems,
            falseItems: [],
            detectedKeys,
        };
    }

    if (operator === "join" || operator === "aggregate" || operator === "sort_limit") {
        try {
            const outputItems = operator === "join" ? evaluateJoinOperation(inputA, inputB, config) : operator === "aggregate" ? evaluateAggregateOperation(allInputs, config) : evaluateSortLimitOperation(allInputs, config);
            return { nodeId: node.id, operator, inputA, inputB, outputItems, trueItems: outputItems, falseItems: [], detectedKeys: collectDetectedKeys(outputItems) };
        } catch (error) {
            return { nodeId: node.id, operator, inputA, inputB, outputItems: [], trueItems: [], falseItems: [], detectedKeys, error: error instanceof Error ? error.message : "算子合同错误" };
        }
    }

    if (operator === "compute") {
        const { items: outputItems, error } = evaluateComputeOperation(allInputs, config);
        return {
            nodeId: node.id,
            operator,
            inputA: allInputs,
            inputB: [],
            outputItems,
            trueItems: outputItems,
            falseItems: [],
            detectedKeys: collectDetectedKeys(outputItems),
            error,
        };
    }

    if (operator === "if_else") {
        const trueItems: CanvasDataItem[] = [];
        const falseItems: CanvasDataItem[] = [];
        let firstError: string | undefined;
        for (const item of allInputs) {
            const res = evaluateLogicCondition(item, comparator, fieldPath, compareValue);
            if (res.error && !firstError) firstError = res.error;
            if (res.matched) trueItems.push(item);
            else falseItems.push(item);
        }
        return {
            nodeId: node.id,
            operator,
            inputA: allInputs,
            inputB: [],
            outputItems: trueItems,
            trueItems,
            falseItems,
            detectedKeys,
            error: firstError,
        };
    }

    if (operator === "filter") {
        const outputItems: CanvasDataItem[] = [];
        let firstError: string | undefined;
        for (const item of allInputs) {
            const res = evaluateLogicCondition(item, comparator, fieldPath, compareValue);
            if (res.error && !firstError) firstError = res.error;
            if (res.matched) outputItems.push(item);
        }
        return {
            nodeId: node.id,
            operator,
            inputA: allInputs,
            inputB: [],
            outputItems,
            trueItems: outputItems,
            falseItems: [],
            detectedKeys,
            error: firstError,
        };
    }

    // json_extract
    const extracted: CanvasDataItem[] = [];
    for (const item of allInputs) {
        const val = readDataItemField(item, fieldPath);
        if (val === undefined || val === null || val === "") continue;
        if (Array.isArray(val)) {
            val.forEach((v) => {
                if (v !== undefined && v !== null && v !== "") extracted.push(toCanvasDataItem(v));
            });
        } else {
            extracted.push(toCanvasDataItem(val));
        }
    }
    return {
        nodeId: node.id,
        operator,
        inputA: allInputs,
        inputB: [],
        outputItems: extracted,
        trueItems: extracted,
        falseItems: [],
        detectedKeys,
    };
}

export function formatDataItemsForPrompt(items: CanvasDataItem[]): string {
    if (!items.length) return "";
    const allPrimitive = items.every((item) => typeof item !== "object" || item === null);
    if (allPrimitive) {
        return items.map((item) => String(item)).join("\n");
    }
    return JSON.stringify(items, null, 2);
}

export function evaluateCanvasDataflow(nodes: CanvasNodeData[], connections: CanvasConnection[]): CanvasDataflowEvaluation {
    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    const incomingByTarget = new Map<string, CanvasConnection[]>();
    for (const conn of connections) {
        if (!nodeById.has(conn.fromNodeId) || !nodeById.has(conn.toNodeId)) continue;
        if (conn.relation === "batch-output" || conn.relation === "storyboard-output") continue;
        const list = incomingByTarget.get(conn.toNodeId) || [];
        list.push(conn);
        incomingByTarget.set(conn.toNodeId, list);
    }

    const results = new Map<string, DataflowNodeResult>();
    const visitState = new Map<string, "visiting" | "visited">();

    const readEdgeItemsInternal = (conn: CanvasConnection): CanvasDataItem[] => {
        const sourceResult = evaluateNode(conn.fromNodeId);
        const sourceNode = nodeById.get(conn.fromNodeId);
        if (sourceNode?.type === CanvasNodeType.Logic && isLogicDualOutput(sourceNode)) {
            if (conn.fromHandleId === "false") return sourceResult.falseItems;
            return sourceResult.trueItems;
        }
        return sourceResult.outputItems;
    };

    // A node whose managed workflow run succeeded carries the server's
    // authoritative output in metadata (including an explicit empty array).
    // Local prompt previews and stale local content must yield to it.
    const applyManagedRunOutput = (node: CanvasNodeData, res: DataflowNodeResult): DataflowNodeResult => {
        const derived = node.metadata?.derived;
        if (derived?.status !== "succeeded" || !derived.runId) return res;
        const computed = node.metadata?.computed;
        if (!Array.isArray(computed)) return res;
        const items = computed as CanvasDataItem[];
        const branches = node.metadata?.computedBranches;
        const dualBranch = node.type === CanvasNodeType.Logic && isLogicDualOutput(node);
        const trueItems = dualBranch && Array.isArray(branches?.true) ? (branches!.true as CanvasDataItem[]) : items;
        const falseItems = dualBranch && Array.isArray(branches?.false) ? (branches!.false as CanvasDataItem[]) : res.falseItems;
        return {
            ...res,
            outputItems: items,
            trueItems,
            falseItems,
            detectedKeys: collectDetectedKeys(items),
        };
    };

    const evaluateNode = (nodeId: string): DataflowNodeResult => {
        const cached = results.get(nodeId);
        if (cached) return cached;

        const state = visitState.get(nodeId);
        if (state === "visiting") {
            const cycleResult: DataflowNodeResult = {
                nodeId,
                inputA: [],
                inputB: [],
                outputItems: [],
                trueItems: [],
                falseItems: [],
                detectedKeys: [],
                cycleDetected: true,
                error: "检测到循环连线，已中止求值",
            };
            results.set(nodeId, cycleResult);
            return cycleResult;
        }

        visitState.set(nodeId, "visiting");
        const node = nodeById.get(nodeId);
        if (!node) {
            const empty: DataflowNodeResult = {
                nodeId,
                inputA: [],
                inputB: [],
                outputItems: [],
                trueItems: [],
                falseItems: [],
                detectedKeys: [],
            };
            visitState.set(nodeId, "visited");
            results.set(nodeId, empty);
            return empty;
        }

        if (node.type === CanvasNodeType.Logic) {
            const incoming = incomingByTarget.get(nodeId) || [];
            const dualInput = isLogicDualInput(node);
            const inputA: CanvasDataItem[] = [];
            const inputB: CanvasDataItem[] = [];
            let cyclicUpstream = false;

            incoming.forEach((conn, index) => {
                const upstreamRes = evaluateNode(conn.fromNodeId);
                if (upstreamRes.cycleDetected) cyclicUpstream = true;
                const items = readEdgeItemsInternal(conn);
                if (!dualInput) {
                    inputA.push(...items);
                    return;
                }
                if (conn.toHandleId === "input_b") {
                    inputB.push(...items);
                } else if (conn.toHandleId === "input_a") {
                    inputA.push(...items);
                } else if (index === 0) {
                    inputA.push(...items);
                } else {
                    inputB.push(...items);
                }
            });

            if (cyclicUpstream || results.get(nodeId)?.cycleDetected) {
                const cycleRes: DataflowNodeResult = {
                    nodeId,
                    operator: readNodeLogicConfig(node).operator,
                    inputA,
                    inputB,
                    outputItems: [],
                    trueItems: [],
                    falseItems: [],
                    detectedKeys: collectDetectedKeys([...inputA, ...inputB]),
                    cycleDetected: true,
                    error: "检测到循环连线，已中止求值",
                };
                visitState.set(nodeId, "visited");
                results.set(nodeId, cycleRes);
                return cycleRes;
            }

            const evaluated = applyManagedRunOutput(node, evaluateLogicNode(node, inputA, inputB));
            visitState.set(nodeId, "visited");
            results.set(nodeId, evaluated);
            return evaluated;
        }

        if (node.type === CanvasNodeType.Skill) {
            const incoming = incomingByTarget.get(nodeId) || [];
            if (incoming.length > 0) {
                const inputA: CanvasDataItem[] = [];
                let cyclicUpstream = false;
                let hasLogicOrSkillUpstream = false;
                let anyActiveUpstream = false;

                incoming.forEach((conn) => {
                    const upstreamRes = evaluateNode(conn.fromNodeId);
                    if (upstreamRes.cycleDetected) cyclicUpstream = true;
                    const sourceNode = nodeById.get(conn.fromNodeId);
                    if (sourceNode?.type === CanvasNodeType.Logic || sourceNode?.type === CanvasNodeType.Skill) {
                        hasLogicOrSkillUpstream = true;
                    }
                    const items = readEdgeItemsInternal(conn);
                    if (items.length > 0) anyActiveUpstream = true;
                    inputA.push(...items);
                });

                if (cyclicUpstream || results.get(nodeId)?.cycleDetected) {
                    const cycleRes: DataflowNodeResult = {
                        nodeId,
                        inputA,
                        inputB: [],
                        outputItems: [],
                        trueItems: [],
                        falseItems: [],
                        detectedKeys: collectDetectedKeys(inputA),
                        cycleDetected: true,
                        error: "检测到循环连线，已中止求值",
                    };
                    visitState.set(nodeId, "visited");
                    results.set(nodeId, cycleRes);
                    return cycleRes;
                }

                if (hasLogicOrSkillUpstream && !anyActiveUpstream) {
                    const emptyRes: DataflowNodeResult = {
                        nodeId,
                        inputA: [],
                        inputB: [],
                        outputItems: [],
                        trueItems: [],
                        falseItems: [],
                        detectedKeys: [],
                    };
                    visitState.set(nodeId, "visited");
                    results.set(nodeId, emptyRes);
                    return emptyRes;
                }

                const skillPrompt = formatSkillNodePrompt(node);
                const upstreamText = formatDataItemsForPrompt(inputA);
                const combined = [skillPrompt, upstreamText ? `【上游输入数据】\n${upstreamText}` : ""].filter(Boolean).join("\n\n");
                const outputItems: CanvasDataItem[] = combined ? [combined] : inputA;
                const skillRes: DataflowNodeResult = {
                    nodeId,
                    format: "txt",
                    inputA,
                    inputB: [],
                    outputItems,
                    trueItems: outputItems,
                    falseItems: [],
                    detectedKeys: collectDetectedKeys(inputA),
                };
                const authoritative = applyManagedRunOutput(node, skillRes);
                visitState.set(nodeId, "visited");
                results.set(nodeId, authoritative);
                return authoritative;
            }
        }

        const parsed = extractNodeItems(node);
        const localResult: DataflowNodeResult = {
            nodeId,
            format: parsed.format,
            inputA: [],
            inputB: [],
            outputItems: parsed.items,
            trueItems: parsed.items,
            falseItems: [],
            detectedKeys: collectDetectedKeys(parsed.items),
            error: parsed.error,
        };
        const nodeResult = applyManagedRunOutput(node, localResult);
        visitState.set(nodeId, "visited");
        results.set(nodeId, nodeResult);
        return nodeResult;
    };

    for (const node of nodes) {
        if (node.type === CanvasNodeType.DataSource || node.type === CanvasNodeType.Logic || node.type === CanvasNodeType.Skill) {
            evaluateNode(node.id);
        }
    }

    const getEdgeItems = (fromNodeId: string, fromHandleId?: string) => {
        const node = nodeById.get(fromNodeId);
        const res = evaluateNode(fromNodeId);
        if (node?.type === CanvasNodeType.Logic && isLogicDualOutput(node)) {
            const isFalse = fromHandleId === "false";
            const items = isFalse ? res.falseItems : res.trueItems;
            return {
                items,
                active: items.length > 0 && !res.cycleDetected,
                isConditionalBranch: true,
                branchLabel: isFalse ? "False" : "True",
            };
        }
        return {
            items: res.outputItems,
            active: res.outputItems.length > 0 && !res.cycleDetected,
            isConditionalBranch: false,
        };
    };

    const getEdgeText = (fromNodeId: string, fromHandleId?: string) => {
        const { items, active } = getEdgeItems(fromNodeId, fromHandleId);
        if (!active) return "";
        return formatDataItemsForPrompt(items);
    };

    return {
        nodes: results,
        getEdgeItems,
        getEdgeText,
    };
}

/**
 * 检查目标节点上游是否连接了未命中的逻辑分支或异常逻辑节点。
 * 若某个目标节点连接了至少一个逻辑节点，且所有连入的逻辑分支均为空（0 条命中）或报错，
 * 则拦截下游 AI 生成并给出明确原因。
 */
export function findBlockedLogicBranchInput(
    nodeId: string,
    nodes: CanvasNodeData[],
    connections: CanvasConnection[],
): { node: CanvasNodeData; reason: string } | null {
    const hasLogicOrDataSource = nodes.some((node) => node.type === CanvasNodeType.Logic || node.type === CanvasNodeType.DataSource);
    if (!hasLogicOrDataSource) return null;

    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    const evaluation = evaluateCanvasDataflow(nodes, connections);

    // 覆盖“生成节点 -> 配置节点 <- 逻辑节点”、“生成节点 <- Skill 节点 <- 逻辑节点”以及直接连线“生成节点 <- 逻辑节点”等拓扑
    const targetIds = new Set<string>([nodeId]);
    for (const conn of connections) {
        if (conn.fromNodeId === nodeId && nodeById.get(conn.toNodeId)?.type === CanvasNodeType.Config) {
            targetIds.add(conn.toNodeId);
        }
    }
    let expanded = true;
    while (expanded) {
        expanded = false;
        for (const conn of connections) {
            if (
                targetIds.has(conn.toNodeId) &&
                !targetIds.has(conn.fromNodeId) &&
                nodeById.get(conn.fromNodeId)?.type === CanvasNodeType.Skill &&
                !evaluation.getEdgeItems(conn.fromNodeId, conn.fromHandleId).active
            ) {
                targetIds.add(conn.fromNodeId);
                expanded = true;
            }
        }
    }

    const logicEdges = connections.filter(
        (conn) => targetIds.has(conn.toNodeId) && nodeById.get(conn.fromNodeId)?.type === CanvasNodeType.Logic,
    );
    if (!logicEdges.length) return null;

    let anyActive = false;
    let firstBlocked: { node: CanvasNodeData; reason: string } | null = null;

    for (const edge of logicEdges) {
        const logicNode = nodeById.get(edge.fromNodeId)!;
        const result = evaluation.nodes.get(logicNode.id);
        if (result?.cycleDetected) {
            return { node: logicNode, reason: "存在循环连线" };
        }
        if (result?.error) {
            return { node: logicNode, reason: result.error };
        }
        const edgeStatus = evaluation.getEdgeItems(edge.fromNodeId, edge.fromHandleId);
        if (edgeStatus.active) {
            anyActive = true;
        } else if (!firstBlocked) {
            const reason = edgeStatus.isConditionalBranch
                ? `${edgeStatus.branchLabel || "True"} 分支当前未命中（0 条数据）`
                : "当前运算结果为空（0 条数据）";
            firstBlocked = { node: logicNode, reason };
        }
    }

    return anyActive ? null : firstBlocked;
}
