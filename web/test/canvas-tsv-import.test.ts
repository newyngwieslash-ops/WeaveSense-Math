import { expect, test } from "bun:test";

import { parseCsvItems, parseDataSourceNode } from "@/lib/canvas/dataflow-evaluator";
import { CanvasNodeType, type CanvasDataSourceFormat } from "@/types/canvas";

function parse(rawText: string, fileName = "rows.tsv", format: CanvasDataSourceFormat = "csv") {
    return parseDataSourceNode({
        id: "tsv-source", type: CanvasNodeType.DataSource, title: "TSV导入",
        position: { x: 0, y: 0 }, width: 430, height: 330,
        metadata: { dataSource: { format, fileName, rawText } },
    });
}

test("splits a Chinese TSV into independent fields and typed records", () => {
    expect(parse("id\ttitle\tscore\nt1\t制表甲\t90\nt2\t制表乙\t65\n").items).toEqual([
        { id: "t1", title: "制表甲", score: 90 },
        { id: "t2", title: "制表乙", score: 65 },
    ]);
});

test("preserves empty edge cells with BOM, CRLF and uppercase TSV filenames", () => {
    expect(parse("\uFEFF\r\nid\ttitle\tflag\tlast\r\n\r\n\t甲\ttrue\t\r\nq\t\tfalse\t\r\n", " ROWS.TSV ").items).toEqual([
        { id: "", title: "甲", flag: true, last: "" },
        { id: "q", title: "", flag: false, last: "" },
    ]);
});

test("keeps quoted tabs, commas and escaped quotes inside a TSV cell", () => {
    expect(parse('id\ttitle\tnote\nq1\t"引号\t中间"\t"逗号,与""双引号"""\n').items).toEqual([
        { id: "q1", title: "引号\t中间", note: '逗号,与"双引号"' },
    ]);
});

test("requires unique nonempty headers and treats a header-only file as no records", () => {
    expect(parse("甲\t乙").items).toEqual([]);
    expect(parse("").items).toEqual([]);
    expect(parse("\uFEFF\r\n").items).toEqual([]);
    for (const text of ["\t甲\t乙\t", "a\ta\n1\t2", "a\t a \n1\t2", "a\tb\n1", "a\tb\n1\t2\t3", 'a\tb\n"unclosed\tx', 'a\tb\nx"bad\ty', 'a\tb\n"x"bad\ty']) {
        const result = parse(text); expect(result.error).toBeTruthy(); expect(result.items).toEqual([]);
    }
});

test("CSV and TSV support multiline quoted cells and retain leading-zero IDs", () => {
    for (const [delimiter, name] of [[",", "rows.csv"], ["\t", "rows.tsv"]]) {
        const text = `\uFEFFid${delimiter}note${delimiter}value${delimiter}empty\r\n001${delimiter}"line1\r\nline2${delimiter}""quoted"""${delimiter}23${delimiter}\r\n002${delimiter}" padded "${delimiter}27.6${delimiter}\r\n`;
        const result = parse(text, name);
        expect(result.error).toBeUndefined();
        expect(result.items).toEqual([{ id: "001", note: `line1\nline2${delimiter}"quoted"`, value: 23, empty: "" }, { id: "002", note: " padded ", value: 27.6, empty: "" }]);
        expect(parse(`id${delimiter}v\n1${delimiter}2\n3`, name).error).toMatch(/3/);
    }
});

test("keeps comma defaults and respects non-CSV formats despite a TSV filename", () => {
    expect(parse('id,title\nc,"中文,逗号"', "rows.csv").items).toEqual([{ id: "c", title: "中文,逗号" }]);
    const tabText = "id\ttitle\nc\t中文";
    const commaResult = [{ "id\ttitle": "c\t中文" }];
    expect(parse(tabText, "rows.csv").items).toEqual(commaResult);
    expect(parse(tabText, "").items).toEqual(commaResult);
    expect(parseCsvItems(tabText)).toEqual(commaResult);
    expect(parse('[{"id":"j"}]', "rows.tsv", "json").items).toEqual([{ id: "j" }]);
    expect(parse('{"id":"a"}\n{"id":"b"}', "rows.tsv", "json").error).toBeDefined();
    expect(parse("甲\t乙\n丙\t丁", "rows.tsv", "txt").items).toEqual(["甲\t乙", "丙\t丁"]);
    expect(parse("# 标题\n正文", "rows.tsv", "markdown").items).toEqual([{ id: "1", title: "标题", content: "正文" }]);
    expect(parse('[{"id":"i"}]', "rows.tsv", "intel").items).toEqual([{ id: "i" }]);
});
