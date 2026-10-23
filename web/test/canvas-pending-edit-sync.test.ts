import { expect, spyOn, test } from "bun:test";
import localforage from "localforage";
import { canvasContentHash } from "@/lib/canvas/canvas-content";
import { ApiError, http } from "@/services/api/request";
import { initializeRemoteUserDataSession, loadCanvasProjectForEditing, refreshCanvasAfterAgent, resetRemoteUserDataSync, saveRemoteUserDataNow } from "@/services/user-data-sync";
import type { CanvasOperationsRequest } from "@/services/api/user-data";
import { useSyncProgressStore } from "@/stores/use-sync-progress-store";
import { flushCanvasStorePersistence, useCanvasStore, type CanvasProject } from "@/stores/canvas/use-canvas-store";
import { CanvasNodeType } from "@/types/canvas";

test.each(["合法正则", "节点标题", "显式直存", "自动保存", "Agent刷新", "提交中新编辑", "ACK未知重开", "确认读取失败", "哈希不符", "缺少哈希", "权限拒绝", "账号切换", "哈希不符直存", "缺少哈希直存", "权限拒绝直存", "另一图缺基线"])("脏缓存恢复保存边界：%s", async (scenario) => {
    const oldWindow = Object.getOwnPropertyDescriptor(globalThis, "window");
    const storage = new Map<string, unknown>();
    const reads = spyOn(localforage, "getItem").mockImplementation(async (key) => storage.get(key) ?? null);
    const writes = spyOn(localforage, "setItem").mockImplementation(async (key, value) => { storage.set(key, value); return value; });
    const removes = spyOn(localforage, "removeItem").mockImplementation(async (key) => { storage.delete(key); });
    Object.defineProperty(globalThis, "window", { configurable: true, value: {
        setTimeout, clearTimeout,
        localStorage: { length: 0, key: () => null, getItem: () => null, setItem: () => {}, removeItem: () => {} },
    } });
    const base: CanvasProject = {
        id: "pending-edit", revision: 5, title: "待同步", createdAt: "2026-10-23", updatedAt: "2026-10-23",
        nodes: [{ id: "regex", type: CanvasNodeType.Logic, title: "原标题", position: { x: 0, y: 0 }, width: 100, height: 100,
            metadata: { logic: { operator: "if_else", comparator: "regex", fieldPath: "name", compareValue: "*" } } }],
        connections: [], chatSessions: [], activeChatId: null, backgroundMode: "dots", showImageInfo: false,
        viewport: { x: 0, y: 0, k: 1 }, previsScenes: [],
    };
    let remote = base;
    const node = base.nodes[0]!;
    const local: CanvasProject = { ...base, remoteContentHash: await canvasContentHash(base), nodes: [scenario === "节点标题"
        ? { ...node, title: "本地合法标题" }
        : { ...node, metadata: { ...node.metadata, logic: { ...node.metadata!.logic!, compareValue: "^深层甲$" } } }] };
    if (scenario.startsWith("哈希不符")) remote = { ...base, title: "其他客户端已修改" };
    if (scenario.startsWith("缺少哈希")) delete local.remoteContentHash;
    let readCount = 0;
    const get = spyOn(http, "get").mockImplementation(async (url) => {
        if (url === "/canvas-projects/blocked") return { project: { ...base, id: "blocked" } };
        readCount += 1;
        if (scenario === "确认读取失败" && readCount > 1) throw new ApiError("确认暂时失败", { status: 503 });
        if (scenario.startsWith("权限拒绝")) throw new ApiError("无权限", { status: 403 });
        if (scenario === "账号切换") resetRemoteUserDataSync();
        return { project: remote };
    });
    const requests: CanvasOperationsRequest[] = [];
    const patch = spyOn(http, "patch").mockImplementation(async (_url, payload) => {
        const request = structuredClone(payload) as CanvasOperationsRequest;
        requests.push(request);
        if (requests.length > 1 && scenario === "ACK未知重开" && requests.length === 2) {
            expect(request).toEqual(requests[0]!);
            return { project: { id: base.id, revision: 6 } };
        }
        if (requests.length > 1) {
            expect(request.operations).toEqual([{ type: "patch_node", id: "regex", patch: { title: "请求期间新标题" }, before: { title: "原标题" } }]);
            remote = { ...remote, revision: 7, nodes: [{ ...remote.nodes[0]!, title: "请求期间新标题" }] };
            return { project: { id: base.id, revision: 7 } };
        }
        expect(request.operations).toEqual([{ type: "patch_node", id: "regex",
            patch: scenario === "节点标题" ? { title: "本地合法标题" } : { "metadata.logic.compareValue": "^深层甲$" },
            before: scenario === "节点标题" ? { title: "原标题" } : { "metadata.logic.compareValue": "*" } }]);
        remote = { ...local, revision: 6 };
        if (scenario === "提交中新编辑") useCanvasStore.setState({ projects: [{ ...local, nodes: [{ ...local.nodes[0]!, title: "请求期间新标题" }] }] });
        if (scenario === "ACK未知重开") throw new ApiError("响应丢失", { retryable: true });
        return { project: { id: base.id, revision: 6 } };
    });
    try {
        useCanvasStore.setState({ projects: scenario === "另一图缺基线" ? [{ ...local, id: "blocked", remoteContentHash: undefined }, local] : [local] });
        await flushCanvasStorePersistence();
        await initializeRemoteUserDataSession("pending-user");
        const loading = scenario === "Agent刷新" ? refreshCanvasAfterAgent(base.id) : scenario.endsWith("直存") ? saveRemoteUserDataNow(base.id) : loadCanvasProjectForEditing(base.id);
        if (["哈希不符", "缺少哈希", "权限拒绝"].some((value) => scenario.startsWith(value)) || scenario === "账号切换") {
            await expect(loading).rejects.toThrow(scenario.startsWith("权限拒绝") ? "无权限" : scenario === "账号切换" ? "账号已切换" : "可信画布合并基线");
            expect(useCanvasStore.getState().openProject(base.id)?.nodes).toEqual(local.nodes);
            expect(requests).toHaveLength(0);
            expect(remote.revision).toBe(5);
            return;
        }
        await loading;
        if (scenario === "另一图缺基线") {
            await expect(saveRemoteUserDataNow()).rejects.toThrow("可信画布合并基线");
            expect(remote.revision).toBe(6);
            expect(useCanvasStore.getState().openProject("blocked")?.nodes).toEqual(local.nodes);
            expect(useSyncProgressStore.getState().syncingProjects.blocked?.phase).toBe("error");
            expect(useSyncProgressStore.getState().syncingProjects[base.id]?.phase).toBe("done");
            return;
        }
        if (scenario === "ACK未知重开") {
            await expect(saveRemoteUserDataNow(base.id)).rejects.toThrow("响应丢失");
            expect(useSyncProgressStore.getState().syncingProjects[base.id]?.phase).toBe("error");
            resetRemoteUserDataSync();
            useCanvasStore.setState({ projects: [{ ...local, nodes: [{ ...local.nodes[0]!, title: "请求期间新标题" }] }] });
            await initializeRemoteUserDataSession("pending-user");
            await loadCanvasProjectForEditing(base.id);
        }
        if (scenario === "确认读取失败") {
            await expect(saveRemoteUserDataNow(base.id)).rejects.toThrow("确认暂时失败");
            expect(remote.revision).toBe(6);
            expect(useCanvasStore.getState().openProject(base.id)?.nodes).toEqual(local.nodes);
            expect(useSyncProgressStore.getState().syncingProjects[base.id]?.phase).toBe("error");
            return;
        }
        if (scenario === "自动保存") await new Promise((resolve) => setTimeout(resolve, 1500));
        else await saveRemoteUserDataNow(base.id);
        const laterEdit = ["提交中新编辑", "ACK未知重开"].includes(scenario);
        expect(remote.revision).toBe(laterEdit ? 7 : 6);
        expect(remote.nodes).toEqual(laterEdit ? [{ ...local.nodes[0]!, title: "请求期间新标题" }] : local.nodes);
        expect(useCanvasStore.getState().openProject(base.id)?.nodes).toEqual(remote.nodes);
        expect(useSyncProgressStore.getState().syncingProjects[base.id]?.phase).toBe("done");
    } finally {
        resetRemoteUserDataSync();
        await flushCanvasStorePersistence();
        get.mockRestore(); patch.mockRestore(); reads.mockRestore(); writes.mockRestore(); removes.mockRestore();
        if (oldWindow) Object.defineProperty(globalThis, "window", oldWindow);
        else Reflect.deleteProperty(globalThis, "window");
    }
});
