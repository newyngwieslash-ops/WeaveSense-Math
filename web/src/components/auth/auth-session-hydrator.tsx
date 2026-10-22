import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { Button } from "antd";

import { getAuthSession, invalidateAuthSessionCache } from "@/services/api/auth";
import { FullScreenLoader } from "@/components/ui/aceternity/full-screen-loader";
import { preloadWorkspaceRoute } from "@/lib/workspace-route-modules";
import { useUserStore } from "@/stores/use-user-store";
import { recordDiagnosticEvent } from "@/services/diagnostics/client-diagnostics";
import { ACTIVE_USER_SCOPE_KEY, AUTH_SESSION_CHANGE_KEY, beginUserSessionCheck, captureUserSession, completeUserSessionCheck, getActiveUserScope, readAuthSessionRevision } from "@/lib/user-scope";

export function AuthSessionHydrator({ children }: { children: ReactNode }) {
    const hydrated = useUserStore((state) => state.hydrated);
    const [checking, setChecking] = useState(false);
    const [sessionError, setSessionError] = useState(false);
    const [retry, setRetry] = useState(0);

    useEffect(() => {
        let cancelled = false;
        let sequence = 0;
        const refresh = async (force: boolean, identityChanged = true) => {
            const current = ++sequence;
            const previous = captureUserSession();
            const revision = readAuthSessionRevision();
            const invalidating = force && (identityChanged || previous.revision !== revision || previous.scope !== (window.localStorage.getItem(ACTIVE_USER_SCOPE_KEY) || "guest"));
            if (force) {
                if (invalidating) beginUserSessionCheck();
                invalidateAuthSessionCache(false);
            }
            if (invalidating || !useUserStore.getState().hydrated) setChecking(true);
            setSessionError(false);
            try {
                const payload = await getAuthSession();
                if (cancelled || current !== sequence || revision !== readAuthSessionRevision()) return;
                const unchanged = useUserStore.getState().hydrated && (useUserStore.getState().user?.id || "guest") === (payload.user?.id || "guest") && previous.revision === revision && previous.scope === (window.localStorage.getItem(ACTIVE_USER_SCOPE_KEY) || "guest");
                if (unchanged) {
                    completeUserSessionCheck();
                } else {
                    beginUserSessionCheck();
                    setChecking(true);
                    const { applyUserSession } = await import("@/lib/user-session");
                    if (cancelled || current !== sequence || revision !== readAuthSessionRevision()) return;
                    await applyUserSession(payload);
                }
                if (!cancelled && current === sequence && revision === readAuthSessionRevision()) {
                    setChecking(false);
                    preloadWorkspaceRoute(window.location.pathname);
                    recordDiagnosticEvent({ category: "navigation", level: "info", code: "startup.auth_session_ready", message: "认证会话已确认" });
                }
            } catch {
                if (!cancelled && current === sequence) {
                    if (!invalidating && useUserStore.getState().hydrated) return;
                    // Preserve local drafts and the old scope, but keep its UI and network access gated.
                    beginUserSessionCheck();
                    setChecking(false);
                    setSessionError(true);
                }
            }
        };
        const onChange = () => { void refresh(true); };
        const onStorage = (event: StorageEvent) => {
            if (event.key === AUTH_SESSION_CHANGE_KEY || (event.key === ACTIVE_USER_SCOPE_KEY && event.newValue !== getActiveUserScope())) onChange();
        };
        const onFocus = () => { if (document.visibilityState === "visible") void refresh(true, false); };
        window.addEventListener("storage", onStorage);
        window.addEventListener(AUTH_SESSION_CHANGE_KEY, onChange);
        window.addEventListener("focus", onFocus);
        document.addEventListener("visibilitychange", onFocus);
        void refresh(retry > 0);
        return () => {
            cancelled = true;
            window.removeEventListener("storage", onStorage);
            window.removeEventListener(AUTH_SESSION_CHANGE_KEY, onChange);
            window.removeEventListener("focus", onFocus);
            document.removeEventListener("visibilitychange", onFocus);
        };
    }, [retry]);

    if (sessionError) return <div role="alert" className="flex min-h-screen flex-col items-center justify-center gap-4">
            <p>暂时无法确认登录身份，原账号的本地编辑已保留。</p>
            <Button onClick={() => setRetry((value) => value + 1)}>重新确认登录</Button>
        </div>;
    // Unmount protected content: a hidden parent cannot hide portals mounted under body.
    return checking || !hydrated ? <FullScreenLoader /> : children;
}
