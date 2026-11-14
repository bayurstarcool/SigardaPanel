import type { User, ImpersonatorInfo } from "./types";
import { setSessionExpiredHandler } from "./api";

function log(...args: any[]) {
  console.log("%c[AUTH]", "color: #f59e0b; font-weight: bold", ...args);
}

function createAuthStore() {
  let token = $state<string | null>(null);
  let user = $state<User | null>(null);
  let loading = $state(true);
  let isImpersonating = $state(false);
  let impersonatingUser = $state<User | null>(null);
  let impersonator = $state<ImpersonatorInfo | null>(null);
  let originalToken = $state<string | null>(null);

  // Track consecutive validation failures — only clear after 2 failures
  let consecutiveFailures = 0;
  const MAX_FAILURES = 2;

  function loadSession() {
    try {
      // Only the display-name cache lives here; the session token now lives
      // in the HttpOnly sp_session cookie and never touches JS storage.
      const u = localStorage.getItem("sigarda_user");
      if (u) {
        user = JSON.parse(u) as User;
        log("user cache loaded from localStorage:", user?.username);
      } else {
        log("no cached user in localStorage");
      }
    } catch (e) {
      log("failed to parse localStorage session:", e);
    }
  }

  async function validateToken(tok: string | null) {
    if (tok) log("validating token:", tok.slice(0, 8) + "...");
    else log("validating session cookie via /auth/me...");
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };
    // Impersonation still rides on an explicit Bearer token; normal
    // sessions authenticate with the HttpOnly cookie.
    if (tok) headers["Authorization"] = `Bearer ${tok}`;
    const res = await fetch("/api/proxy/auth/me", {
      method: "GET",
      headers,
      cache: "no-store",
    });

    log("/auth/me response:", res.status);

    // Network/proxy error — throw so caller knows it's transient
    if (!res.ok && res.status !== 401 && res.status !== 403) {
      throw new Error(`server error ${res.status}`);
    }

    if (res.status === 401 || res.status === 403) {
      const json = await res.json().catch(() => null);
      log("/auth/me rejected:", json?.error?.message || "unknown");
      return null;
    }

    const json = await res.json();
    if (json.error || !json.data) {
      log("/auth/me no data:", json);
      return null;
    }
    log("/auth/me OK, user:", json.data.username);
    return {
      user: {
        id: json.data.id,
        username: json.data.username,
        email: json.data.email,
        role: json.data.role,
        permissions: Array.isArray(json.data.permissions) ? json.data.permissions : [],
        password_change_required: !!json.data.password_change_required,
      } as User,
      isImpersonating: !!json.data.impersonated_by,
      impersonator: json.data.impersonator || null,
    };
  }

  // The session behind the cookies: impersonation is kept in the HttpOnly
  // sp_imp cookie (see /api/impersonate), so it survives page reloads.
  function applySession(result: NonNullable<Awaited<ReturnType<typeof validateToken>>>) {
    token = null;
    user = result.user;
    isImpersonating = result.isImpersonating;
    impersonatingUser = result.isImpersonating ? result.user : null;
    impersonator = result.isImpersonating ? result.impersonator : null;
    localStorage.setItem("sigarda_user", JSON.stringify(result.user));
  }

  // Clears the impersonation cookie (and its server session). Returns false
  // when the request could not be made.
  async function clearImpersonationCookie(): Promise<boolean> {
    try {
      const res = await fetch("/api/impersonate", { method: "DELETE" });
      return res.ok;
    } catch {
      return false;
    }
  }

  async function init() {
    log("init() called");
    loadSession();
    // Validate whatever the cookie says — even without a cached user the
    // browser may still hold a valid session.
    try {
      let result = await validateToken(null);
      if (!result) {
        // An expired or revoked impersonation cookie hides the admin's own
        // session: drop it and try again with sp_session.
        if (await clearImpersonationCookie()) result = await validateToken(null);
      }
      if (result) {
        consecutiveFailures = 0;
        applySession(result);
        log("init() session valid, user:", user?.username, "impersonating:", isImpersonating);
      } else {
        // Session invalid (401/403) — count failures
        consecutiveFailures++;
        log(
          `init() session invalid, failures: ${consecutiveFailures}/${MAX_FAILURES}`,
        );
        if (consecutiveFailures >= MAX_FAILURES) {
          // Truly invalid — clear everything
          log("init() clearing session after", MAX_FAILURES, "failures");
          clearSession();
        }
        // Otherwise keep session — might be transient
      }
    } catch (e) {
      // Network error or transient failure — keep session
      log("init() network error, keeping session:", e);
    }
    loading = false;
    log(
      "init() done, user:",
      user?.username || "null",
      "impersonating:",
      isImpersonating,
    );
  }

  function clearSession() {
    const isImpersonatingAtClear = isImpersonating;
    token = null;
    user = null;
    isImpersonating = false;
    impersonatingUser = null;
    impersonator = null;
    originalToken = null;
    consecutiveFailures = 0;
    // Also wipe legacy keys from pre-cookie versions so old copies die.
    localStorage.removeItem("sigarda_token");
    localStorage.removeItem("sigarda_user");
    localStorage.removeItem("sigarda_imp_token");
    localStorage.removeItem("sigarda_imp_user");
    document.cookie = "panel_token=; path=/; max-age=0";
    // An impersonation cookie must not outlive the session it belongs to.
    if (isImpersonatingAtClear) {
      fetch("/api/impersonate", { method: "DELETE", keepalive: true }).catch(() => {});
    }
    log("clearSession() called");
  }

  function clearAuth() {
    clearSession();
    // Full redirect — dipanggil dari layout guard, bukan dari api.ts
    window.location.href = "/login";
  }

  async function login(
    username: string,
    password: string,
    remember_me?: boolean,
    second_factor?: { totp_code?: string; recovery_code?: string },
  ) {
    log("login() called for:", username, "remember_me:", remember_me);
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        username,
        password,
        remember_me,
        ...second_factor,
      }),
    });
    const json = await res.json();
    if (json.error) {
      const err = new Error(json.error.message || json.error.code) as Error & {
        code?: string;
      };
      err.code = json.error.code;
      throw err;
    }
    const t = json.data.token;
    const u = json.data.user as User;
    // The token arrives as an HttpOnly cookie via the login route's
    // Set-Cookie; keep it out of JS storage entirely.
    token = null;
    user = u;
    consecutiveFailures = 0;
    localStorage.setItem("sigarda_user", JSON.stringify(u));
    log("login() success (HttpOnly cookie session), user:", u.username);
  }

  // Called after the user set a new password: lifts the first-login prompt.
  function markPasswordChanged() {
    if (!user) return;
    user = { ...user, password_change_required: false };
    localStorage.setItem("sigarda_user", JSON.stringify(user));
  }

  async function logout() {
    log("logout() called");
    if (isImpersonating) {
      stopImpersonation();
      return;
    }
    // The HttpOnly cookie authenticates the call — no token in JS. Wait for
    // the server to revoke the session (keepalive lets it finish even if
    // the page navigates away); otherwise the cookie stays valid.
    try {
      await fetch("/api/proxy/auth/logout", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        keepalive: true,
      });
    } catch {}
    clearAuth();
  }

  async function stopImpersonation() {
    log("stopImpersonation() called");
    await clearImpersonationCookie();
    // The admin's own HttpOnly sp_session takes over again.
    try {
      const result = await validateToken(null);
      if (!result?.user) {
        clearAuth();
        return;
      }
      applySession(result);
    } catch {
      clearAuth();
      return;
    }
    originalToken = null;
    localStorage.removeItem("sigarda_imp_token");
    localStorage.removeItem("sigarda_imp_user");
  }

  async function impersonateUser(targetUserId: number) {
    // The web server keeps the impersonation token in an HttpOnly cookie;
    // page JS never sees it.
    const res = await fetch("/api/impersonate", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ user_id: targetUserId }),
    });
    const json = await res.json().catch(() => null);
    if (!res.ok || json?.error) {
      throw new Error(json?.error?.message || json?.error?.code || "impersonation failed");
    }
    const result = await validateToken(null);
    if (!result?.isImpersonating) {
      await clearImpersonationCookie();
      throw new Error("impersonation failed");
    }
    applySession(result);
  }

  return {
    get token() {
      return token;
    },
    get user() {
      return user;
    },
    get loading() {
      return loading;
    },
    get isImpersonating() {
      return isImpersonating;
    },
    get impersonatingUser() {
      return impersonatingUser;
    },
    get impersonator() {
      return impersonator;
    },
    init,
    login,
    logout,
    impersonateUser,
    stopImpersonation,
    markPasswordChanged,
    clearSession, // expose for api.ts handler
    clearAuth, // full redirect
  };
}

