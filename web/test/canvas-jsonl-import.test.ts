import { expect, test } from "bun:test";

import { parseDataSourceNode, parseJsonItems } from "@/lib/canvas/dataflow-evaluator";
import { CanvasNodeType, type CanvasDataSourceFormat } from "@/types/canvas";

function parse(rawText: string, fileName = "rows.jsonl", format: CanvasDataSourceFormat = "json") {
    return parseDataSourceNode({
        id: "jsonl-source",
        title: "JSONL 导入",
        type: CanvasNodeType.DataSource,
        position: { x: 0, y: 0 },
        width: 420,
        height: 340,
        metadata: { dataSource: { format, fileName, rawText } },
    });
}

test("imports the two Chinese JSONL records in file order", () => {
    const result = parse('{"id":"j1","title":"逐行甲"}\n{"id":"j2","title":"逐行乙"}\n');
    expect(result.items).toEqual([{ id: "j1", title: "逐行甲" }, { id: "j2", title: "逐行乙" }]);
    expect(result.error).toBeUndefined();
});

test("ignores whitespace and BOM while accepting CRLF and uppercase JSONL filenames", () => {
    const result = parse('\uFEFF\r\n  {"title":"甲"}\r\n \t\r\n{"title":"乙"}  \r\n', ' ROWS.JSONL ');
    expect(result.items).toEqual([{ title: "甲" }, { title: "乙" }]);
    expect(result.error).toBeUndefined();
    expect(parse("\uFEFF\r\n \t\r\n").items).toEqual([]);
    expect(parse("").error).toBeUndefined();
});

test("keeps each JSON value as one record without unwrapping row containers", () => {
    const result = parse('{"items":[1,2]}\n[1,"二"]\n"字符串\\n正文"\n42\nfalse\nnull');
    expect(result.items).toEqual([
        { items: [1, 2] },
        { items: [1, "二"], text: '[1,"二"]' },
        "字符串\n正文", 42, false, "",
    ]);
    expect(result.error).toBeUndefined();
});

test("rejects the whole file and reports the original physical line on malformed input", () => {
    const result = parse('\r\n\r\n{"title":"有效"}\r\n \r\n{bad}\r\n{"title":"不应返回"}');
    expect(result.items).toEqual([]);
    expect(result.error).toContain("JSONL");
    expect(result.error).toMatch(/第\s*5\s*行/);
    expect(parse('{\n  "title":"跨行对象"\n}').items).toEqual([]);
});

test("JSON and intel share object/array/envelope contracts without bad-text fallback", () => {
    for (const format of ["json", "intel"] as const) {
        for (const key of ["items", "rows", "data", "records", "list", "documents", "results"]) expect(parse(JSON.stringify({ [key]: [{ id: "001", value: 23 }] }), "rows.json", format).items).toEqual([{ id: "001", value: 23 }]);
        expect(parse('\uFEFF{"id":"001","value":27.6}', "rows.json", format).items).toEqual([{ id: "001", value: 27.6 }]);
        for (const text of ["null", "42", "false", '"text"', "{broken", '[{"a":1},]', '{"v":1e999}']) {
            const result = parse(text, "rows.json", format); expect(result.error).toBeTruthy(); expect(result.items).toEqual([]);
        }
    }
    expect(parse('{"v":1}\n\n{"v":1e999}').error).toMatch(/第\s*3\s*行/);
});

test("preserves strict JSON and respects an explicit non-JSON format despite a JSONL filename", () => {
    const lines = '{"id":"a"}\n{"id":"b"}';
    expect(parse(lines, "ordinary.json").error).toBeDefined();
    expect(parse(lines, "").error).toBeDefined();
    expect(parseJsonItems(lines).error).toBeDefined();
    expect(parse('[{"id":"a"},{"id":"b"}]', "ordinary.json").items).toEqual([{ id: "a" }, { id: "b" }]);
    expect(parse('{"items":[{"id":"a"}]}', "ordinary.json").items).toEqual([{ id: "a" }]);
    expect(parse("id,title\na,中文", "rows.jsonl", "csv").items).toEqual([{ id: "a", title: "中文" }]);
    expect(parse("第一行\n第二行", "rows.jsonl", "txt").items).toEqual(["第一行", "第二行"]);
    expect(parse("情报甲\n情报乙", "rows.jsonl", "intel").error).toBeTruthy();
    expect(parse("# 章甲\n正文甲\n# 章乙\n正文乙", "rows.jsonl", "markdown").items).toHaveLength(2);
});
