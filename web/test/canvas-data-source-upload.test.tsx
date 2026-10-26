import { afterAll, describe, expect, test } from "bun:test";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { join, resolve } from "node:path";
import { createRequire } from "node:module";
import { renderToStaticMarkup } from "react-dom/server";
import type { ChangeEvent, DragEvent, ReactElement } from "react";
import { CANVAS_UPLOAD_ACCEPT, isTextUploadFile, uploadNodeType } from "../src/lib/canvas/canvas-file-upload";
import { parseDataSourceNode } from "../src/lib/canvas/dataflow-evaluator";
import { readDataSourceFile } from "../src/lib/canvas/data-source-file";
import { CanvasNodeType, type CanvasNodeData } from "../src/types/canvas";
import { canvasThemes } from "../src/lib/canvas-theme";
import { useUserStore } from "../src/stores/use-user-store";

// Isolate only external services/UI contexts. Execute the real hook callbacks with
// real React hooks (SSR capture), real File reads and the real dataflow parser.
// No mock.module: these doubles cannot leak into other Bun test files.
const dir = mkdtempSync(resolve(import.meta.dir, "../node_modules/.cache-canvas-datasource-upload-"));
const require = createRequire(import.meta.url);
const src = resolve(import.meta.dir, "../src");
const dependencies = join(dir, "dependencies.ts");
writeFileSync(dependencies, `
export const state = { uploads: [], media: [], assets: [], errors: [], failure: null, updateNode: undefined };
export const App = { useApp: () => ({ message: {
    error: (value) => state.errors.push(value), warning: () => {}, success: () => {},
} }) };
export const useQueryClient = () => ({ invalidateQueries: async () => {} });
export const uploadResourceFile = async (file, kind, meta, progress) => {
    state.uploads.push({ file, kind, meta }); progress?.(file.size, file.size);
    if (state.pendingUpload) await state.pendingUpload(progress);
    if (state.failure) throw state.failure;
    return { id: "source-file" };
};
export const resourceStorageKey = (id) => "resource:" + id;
export const resourceIdFromStorageKey = (key) => key?.startsWith("resource:") ? key.slice(9) : "";
export const uploadMediaFile = async (_file, kind) => { state.media.push(kind); return {}; };
export const ensureCanvasNodeAsset = async (input) => { state.assets.push(input); return { assetId: "asset" }; };
export const CANVAS_IMAGE_ASSET_DND_TYPE = "image-asset";
export const CANVAS_PROJECT_CHAPTER_DND_TYPE = "chapter";
export const useCanvasNodeActions = () => ({ updateNode: state.updateNode });
export const useDataflowNodeResult = () => ({ outputItems: [], detectedKeys: [] });
export const useAssetStore = { getState: () => ({ assets: [] }) };
export const audioMetadata = () => ({}), videoMetadata = () => ({}), imageMetadata = () => ({});
export const getDataUrlByteSize = () => 0, readImageMeta = () => ({});
export const createCanvasNode = () => ({}), isAudioFile = () => false;
export const fitNodeSize = () => ({}), VIDEO_NODE_MAX_SIZE = {};
export const resolveImageUrl = () => "", uploadImage = async () => ({}), getProjectUnit = async () => ({});
`);
const real = new Set(["@/constant/canvas", "@/types/canvas", "@/lib/canvas/canvas-file-upload", "@/lib/canvas/data-source-file", "@/lib/canvas/dataflow-evaluator"]);
function isolate(relativePath: string, name: string) {
    const source = readFileSync(join(src, relativePath), "utf8").replace(/from "([^"]+)"/g, (statement, specifier: string) => {
        if (specifier === "react" || specifier === "lucide-react") return `from ${JSON.stringify(require.resolve(specifier))}`;
        if (real.has(specifier)) return `from ${JSON.stringify(join(src, specifier.slice(2)))}`;
        if (specifier.startsWith("@/") || specifier === "antd" || specifier === "@tanstack/react-query") return `from ${JSON.stringify(dependencies)}`;
        return statement;
    });
    const path = join(dir, name);
    writeFileSync(path, source);
    return path;
}
const { useCanvasUpload }: typeof import("../src/pages/canvas/use-canvas-upload") = await import(isolate("pages/canvas/use-canvas-upload.ts", "hook.ts"));
const { DataSourceNodeContent }: typeof import("../src/components/canvas/nodes/data-source-node") = await import(isolate("components/canvas/nodes/data-source-node.tsx", "node.tsx"));
const { state } = await import(dependencies);
afterAll(() => rmSync(dir, { recursive: true, force: true }));