export const auth = createAuthStore();

/**
 * Authorization headers for direct fetch calls to /api/proxy/*.
 * Cookie sessions need none (the proxy derives them server-side); only
 * impersonation must carry its explicit Bearer token.
 */
export function bearerHeaders(): Record<string, string> {
  return auth.token ? { Authorization: `Bearer ${auth.token}` } : {};
}

// Register handler biar api.ts panggil clearSession() bukan redirect langsung
setSessionExpiredHandler((reason: string) => {
  log("session expired handler called:", reason);
  auth.clearSession();
  // Redirect via layout guard (reactive), bukan dari sini
});

export function hasRole(
  userRole: string | undefined,
  requiredRole: "super_admin" | "admin" | "user",
): boolean {
  if (!userRole) return false;
  if (requiredRole === "user") return true;
  if (requiredRole === "admin")
    return userRole === "super_admin" || userRole === "admin";
  if (requiredRole === "super_admin") return userRole === "super_admin";
  return false;
}

/**
 * Whether the signed-in user's role holds a permission (from /auth/me). Only
 * hides UI the API would refuse; the API checks every request itself. A
 * cached user without the list (older session) falls back to the built-in
 * role's capabilities through hasRole at the call site.
 */
export function hasPermission(permission: string): boolean {
  return !!auth.user?.permissions?.includes(permission);
}
