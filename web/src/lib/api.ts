// Auth-aware fetch with retry, re-validation, and grace period
// Multiple safeguards prevent false auto-logout

const MAX_RETRIES = 2;
const RETRY_DELAY_MS = 1500;

// Track logout state to prevent cascading redirects
let isLoggingOut = false;
let lastLogoutTime = 0;
const LOGOUT_COOLDOWN_MS = 5000;

// Track last successful validation time
let lastValidatedAt = 0;
const REVALIDATE_THRESHOLD_MS = 30_000; // if validated <30s ago, re-validate instead of logout

function log(...args: any[]) {
  console.log("%c[AUTH]", "color: #f59e0b; font-weight: bold", ...args);
}

function sleep(ms: number) {
  return new Promise((r) => setTimeout(r, ms));
}

// Callback untuk memberi tahu auth store bahwa sesi expired
// Dipasang dari auth.svelte.ts — bukan redirect langsung
let onSessionExpired: ((reason: string) => void) | null = null;

export function setSessionExpiredHandler(handler: (reason: string) => void) {
  onSessionExpired = handler;
}

function shouldLogout() {
  const now = Date.now();
  if (isLoggingOut) {
    log("blocked: already logging out");
    return false;
  }
  if (now - lastLogoutTime < LOGOUT_COOLDOWN_MS) {
    log("blocked: cooldown active");
    return false;
  }
  return true;
}

function doLogout(reason: string) {
  if (!shouldLogout()) return;
  log("LOGOUT triggered:", reason);
  isLoggingOut = true;
  lastLogoutTime = Date.now();
  localStorage.removeItem("sigarda_token");
  localStorage.removeItem("sigarda_user");
  localStorage.removeItem("sigarda_imp_token");
  localStorage.removeItem("sigarda_imp_user");

  // Notify auth store — clear token state (reactive)
  // Layout guard redirect ke /login via $effect
  if (onSessionExpired) {
    onSessionExpired(reason);
  } else {
    // Fallback: redirect langsung kalau ga ada handler
    window.location.href = "/login";
  }
}

// Re-validate token with /auth/me before giving up
async function revalidateToken(token: string): Promise<boolean> {
  try {
    log("re-validating token with /auth/me...");
    const res = await fetch("/api/proxy/auth/me", {
      method: "GET",
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      cache: "no-store",
    });
    if (res.ok) {
      const json = await res.json();
      if (json.data && !json.error) {
        log(
          "re-validation OK — session still valid, user:",
          json.data.username,
        );
        lastValidatedAt = Date.now();
        return true;
      }
    }
    log("re-validation failed:", res.status);
    return false;
  } catch (e) {
    log("re-validation network error:", e);
    return false; // can't confirm validity, but don't force logout on network error
  }
}

export function apiIsLoggingOut() {
  return isLoggingOut;
}

export function markValidated() {
  lastValidatedAt = Date.now();
}

export async function apiFetch(
  token: string | null,
  method: string,
  path: string,
  body?: any,
): Promise<any> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const opts: RequestInit = { method, headers };
  if (body && method !== "GET" && method !== "DELETE")
    opts.body = JSON.stringify(body);

  let lastError: any = null;

  for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
    if (isLoggingOut) throw new Error("session expired");

    try {
      const res = await fetch(`/api/proxy/${path}`, opts);

      if (res.status === 401) {
        const json = await res.json().catch(() => null);
        const msg = json?.error?.message || "";
        log(
          `401 on ${method} ${path}: "${msg}" (attempt ${attempt + 1}/${MAX_RETRIES + 1})`,
        );

        // Session truly expired/revoked — these are definitive, no retry
        if (msg === "session expired" || msg === "session revoked") {
          doLogout(`${msg} on ${method} ${path}`);
          throw new Error("session expired");
        }

        // Token invalid — but maybe it's a transient issue
        if (
          msg === "invalid token" ||
          msg === "missing authorization header" ||
          msg === "not authenticated"
        ) {
          // If we validated recently (<30s), try re-validating instead of retrying
          if (
            lastValidatedAt > 0 &&
            Date.now() - lastValidatedAt < REVALIDATE_THRESHOLD_MS
          ) {
            log("token was valid recently — re-validating with /auth/me...");
            const stillValid = await revalidateToken(token || "");
            if (stillValid) {
              // Token is actually valid, retry the original request
              log("re-validation passed — retrying original request...");
              continue;
            }
          }

          // Normal retry
          if (attempt < MAX_RETRIES) {
            log(`retrying ${method} ${path} in ${RETRY_DELAY_MS}ms...`);
            await sleep(RETRY_DELAY_MS);
            continue;
          }

          // All retries exhausted — now try re-validation as last resort
          if (token) {
            const stillValid = await revalidateToken(token);
            if (stillValid) {
              log(
                "re-validation passed after retries — token is valid, retrying once more...",
              );
              const retryRes = await fetch(`/api/proxy/${path}`, opts);
              if (retryRes.ok) {
                const retryJson = await retryRes.json();
                if (!retryJson.error) return retryJson.data;
              }
            }
          }

          doLogout(`${msg} (exhausted all retries) on ${method} ${path}`);
          throw new Error("session expired");
        }

        // Other 401 (rate limit, etc) — throw but don't logout
        throw new Error(msg || "unauthorized");
      }

      // Proxy error (502/503) — retry
      if (res.status === 502 || res.status === 503) {
        log(
          `proxy error ${res.status} on ${method} ${path} (attempt ${attempt + 1})`,
        );
        if (attempt < MAX_RETRIES) {
          await sleep(RETRY_DELAY_MS);
          continue;
        }
        throw new Error("server unavailable");
      }

      const json = await res.json();
      if (json.error) throw new Error(json.error.message || json.error.code);

      // Mark as validated on successful request
      lastValidatedAt = Date.now();
      return json.data;
    } catch (e: any) {
      lastError = e;
      // Network error — retry
      if (e.name === "TypeError" && attempt < MAX_RETRIES) {
        log(`network error on ${method} ${path}, retrying...`);
        await sleep(RETRY_DELAY_MS);
        continue;
      }
      throw e;
    }
  }

  throw lastError || new Error("request failed");
}
