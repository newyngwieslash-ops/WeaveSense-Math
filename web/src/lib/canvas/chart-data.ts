export type ChartRow = Record<string, string | number>;
export type ParsedChart = { rows: ChartRow[]; xKey: string; series: string[] };
export type ChartFieldConfig = { xKey?: unknown; series?: unknown };
export type ChartParseResult = { chart: ParsedChart | null; error?: string };

/** 模型文本解析失败不抛出；显式字段不允许静默退回自动推断。 */
export function parseChartData(source: string | string[], config: ChartFieldConfig = {}): ChartParseResult {
    const sources = (Array.isArray(source) ? source : [source]).map((text) => text.trim()).filter(Boolean);
    const rows: ChartRow[] = [];
    for (const text of sources) {
        // 每条边独立解码再合并，不能把 JSON 数组拼成文本，也不能丢弃混入的 CSV/非法输入。
        if (text === "[]") continue;
        const parsed = parseJsonRows(text) ?? (text.startsWith("[") || text.startsWith("{") ? null : parseCsvRows(text));
        if (!parsed) return { chart: null };
        for (const row of parsed) rows.push(row);
    }
    if (!rows.length) return { chart: null };
    const invalid = (reason: string): ChartParseResult => ({ chart: null, error: `图表配置错误：${reason}` });
    if (config.xKey !== undefined && (typeof config.xKey !== "string" || !config.xKey.trim())) return invalid("xKey 必须是非空字段名");
    if (config.series !== undefined && (!Array.isArray(config.series) || !config.series.length || !config.series.every((key) => typeof key === "string" && key.trim()))) return invalid("series 必须是非空字段名数组");

    const explicitX = config.xKey as string | undefined;
    const explicitSeries = config.series as string[] | undefined;
    for (const key of [ ...(explicitX === undefined ? [] : [explicitX]), ...(explicitSeries ?? []) ]) {
        if (!rows.every((row) => Object.hasOwn(row, key))) return invalid(`缺少字段「${key}」`);
    }
    const isNumeric = (key: string) => rows.every((row) => typeof row[key] === "number" && Number.isFinite(row[key]));
    for (const key of explicitSeries ?? []) {
        if (!isNumeric(key)) return invalid(`系列「${key}」必须为数值`);
    }
    const keys = Object.keys(rows[0]);
    const numericKeys = keys.filter(isNumeric);
    const series = explicitSeries ?? numericKeys.filter((key) => key !== explicitX);
    const xKey = explicitX ?? keys.find((key) => !numericKeys.includes(key)) ?? "__index";
    if (!series.length) return { chart: null };
    if (explicitX === undefined && xKey === "__index") rows.forEach((row, index) => { row.__index = index + 1; });
    return { chart: { rows, xKey, series } };
}

export function parseChartSource(text: string): ParsedChart | null {
    return parseChartData(text).chart;
}

function parseJsonRows(text: string): ChartRow[] | null {
    try {
        const parsed: unknown = JSON.parse(text);
        if (!Array.isArray(parsed)) return null;
        if (!parsed.every((item) => Boolean(item) && typeof item === "object" && !Array.isArray(item))) return null;
        return parsed.length ? parsed.map(normalizeRow) : null;
    } catch {
        return null;
    }
}

function parseCsvRows(text: string): ChartRow[] | null {
    const lines = text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
    if (lines.length < 2) return null;
    const header = lines[0].split(",").map((cell) => cell.trim());
    if (header.length < 2) return null;
    return lines.slice(1).map((line) => {
        const cells = line.split(",").map((cell) => cell.trim());
        return normalizeRow(Object.fromEntries(header.slice(0, cells.length).map((key, index) => [key, cells[index]])));
    });
}

function normalizeRow(row: Record<string, unknown>): ChartRow {
    return Object.fromEntries(Object.entries(row).map(([key, value]) => {
        if (typeof value === "number") return [key, value];
        const text = String(value ?? "").trim();
        const numeric = text !== "" && Number.isFinite(Number(text));
        return [key, numeric ? Number(text) : text];
    }));
}