const existing = (): CanvasNodeData => ({ id: "existing", type: CanvasNodeType.Text, title: "keep", position: { x: -500, y: -500 }, width: 100, height: 100, metadata: { content: "original" } });
function harness(initial: CanvasNodeData[] = [existing()]) {
    Object.assign(state, { uploads: [], media: [], assets: [], errors: [], failure: null, pendingUpload: null });
    const nodesRef = { current: initial };
    let api!: ReturnType<typeof useCanvasUpload>;
    function Capture() {
        api = useCanvasUpload({
            canvasId: "canvas", domainProjectId: "project", nodesRef,
            selectedNodeIdsRef: { current: new Set<string>() },
            getCanvasCenter: () => ({ x: 400, y: 400 }), screenToCanvas: (x, y) => ({ x, y }),
            setNodes: (next) => { nodesRef.current = typeof next === "function" ? next(nodesRef.current) : next; },
            setSelectedNodeIds: () => {}, setSelectedConnectionId: () => {}, setContextMenu: () => {}, setDialogNodeId: () => {},
        });
        return null;
    }
    renderToStaticMarkup(<Capture />);
    return { api, nodesRef };
}
async function withWindow(run: () => Promise<void>) {
    const before = Object.getOwnPropertyDescriptor(globalThis, "window");
    const elementBefore = Object.getOwnPropertyDescriptor(globalThis, "Element");
    Object.defineProperty(globalThis, "Element", { configurable: true, value: class Element {} });
    Object.defineProperty(globalThis, "window", { configurable: true, value: { setTimeout: () => 1, clearTimeout: () => {} } });
    try { await run(); } finally {
        if (before) Object.defineProperty(globalThis, "window", before);
        else Reflect.deleteProperty(globalThis, "window");
        if (elementBefore) Object.defineProperty(globalThis, "Element", elementBefore);
        else Reflect.deleteProperty(globalThis, "Element");
    }
}
async function settled() {
    for (let i = 0; i < 20; i++) await new Promise((resolve) => setTimeout(resolve, 1));
}
const tsv = '\uFEFFname\tvalue\tempty\r\n"a\tb"\t2\t\r\n\r\n';
const cases = [
    ["rows.CSV", "text/plain", "name,value\na,2", "csv"],
    ["rows.tsv", "", tsv, "csv"],
    ["rows.jsonl", "text/plain", '\uFEFF{"name":"a"}\r\n\r\n{"name":"b"}', "json"],
    ["rows.json", "application/octet-stream", '{"items":[{"name":"a"}]}', "json"],
    ["download", "text/tab-separated-values", "name\tvalue\na\t2", "csv"],
    ["download", "application/x-ndjson", '{"name":"a"}\n{"name":"b"}', "json"],
] as const;

describe("T05a upload classification", () => {
    test.each(cases)("%s (%s) is a DataSource, not plain text", (name, type) => {
        expect(uploadNodeType({ name, type })).toBe(CanvasNodeType.DataSource);
        expect(isTextUploadFile({ name, type })).toBe(false);
    });
    test.each(["text/csv", "application/csv", "application/json", "application/jsonl", "application/ndjson", "application/x-ndjson"])("supports MIME %s without an extension", (type) => {
        expect(uploadNodeType({ name: "download", type })).toBe(CanvasNodeType.DataSource);
    });
    test("extensions win over conflicting MIME and TXT/MD stay Text", () => {
        for (const name of ["rows.csv", "rows.tsv", "rows.jsonl", "rows.json"]) expect(uploadNodeType({ name, type: "audio/mpeg" })).toBe(CanvasNodeType.DataSource);
        for (const name of ["notes.txt", "notes.md", "notes.markdown"]) expect(uploadNodeType({ name, type: "application/json" })).toBe(CanvasNodeType.Text);
        expect(uploadNodeType({ name: "notes", type: "text/plain" })).toBe(CanvasNodeType.Text);
        expect(uploadNodeType({ name: "file.pdf", type: "application/pdf" })).toBeNull();
        for (const ext of [".csv", ".tsv", ".jsonl", ".json"]) expect(CANVAS_UPLOAD_ACCEPT.split(",")).toContain(ext);
    });
});

