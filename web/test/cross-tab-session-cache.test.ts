import { afterEach, beforeEach, expect, spyOn, test } from "bun:test";
import { getAuthSession, invalidateAuthSessionCache } from "../src/services/api/auth";
import { cachedConversations, listConversations, writeCachedConversations, type FlowConversation } from "../src/services/api/intelligence";
import { apiClient, http } from "../src/services/api/request";
import { AUTH_SESSION_CHANGE_KEY, beginUserSessionCheck, getActiveUserScope, scopedLocalStorage, setActiveUserScope } from "../src/lib/user-scope";
import { beginGenerationConsumer, runGenerationConsumer } from "../src/services/generation-consumer-lifecycle";
import { subscribeAgentEvents } from "../src/services/api/agent";
import { useUserStore } from "../src/stores/use-user-store";

const originalWindow = globalThis.window;
const originalStorage = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
const originalScope = originalWindow ? getActiveUserScope() : null;
const originalUserState = useUserStore.getState();
let values: Map<string, string>;
const user = (id: string) => ({ id, username: id, displayName: id, role: "user" as const, status: "active" as const });
function deferred<T>() {
    let resolve!: (value: T) => void;
    const promise = new Promise<T>((done) => { resolve = done; });
    return { promise, resolve };
}
beforeEach(() => {
    values = new Map();
    const storage = { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value); }, removeItem: (key: string) => { values.delete(key); } };
    Object.defineProperty(globalThis, "localStorage", { configurable: true, value: storage });
    globalThis.window = { localStorage: storage } as unknown as Window & typeof globalThis;
    useUserStore.getState().setUser(user("alice"));
    invalidateAuthSessionCache();
    setActiveUserScope("alice");
});
afterEach(() => {
    useUserStore.getState().clearSession();
    invalidateAuthSessionCache(false);
    // Do not bind the guest scope to this test's invalidation nonce, then
    // restore a window where that nonce does not exist.
    values.delete(AUTH_SESSION_CHANGE_KEY);
    setActiveUserScope(null);
    globalThis.window = originalWindow;
    if (originalWindow) setActiveUserScope(originalScope);
    useUserStore.setState(originalUserState);
    if (originalStorage) Object.defineProperty(globalThis, "localStorage", originalStorage);
    else Reflect.deleteProperty(globalThis, "localStorage");
});

test("another tab changing shared scope does not redirect this tab's queued local writes", () => {
    values.set("yingce:active-user-scope", "bob");
    expect(getActiveUserScope()).toBe("alice");
    scopedLocalStorage.setItem("draft", "alice local edit");
    expect(values.get("draft:user:alice")).toBe("alice local edit");
    expect(values.has("draft:user:bob")).toBe(false);
});

test("invalidating auth cache detaches an old in-flight session request", async () => {
    const old = deferred<any>();
    const fresh = deferred<any>();
    const get = spyOn(http, "get").mockImplementationOnce(() => old.promise).mockImplementationOnce(() => fresh.promise);
    try {
        const first = getAuthSession();
        const firstResult = first.then(() => "applied", (error) => error.name);
        invalidateAuthSessionCache();
        const second = getAuthSession();
        fresh.resolve({ user: user("bob") });
        old.resolve({ user: user("alice") });
        expect(get).toHaveBeenCalledTimes(2);
        expect(await firstResult).toBe("AbortError");
        expect((await second).user?.id).toBe("bob");
        expect((await getAuthSession()).user?.id).toBe("bob");
    } finally { get.mockRestore(); }
});

test("a cross-tab revision detaches old auth work before the storage listener runs", async () => {
    const old = deferred<any>();
    const fresh = deferred<any>();
    const get = spyOn(http, "get").mockImplementationOnce(() => old.promise).mockImplementationOnce(() => fresh.promise);
    try {
        const first = getAuthSession().then(() => "applied", (error) => error.name);
        values.set(AUTH_SESSION_CHANGE_KEY, "other-tab-login");
        const second = getAuthSession();
        old.resolve({ user: user("alice") });
        expect(await first).toBe("AbortError");
        expect(getAuthSession()).toBe(second);
        fresh.resolve({ user: user("bob") });
        expect((await second).user?.id).toBe("bob");
        expect(get).toHaveBeenCalledTimes(2);
    } finally { get.mockRestore(); }
});

