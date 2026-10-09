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
      const t = localStorage.getItem("sigarda_token");
      const u = localStorage.getItem("sigarda_user");
      if (t && u) {
        token = t;
        user = JSON.parse(u) as User;
        document.cookie = `panel_token=${t}; path=/; SameSite=Lax; max-age=604800`;
        log(
          "session loaded from localStorage:",
          user?.username,
          "token=" + t.slice(0, 8) + "...",
        );
      } else {
        log("no session in localStorage");
      }
    } catch (e) {
      log("failed to parse localStorage session:", e);
    }
  }

  async function validateToken(tok: string) {
    log("validating token:", tok.slice(0, 8) + "...");
    const res = await fetch("/api/proxy/auth/me", {
      method: "GET",
      headers: {
        Authorization: `Bearer ${tok}`,
        "Content-Type": "application/json",
      },
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
      } as User,
      isImpersonating: !!json.data.impersonated_by,
      impersonator: json.data.impersonator || null,
    };
  }

  async function init() {
    log("init() called");
    loadSession();
    if (token && user) {
      try {
        const result = await validateToken(token);
        if (result) {
          consecutiveFailures = 0;
          if (result.isImpersonating) {
            isImpersonating = true;
            impersonatingUser = result.user;
            impersonator = result.impersonator;
            const orig = localStorage.getItem("sigarda_imp_token");
            if (orig) originalToken = orig;
          } else {
            isImpersonating = false;
            impersonatingUser = null;
            impersonator = null;
            localStorage.removeItem("sigarda_imp_token");
            localStorage.removeItem("sigarda_imp_user");
          }
          user = result.user;
          localStorage.setItem("sigarda_user", JSON.stringify(result.user));
          log("init() session valid, user:", user?.username);
        } else {
          // Token invalid (401/403) — count failures
          consecutiveFailures++;
          log(
            `init() token invalid, failures: ${consecutiveFailures}/${MAX_FAILURES}`,
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
    }
    loading = false;
    log(
      "init() done, token:",
      token ? token.slice(0, 8) + "..." : "null",
      "user:",
      user?.username || "null",
    );
  }

  function clearSession() {
    token = null;
    user = null;
    isImpersonating = false;
    impersonatingUser = null;
    impersonator = null;
    originalToken = null;
    consecutiveFailures = 0;
    localStorage.removeItem("sigarda_token");
    document.cookie = "panel_token=; path=/; max-age=0";
    localStorage.removeItem("sigarda_user");
    localStorage.removeItem("sigarda_imp_token");
    localStorage.removeItem("sigarda_imp_user");
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
  ) {
    log("login() called for:", username, "remember_me:", remember_me);
    const res = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password, remember_me }),
    });
    const json = await res.json();
    if (json.error) throw new Error(json.error.message || json.error.code);
    const t = json.data.token;
    const u = json.data.user as User;
    token = t;
    user = u;
    consecutiveFailures = 0;
    localStorage.setItem("sigarda_token", t);
    document.cookie = `panel_token=${t}; path=/; SameSite=Lax; max-age=604800`;
    localStorage.setItem("sigarda_user", JSON.stringify(u));
    log("login() success, token:", t.slice(0, 8) + "...");
  }

  function logout() {
    log("logout() called");
    if (isImpersonating) {
      stopImpersonation();
      return;
    }
    if (token) {
      fetch("/api/proxy/auth/logout", {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
      }).catch(() => {});
    }
    clearAuth();
  }

  async function stopImpersonation() {
    log("stopImpersonation() called");
    try {
      await fetch("/api/proxy/auth/stop-impersonation", {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
      });
    } catch {}

    const orig = originalToken || localStorage.getItem("sigarda_imp_token");
    if (orig) {
      try {
        const result = await validateToken(orig);
        if (result?.user) {
          token = orig;
          user = result.user;
          localStorage.setItem("sigarda_token", orig);
          localStorage.setItem("sigarda_user", JSON.stringify(result.user));
        } else {
          clearAuth();
        }
      } catch {
        clearAuth();
      }
    } else {
      clearAuth();
    }

    isImpersonating = false;
    impersonatingUser = null;
    impersonator = null;
    originalToken = null;
    localStorage.removeItem("sigarda_imp_token");
    localStorage.removeItem("sigarda_imp_user");
  }

  async function impersonateUser(targetUserId: number) {
    try {
      const res = await fetch(`/api/proxy/users/${targetUserId}/impersonate`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
      });
      const json = await res.json();
      if (json.error) throw new Error(json.error.message || json.error.code);
      const {
        token: impToken,
        user: impUser,
        impersonator: impInfo,
      } = json.data;

      if (token) {
        originalToken = token;
        localStorage.setItem("sigarda_imp_token", token);
      }

      token = impToken;
      user = impUser as User;
      isImpersonating = true;
      impersonatingUser = impUser as User;
      impersonator = impInfo || null;
      localStorage.setItem("sigarda_token", impToken);
      localStorage.setItem("sigarda_user", JSON.stringify(impUser));
    } catch (e: any) {
      throw new Error(e.message || "impersonation failed");
    }
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
    clearSession, // expose for api.ts handler
    clearAuth, // full redirect
  };
}

export const auth = createAuthStore();

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