test("shared reader rejects malformed data and propagates file read errors", async () => {
    await expect(readDataSourceFile(new File(['{}\n\n{bad}'], "rows.jsonl"))).rejects.toThrow("第 3 行");
    const file = new File(["[]"], "rows.json");
    file.text = async () => { throw new Error("read denied"); };
    await expect(readDataSourceFile(file)).rejects.toThrow("read denied");
});

describe("T05a selection and global drop share validated imports", () => {
    for (const entry of ["selection", "drop", "input"] as const) {
        test.each(cases)(`${entry}: %s preserves existing nodes and resource provenance`, async (name, type, content, format) => withWindow(async () => {
            const { api, nodesRef } = harness();
            const original = nodesRef.current[0];
            const file = new File([content], name, { type });
            if (entry === "selection") expect(await api.handleUploadFiles([file])).toBe(true);
            else if (entry === "input") await api.handleImageInputChange({ target: { files: [file], value: name } } as unknown as ChangeEvent<HTMLInputElement>);
            else {
                api.handleDrop({ preventDefault() {}, stopPropagation() {}, target: null, clientX: 400, clientY: 400,
                    dataTransfer: { files: [file], getData: () => "" } } as unknown as DragEvent<HTMLDivElement>);
                await settled();
            }
            expect(nodesRef.current).toHaveLength(2);
            expect(nodesRef.current[0]).toBe(original);
            const node = nodesRef.current[1];
            expect(node.type).toBe(CanvasNodeType.DataSource);
            expect(node.metadata).toMatchObject({ content: await file.text(), storageKey: "resource:source-file", bytes: file.size, status: "success", dataSource: { format, rawText: await file.text(), fileName: name === "download" ? `${name}.${type === "text/tab-separated-values" ? "tsv" : "jsonl"}` : name } });
            expect(node.metadata?.fileUpload).toBeUndefined();
            expect(parseDataSourceNode(node).error).toBeUndefined();
            expect(parseDataSourceNode(node).items.length).toBeGreaterThan(0);
            if (name.endsWith("tsv")) expect(parseDataSourceNode(node).items).toEqual([{ name: "a\tb", value: 2, empty: "" }]);
            if (type === "text/tab-separated-values") expect(parseDataSourceNode(node).items).toEqual([{ name: "a", value: 2 }]);
            if (type === "application/x-ndjson") expect(parseDataSourceNode(node).items).toEqual([{ name: "a" }, { name: "b" }]);
            expect(state.uploads).toHaveLength(1);
            expect(state.uploads[0]).toMatchObject({ file, kind: "file", meta: { fileName: name } });
            expect(state.media).toEqual([]);
            expect(state.assets).toEqual([]);
        }));
    }
    test.each(["rows.json", "rows.jsonl"])("invalid %s fails before upload; never becomes successful Text", async (name) => withWindow(async () => {
        const { api, nodesRef } = harness();
        const content = name.endsWith("jsonl") ? '{"ok":true}\n\n{broken}\n{"ok":false}' : '{broken}';
        expect(await api.createFileNode(new File([content], name, { type: "text/plain" }), { x: 400, y: 400 })).toBeNull();
        expect(state.uploads).toEqual([]);
        expect(state.media).toEqual([]);
        expect(state.errors.join(" ")).toContain(name.endsWith("jsonl") ? "第 3 行" : "JSON");
        expect(nodesRef.current[0]).toEqual(existing());
        expect(nodesRef.current.slice(1).every((node) => node.type === CanvasNodeType.DataSource && node.metadata?.fileUpload === "error")).toBe(true);
    }));
    test("replacement failure restores the complete original even without content", async () => withWindow(async () => {
        const original: CanvasNodeData = { ...existing(), type: CanvasNodeType.DataSource, title: "bound source", width: 999, height: 777,
            metadata: { dataSource: { format: "json", rawText: "[]", fileName: "old.json", sourceBinding: { kind: "static" } }, storageKey: "resource:old", status: "error", errorDetails: "old error" } };
        const { api, nodesRef } = harness([original, { ...existing(), id: "other" }]);
        const before = structuredClone(nodesRef.current);
        state.failure = new Error("quota exceeded");
        expect(await api.replaceNodeMedia(original.id, new File(['[{"x":1}]'], "new.json", { type: "text/plain" }))).toBe(false);
        expect(nodesRef.current).toEqual(before);
        expect(nodesRef.current[0]).toEqual(original);
        expect(state.errors).toContain("quota exceeded");
    }));
    test("malformed replacement preserves original without content", async () => withWindow(async () => {
        const original: CanvasNodeData = { ...existing(), type: CanvasNodeType.DataSource, metadata: { dataSource: { format: "json", rawText: "[]" } } };
        const { api, nodesRef } = harness([original]);
        expect(await api.replaceNodeMedia(original.id, new File(['{}\ninvalid'], "new.jsonl", { type: "text/plain" }))).toBe(false);
        expect(nodesRef.current).toEqual([original]);
        expect(state.uploads).toHaveLength(0);
    }));
    test("DataSource cannot be silently replaced by a different media type", async () => withWindow(async () => {
        const original = { ...existing(), type: CanvasNodeType.DataSource };
        const { api, nodesRef } = harness([original]);
        expect(await api.replaceNodeMedia(original.id, new File(["note"], "note.txt", { type: "text/plain" }))).toBe(false);
        expect(nodesRef.current).toEqual([original]);
        expect(state.uploads).toEqual([]);
    }));
    test.each(["notes.txt", "notes.md"])("%s retains Text upload behavior", async (name) => withWindow(async () => {
        const { api, nodesRef } = harness();
        expect(await api.handleUploadFiles([new File(["hello"], name, { type: "text/plain" })])).toBe(true);
        expect(nodesRef.current[1].type).toBe(CanvasNodeType.Text);
        expect(nodesRef.current[1].metadata?.content).toBe("hello");
    }));
});

