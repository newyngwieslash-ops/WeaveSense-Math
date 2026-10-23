import { afterAll, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import * as helpers from "./helpers/module-group-source";

const dir = mkdtempSync(join(tmpdir(), "canvas-contract-中文 space-"));
const srcRoot = fileURLToPath(new URL("../src/", import.meta.url));
const testRoot = import.meta.dir;
const sourceReader = () => {
    const read = (helpers as unknown as { readSourceText?: (path: string | URL) => string }).readSourceText;
    expect(typeof read).toBe("function");
    return read!;
};
afterAll(() => {
    expect(dirname(dir)).toBe(resolve(tmpdir()));
    expect(basename(dir)).toStartWith("canvas-contract-");
    rmSync(dir, { recursive: true, force: true });
});

test("source reads normalize only CRLF through native paths and file URLs", () => {
    const read = sourceReader();
    const file = join(dir, "入口.ts");
    const want = '\uFEFFconst 文 = "a b";\n\treturn "甲 乙";\r孤立\n';
    for (const value of [want, want.replace(/\n/g, "\r\n")]) {
        writeFileSync(file, value);
        expect(read(file)).toBe(want);
        expect(read(pathToFileURL(file))).toBe(want);
    }
    expect(() => read(join(dir, "missing.ts"))).toThrow();
});

test("module group reads preserve every member and order with mixed line endings", () => {
    const entry = join(dir, "group.ts");
    const split = join(dir, "part.ts");
    writeFileSync(entry, 'const 标题 = "空 格";\r\n');
    writeFileSync(split, '\treturn "乙";\n');
    helpers.SPLIT_MODULE_GROUPS[entry] = [split];
    try {
        expect(helpers.moduleGroupSource(entry)).toBe('const 标题 = "空 格";\n\n\treturn "乙";\n');
    } finally {
        delete helpers.SPLIT_MODULE_GROUPS[entry];
    }
});

// Run the original assertions against real source copies; mutations never touch src.
// Losing an order, guard, attribute, selector or menu mapping must still be caught.
const contracts = [
    ["canvas-keyboard.test.ts", "Canvas keyboard keeps node copy as the fallback when no text is selected", "event.preventDefault();\n                copySelectedNodes();", "copySelectedNodes();\n                event.preventDefault();"],
    ["canvas-image-upload.test.tsx", "上传先占位再传输，完成替换同一节点，弹窗不等待传输完成", "onClose();\n            await pendingUpload;", "await pendingUpload;\n            onClose();"],
    ["canvas-media-performance.test.ts", "decodes canvas images synchronously so zoom re-rasterization never paints blank frames", 'decoding="sync"', 'decoding="async"'],
    ["canvas-media-performance.test.ts", "keeps inactive video nodes on a viewport-gated static first frame", "hasPersistedPreview || !nearViewport", "hasPersistedPreview || nearViewport"],
    ["canvas-resource-mention-editor.test.ts", "refreshes async reference previews even when prompt text has not changed", "syncInlineMentionPreviews(editor, activeReferences)", "ignoredPreviewRefresh(editor, activeReferences)"],
    ["workspace-menu-density.test.tsx", "collection select focus uses a thin keyboard ring, not a persistent pointer outline", "outline: 1px solid var(--control-focus-ring) !important;", "outline: none !important;"],
    ["workspace-menu-density.test.tsx", "menu surfaces are explicitly scoped and old account inner overrides are removed", "body.app-spatial-overlays :where(.ant-dropdown-menu, .ant-select-dropdown, .ant-cascader-menus, .ant-mentions-dropdown) {\n        border: 0 !important;", "body.app-spatial-overlays :where(.ant-dropdown-menu, .ant-select-dropdown, .ant-cascader-menus, .ant-mentions-dropdown) {\n        border: 1px solid white !important;"],
    ["canvas-connection-create-menu.test.ts", "connection menu omits the header and provides descriptions for every option", 'title="数据源"', 'title="错误数据源"'],
] as const;

function originalContract(file: string, name: string) {
    const path = join(testRoot, file);
    const source = readFileSync(path, "utf8").replace(/\r\n/g, "\n");
    const marker = `test(${JSON.stringify(name)}, `;
    const start = source.indexOf(marker);
    expect(start).toBeGreaterThanOrEqual(0);
    const indent = source.slice(source.lastIndexOf("\n", start) + 1, start);
    const end = source.indexOf(`\n${indent}});`, start);
    expect(end).toBeGreaterThan(start);
    const callback = `${source.slice(start + marker.length, end)}\n}`;
    return callback.replace(/import\.meta\.url/g, JSON.stringify(pathToFileURL(path).href));
}

async function checkContract(file: string, name: string, crlf: boolean, mutation?: readonly [string, string]) {
    const read = sourceReader();
    let mutations = 0;
    const copy = (value: string) => {
        let text = value.replace(/\r\n/g, "\n");
        if (mutation && text.includes(mutation[0])) {
            mutations++;
            text = text.replaceAll(mutation[0], mutation[1]);
        }
        const path = join(dir, "source-copy.txt");
        writeFileSync(path, crlf ? text.replace(/\n/g, "\r\n") : text);
        return read(path);
    };
    const source = (path: string) => copy(readFileSync(resolve(testRoot, path), "utf8"));
    const readSrc = (path: string) => copy(readFileSync(resolve(srcRoot, path), "utf8"));
    const grouped = (path: string) => copy(helpers.moduleGroupSource(path));
    const bindings = {
        expect, source, read: readSrc,
        readSourceText: (path: string | URL) => copy(readFileSync(path, "utf8")),
        moduleGroupSource: grouped,
        canvasNodeContentSource: grouped("components/canvas/canvas-node-content.tsx"),
        canvasVideoPreviewSource: readSrc("services/canvas-video-preview.ts"),
        component: readSrc("components/canvas/canvas-workspace-overlays.tsx"),
        css: readSrc("styles/globals.css"),
    };
    const compiled = new Bun.Transpiler({ loader: "ts" }).transformSync(`const contract = ${originalContract(file, name)};`);
    const contract = new Function(...Object.keys(bindings), `${compiled}; return contract;`)(...Object.values(bindings));
    if (!mutation) return contract();
    let failure: unknown;
    try { await contract(); } catch (error) { failure = error; }
    expect(mutations).toBeGreaterThan(0);
    expect(failure).toBeInstanceOf(Error);
}

for (const [file, name, from, to] of contracts) {
    test(`original contract survives LF/CRLF and rejects semantic regression: ${name}`, async () => {
        await checkContract(file, name, false);
        await checkContract(file, name, true);
        await checkContract(file, name, true, [from, to]);
    });
}

test("menu descriptions and creation types cannot be missing or mismatched", async () => {
    const name = "connection menu omits the header and provides descriptions for every option";
    await checkContract("canvas-connection-create-menu.test.ts", name, false);
    for (const mutation of [
        ['description="接入 JSON、CSV、Markdown、TXT 或情报条目"', 'description=""'],
        ["onCreate(CanvasNodeType.DataSource)", "onCreate(CanvasNodeType.Text)"],
        ['title="逻辑与集合"', 'title="数据源"'],
    ] as const) await checkContract("canvas-connection-create-menu.test.ts", name, false, mutation);
    const option = helpers.readSourceText(new URL("../src/components/canvas/canvas-workspace-overlays.tsx", import.meta.url))
        .match(/<ConnectionCreateOption\b[\s\S]*?\n\s*\/>/g)?.find((value) => value.includes('title="数据源"'));
    expect(option).toBeDefined();
    await checkContract("canvas-connection-create-menu.test.ts", name, true, [option!, ""]);
});
