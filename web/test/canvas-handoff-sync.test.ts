import { expect, spyOn, test } from "bun:test";
import localforage from "localforage";
import { canvasContentHash } from "@/lib/canvas/canvas-content";

import { http } from "@/services/api/request";
import { initializeRemoteUserDataSession, loadCanvasProjectForEditing, resetRemoteUserDataSync, saveRemoteUserDataNow } from "@/services/user-data-sync";
import { flushCanvasStorePersistence, useCanvasStore, type CanvasProject } from "@/stores/canvas/use-canvas-store";
import { CanvasNodeType } from "@/types/canvas";

test.each(["本地标题", "本地删除", "干净缓存", "同字段冲突", "缺少可信基线", "账号切换"])("交接加载边界：%s", async (scenario) => {
    const originalWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
    const storage = new Map<string, unknown>();
    const reads = spyOn(localforage, "getItem").mockImplementation(async (key) => storage.get(key) ?? null);
    const writes = spyOn(localforage, "setItem").mockImplementation(async (key, value) => { storage.set(key, value); return value; });
    const removes = spyOn(localforage, "removeItem").mockImplementation(async (key) => { storage.delete(key); });
    Object.defineProperty(globalThis, "window", { configurable: true, value: {
        setTimeout, clearTimeout,
        localStorage: { length: 0, key: () => null, getItem: () => null, setItem: () => {}, removeItem: () => {} },
    } });
    const node = (id: string) => ({ id, type: CanvasNodeType.Text, title: id, position: { x: 0, y: 0 }, width: 100, height: 100, metadata: { content: id } });
    const base: CanvasProject = {
        id: "handoff-canvas", revision: 1, title: "交接验收", createdAt: "2026-10-20", updatedAt: "2026-10-20",
        nodes: [node("original")], connections: [], chatSessions: [], activeChatId: null,
        backgroundMode: "dots", showImageInfo: false, viewport: { x: 0, y: 0, k: 1 }, previsScenes: [],
    };
    const remote = { ...base, revision: 2, nodes: [...base.nodes, node("intel-handoff")] };
    if (scenario !== "缺少可信基线") base.remoteContentHash = await canvasContentHash(base);
    if (scenario === "同字段冲突") remote.title = "云端标题";
    const get = spyOn(http, "get").mockImplementation(async () => {
        if (scenario === "账号切换") resetRemoteUserDataSync();
        return { project: remote };
    });
    const patch = spyOn(http, "patch").mockResolvedValue({ project: { revision: 3 } });
    try {
        useCanvasStore.setState({ projects: [base] });
        await flushCanvasStorePersistence();
        await initializeRemoteUserDataSession("handoff-user");
        const local = scenario === "干净缓存" ? base : scenario === "本地删除" ? { ...base, nodes: [] } : { ...base, title: "本地未确认标题" };
        useCanvasStore.setState({ projects: [local] });
        let rendered: CanvasProject | undefined;
        const loading = loadCanvasProjectForEditing(base.id, { onLoad: (project) => { rendered = project; } });
        if (["同字段冲突", "缺少可信基线", "账号切换"].includes(scenario)) {
            await expect(loading).rejects.toThrow(scenario === "同字段冲突" ? "冲突" : scenario === "账号切换" ? "账号已切换" : "可信画布合并基线");
            expect(useCanvasStore.getState().openProject(base.id)).toBe(local);
            expect(rendered).toBeUndefined();
            expect(patch).not.toHaveBeenCalled();
            return;
        }
        const loaded = await loading;
        const stored = useCanvasStore.getState().openProject(base.id);
        await saveRemoteUserDataNow(base.id);
        if (scenario === "干净缓存") expect(patch).not.toHaveBeenCalled();
        else {
            expect(patch).toHaveBeenCalledTimes(1);
            expect((patch.mock.calls[0]![1] as { operations: unknown[] }).operations).toEqual(scenario === "本地删除" ? [{ type: "remove_node", id: "original", expected: base.nodes[0] }] : [{ type: "set_field", field: "title", value: local.title, beforeValue: base.title }]);
        }
        expect(loaded?.nodes.map((item) => item.id)).toEqual(scenario === "本地删除" ? ["intel-handoff"] : ["original", "intel-handoff"]);
        expect(loaded?.title).toBe(local.title);
        expect(rendered).toBe(loaded);
        expect(stored?.nodes).toEqual(loaded!.nodes);
    } finally {
        resetRemoteUserDataSync();
        await flushCanvasStorePersistence();
        get.mockRestore(); patch.mockRestore(); reads.mockRestore(); writes.mockRestore(); removes.mockRestore();
        if (originalWindow) Object.defineProperty(globalThis, "window", originalWindow);
        else Reflect.deleteProperty(globalThis, "window");
    }
});