test("late conversation response cannot populate the next user's cache or the old user's cache", async () => {
    const pending = deferred<FlowConversation[]>();
    const get = spyOn(http, "get").mockImplementationOnce(() => pending.promise);
    try {
        const result = listConversations().then(() => "applied", (error) => error.name);
        setActiveUserScope("bob");
        useUserStore.getState().setUser(user("bob"));
        pending.resolve([{ id: "alice-private", title: "alice only" } as FlowConversation]);
        expect(await result).toBe("AbortError");
        expect(cachedConversations("alice")).toEqual([]);
        expect(cachedConversations("bob")).toEqual([]);
    } finally { get.mockRestore(); }
});

test("logging back into the same account does not revive a previous conversation request", async () => {
    const pending = deferred<FlowConversation[]>();
    const get = spyOn(http, "get").mockImplementationOnce(() => pending.promise);
    try {
        const result = listConversations().then(() => "applied", (error) => error.name);
        beginUserSessionCheck();
        setActiveUserScope("alice");
        pending.resolve([]);
        expect(await result).toBe("AbortError");
    } finally { get.mockRestore(); }
});

test("protected HTTP requests stop before transport and abort an existing transport on invalidation", async () => {
    const pending = deferred<any>();
    const transport = spyOn(apiClient, "request").mockImplementationOnce(() => pending.promise);
    try {
        const result = http.get("/agent/conversations").then(() => "applied", (error) => error.name);
        const signal = transport.mock.calls[0][0].signal as AbortSignal;
        beginUserSessionCheck();
        expect(signal.aborted).toBe(true);
        await expect(http.post("/canvas/projects", {})).rejects.toHaveProperty("name", "AbortError");
        expect(transport).toHaveBeenCalledTimes(1);
        pending.resolve({ data: { code: 0, data: [] } });
        expect(await result).toBe("AbortError");
    } finally { transport.mockRestore(); }
});

test("scope-key drift blocks server requests even before the storage event is delivered", async () => {
    const transport = spyOn(apiClient, "request").mockResolvedValue({ data: { code: 0, data: [] } } as any);
    try {
        values.set("yingce:active-user-scope", "bob");
        await expect(http.get("/agent/conversations")).rejects.toHaveProperty("name", "AbortError");
        expect(transport).not.toHaveBeenCalled();
    } finally { transport.mockRestore(); }
});

test("generation consumers are cancelled and cannot restart while identity is unconfirmed", async () => {
    const handle = beginGenerationConsumer();
    beginUserSessionCheck();
    try {
        expect(handle.signal.aborted).toBe(true);
        await expect(runGenerationConsumer(undefined, async () => "must not execute")).rejects.toHaveProperty("name", "AbortError");
    } finally { handle.release(); }
});

test("same-account directory failure retains its warm cache and an explicit wrong account is refused", async () => {
    const list = [{ id: "alice-private", title: "alice only" } as FlowConversation];
    writeCachedConversations(list);
    const get = spyOn(http, "get").mockRejectedValueOnce(new Error("offline"));
    try {
        await expect(listConversations()).rejects.toThrow("offline");
        expect(cachedConversations()).toEqual(list);
        await expect(listConversations("bob")).rejects.toHaveProperty("name", "AbortError");
        expect(get).toHaveBeenCalledTimes(1);
    } finally { get.mockRestore(); }
});

test("an existing agent stream is aborted when its account becomes unconfirmed", async () => {
    const pending = deferred<Response>();
    const fetch = spyOn(globalThis, "fetch").mockImplementationOnce(() => pending.promise);
    const stop = subscribeAgentEvents("unit-only-no-model", () => {});
    try {
        const signal = (fetch.mock.calls[0][1] as RequestInit).signal as AbortSignal;
        beginUserSessionCheck();
        expect(signal.aborted).toBe(true);
    } finally {
        stop();
        pending.resolve(new Response("", { headers: { "content-type": "text/event-stream" } }));
        await Promise.resolve();
        fetch.mockRestore();
    }
});