test.each([false, true])("failed replacement preserves interleaved edits (upload fields edited: %s)", async (editUploadFields) => withWindow(async () => {
    const original: CanvasNodeData = { ...existing(), type: CanvasNodeType.DataSource, width: 999, height: 777,
        metadata: { content: "[]", dataSource: { format: "json", rawText: "[]" }, status: "error", errorDetails: "old error", size: "old size" } };
    const { api, nodesRef } = harness([original, { ...existing(), id: "other" }]);
    let entered!: () => void;
    const started = new Promise<void>((resolve) => { entered = resolve; });
    let rejectUpload!: (error: Error) => void;
    let uploadProgress!: (loaded: number, total: number) => void;
    state.pendingUpload = (progress: typeof uploadProgress) => {
        uploadProgress = progress;
        entered();
        return new Promise<void>((_resolve, reject) => { rejectUpload = reject; });
    };
    const pending = api.replaceNodeMedia(original.id, new File(["[]"], "replacement.json"));
    await started;
    const edited = {
        ...nodesRef.current[0], title: "renamed while uploading", position: { x: 42, y: 99 },
        ...(editUploadFields ? { width: 456, height: 654 } : {}),
        metadata: { ...nodesRef.current[0].metadata, content: "edited text", dataSource: { format: "json" as const, rawText: '[{"edited":true}]' },
            ...(editUploadFields ? { status: "success" as const, size: "user size", errorDetails: "user error", fileUpload: "error" as const, fileUploadProgress: 37 } : {}) },
    };
    nodesRef.current = [edited, nodesRef.current[1]];
    uploadProgress(1, 2);
    rejectUpload(new Error("upload failed"));
    expect(await pending).toBe(false);
    expect(nodesRef.current[0]).toEqual({ ...original, ...edited,
        width: editUploadFields ? 456 : original.width, height: editUploadFields ? 654 : original.height,
        metadata: { ...original.metadata, content: edited.metadata.content, dataSource: edited.metadata.dataSource,
            ...(editUploadFields ? { status: "success", size: "user size", errorDetails: "user error", fileUpload: "error", fileUploadProgress: 37 } : {}) },
    });
    expect(nodesRef.current[1]).toEqual({ ...existing(), id: "other" });
}));

function sizedFile(bytes: number) {
    const file = new File(["[]"], "large.json");
    Object.defineProperty(file, "size", { value: bytes });
    let reads = 0;
    file.text = async () => { reads++; return "[]"; };
    return { file, reads: () => reads };
}

