export const ACTIVE_USER_SCOPE_KEY = "yingce:active-user-scope";
export const AUTH_SESSION_CHANGE_KEY = "yingce:auth-session-change";
const GUEST_SCOPE = "guest";
let userScopedPersistenceSuppressionDepth = 0;
let activeScope: string | undefined;
let activeRevision: string | undefined;
let scopeGeneration = 0;
let checkingSession = false;
let sessionRequests = new AbortController();

export function beginUserSessionCheck() {
    checkingSession = true;
    scopeGeneration += 1;
    sessionRequests.abort();
    sessionRequests = new AbortController();
}

export function completeUserSessionCheck() {
    checkingSession = false;
}

export function readAuthSessionRevision() {
    if (typeof window === "undefined") return "";
    try { return window.localStorage.getItem(AUTH_SESSION_CHANGE_KEY) || ""; } catch { return ""; }
}

export function publishAuthSessionChange() {
    if (typeof window === "undefined") return;
    beginUserSessionCheck();
    // Only an invalidation nonce is shared; the server remains the identity authority.
    try { window.localStorage.setItem(AUTH_SESSION_CHANGE_KEY, crypto.randomUUID()); } catch { /* focus revalidation remains available */ }
    window.dispatchEvent?.(new Event(AUTH_SESSION_CHANGE_KEY));
}

export function captureUserSession() {
    const scope = getActiveUserScope();
    activeRevision ??= readAuthSessionRevision();
    return { scope, revision: activeRevision, generation: scopeGeneration, signal: sessionRequests.signal };
}

export function assertUserSessionCurrent(session = captureUserSession()) {
    const storedScope = typeof window === "undefined" ? GUEST_SCOPE : window.localStorage.getItem(ACTIVE_USER_SCOPE_KEY) || GUEST_SCOPE;
    if (checkingSession || session.scope !== getActiveUserScope() || session.scope !== storedScope || session.generation !== scopeGeneration || session.revision !== readAuthSessionRevision()) {
        throw new DOMException("账号会话已变化，请重新确认身份", "AbortError");
    }
}

export function withUserScopedPersistenceSuppressed<T>(operation: () => T) {
    userScopedPersistenceSuppressionDepth += 1;
    try {
        return operation();
    } finally {
        userScopedPersistenceSuppressionDepth -= 1;
    }
}

export function isUserScopedPersistenceSuppressed() {
    return userScopedPersistenceSuppressionDepth > 0;
}

export function getActiveUserScope() {
    if (typeof window === "undefined") return GUEST_SCOPE;
    // A different tab must not redirect this tab's queued writes to its account.
    return activeScope ??= window.localStorage.getItem(ACTIVE_USER_SCOPE_KEY) || GUEST_SCOPE;
}

export function setActiveUserScope(userId?: string | null) {
    if (typeof window === "undefined") return;
    activeScope = userId || GUEST_SCOPE;
    activeRevision = readAuthSessionRevision();
    scopeGeneration += 1;
    completeUserSessionCheck();
    window.localStorage.setItem(ACTIVE_USER_SCOPE_KEY, activeScope);
}

export function scopedStorageKey(name: string, scope = getActiveUserScope()) {
    return `${name}:user:${scope}`;
}

export const scopedLocalStorage = {
    getItem: (name: string) => {
        if (typeof window === "undefined") return null;
        return window.localStorage.getItem(scopedStorageKey(name));
    },
    setItem: (name: string, value: string) => {
        if (typeof window === "undefined") return;
        if (isUserScopedPersistenceSuppressed()) return;
        window.localStorage.setItem(scopedStorageKey(name), value);
    },
    removeItem: (name: string) => {
        if (typeof window === "undefined") return;
        if (isUserScopedPersistenceSuppressed()) return;
        window.localStorage.removeItem(scopedStorageKey(name));
    },
};