test("shared reader uses the current exclusive runtime upload limit, before text()", async () => {
    const previous = useUserStore.getState().runtimeLimits;
    try {
        for (const mb of [1, 64]) {
            useUserStore.getState().setRuntimeLimits({ ...previous, resourceUploadMB: mb });
            const limit = mb * 1024 * 1024;
            for (const bytes of [limit, limit + 1]) {
                const sample = sizedFile(bytes);
                await expect(readDataSourceFile(sample.file)).rejects.toThrow(`小于 ${mb}MB`);
                expect(sample.reads()).toBe(0);
            }
            const below = sizedFile(limit - 1);
            expect((await readDataSourceFile(below.file)).rawText).toBe("[]");
            expect(below.reads()).toBe(1);
        }
    } finally { useUserStore.getState().setRuntimeLimits(previous); }
});

test.each(["selection", "input", "drop", "node"])("%s rejects over-limit data before reading or uploading", async (entry) => withWindow(async () => {
    const previous = useUserStore.getState().runtimeLimits;
    try {
        useUserStore.getState().setRuntimeLimits({ ...previous, resourceUploadMB: 1 });
        const sample = sizedFile(1024 * 1024);
        const original = { ...existing(), type: CanvasNodeType.DataSource };
        const { api, nodesRef } = harness([original]);
        if (entry === "selection") expect(await api.handleUploadFiles([sample.file])).toBe(false);
        else if (entry === "input") await api.handleImageInputChange({ target: { files: [sample.file], value: "large.json" } } as unknown as ChangeEvent<HTMLInputElement>);
        else if (entry === "drop") {
            api.handleDrop({ preventDefault() {}, stopPropagation() {}, target: null, clientX: 400, clientY: 400,
                dataTransfer: { files: [sample.file], getData: () => "" } } as unknown as DragEvent<HTMLDivElement>);
            await settled();
        } else {
            let changes = 0;
            state.updateNode = () => { changes++; };
            let onChange!: (event: ChangeEvent<HTMLInputElement>) => Promise<void>;
            function Capture() {
                const element = DataSourceNodeContent({ node: original, theme: canvasThemes.light }) as ReactElement<{ children: ReactElement[] }>;
                onChange = (element.props.children[0].props as { onChange: typeof onChange }).onChange;
                return null;
            }
            renderToStaticMarkup(<Capture />);
            await onChange({ currentTarget: { files: [sample.file], value: "large.json" } } as unknown as ChangeEvent<HTMLInputElement>);
            expect(changes).toBe(0);
        }
        expect(sample.reads()).toBe(0);
        expect(state.uploads).toEqual([]);
        expect(nodesRef.current[0]).toBe(original);
    } finally { useUserStore.getState().setRuntimeLimits(previous); }
}));

test("node-local import validates the same JSONL before modifying existing data", async () => {
    const original: CanvasNodeData = { ...existing(), type: CanvasNodeType.DataSource, metadata: { dataSource: { format: "json", rawText: "[]" } } };
    let node = original;
    state.updateNode = (_id: string, updater: (current: CanvasNodeData) => CanvasNodeData) => { node = updater(node); };
    let onChange!: (event: ChangeEvent<HTMLInputElement>) => Promise<void>;
    function Capture() {
        const element = DataSourceNodeContent({ node, theme: canvasThemes.light }) as ReactElement<{ children: ReactElement[] }>;
        onChange = (element.props.children[0].props as { onChange: typeof onChange }).onChange;
        return null;
    }
    renderToStaticMarkup(<Capture />);
    await onChange({ currentTarget: { files: [new File(['{}\n\n{bad}'], "rows.jsonl")], value: "rows.jsonl" } } as unknown as ChangeEvent<HTMLInputElement>);
    expect(node).toBe(original);
    await onChange({ currentTarget: { files: [new File([tsv], "rows.tsv")], value: "rows.tsv" } } as unknown as ChangeEvent<HTMLInputElement>);
    expect(node.metadata?.dataSource).toMatchObject({ format: "csv", rawText: await new File([tsv], "rows.tsv").text(), fileName: "rows.tsv" });
    expect(parseDataSourceNode(node).items).toEqual([{ name: "a\tb", value: 2, empty: "" }]);
});
