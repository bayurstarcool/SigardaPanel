import { env } from "$env/dynamic/private";

/** Base URL of the Go API (override with SIGARDAPANEL_API_URL). */
export const API_URL = env.SIGARDAPANEL_API_URL || "http://127.0.0.1:7700";

/**
 * Headers that carry the real client IP to the API. nginx must always set
 * X-Real-IP ($remote_addr) on the locations that reach this app, otherwise a
 * client-supplied value would be forwarded. Without it the API sees every
 * browser as 127.0.0.1 and its per-IP login limiter becomes global.
 */
export function clientIpHeaders(request: Request): Record<string, string> {
  const ip = request.headers.get("x-real-ip");
  return ip ? { "X-Real-IP": ip } : {};
}

/** HttpOnly cookie holding an impersonation session's token (set by /api/impersonate). */
export const IMPERSONATION_COOKIE = "sp_imp";

function readCookie(request: Request, name: string): string | null {
  const cookie = request.headers.get("cookie") ?? "";
  const m = cookie.match(new RegExp(`(?:^|;\\s*)${name}=([^;]+)`));
  return m ? decodeURIComponent(m[1]) : null;
}

/** The admin's own session token (sp_session), ignoring any impersonation. */
export function sessionToken(request: Request): string | null {
  return readCookie(request, "sp_session");
}

/** The impersonation token, when an impersonation is active in this browser. */
export function impersonationToken(request: Request): string | null {
  return readCookie(request, IMPERSONATION_COOKIE);
}

/**
 * Authorization for the upstream API: the browser's own header when present
 * (API clients), else the impersonation cookie (an admin acting as a user;
 * it survives page reloads), else the HttpOnly sp_session cookie. Tokens never
 * have to live in page JS.
 */
export function authHeaders(request: Request): Record<string, string> {
  const auth = request.headers.get("Authorization");
  if (auth) return { Authorization: auth };
  const tok = impersonationToken(request) ?? sessionToken(request);
  return tok ? { Authorization: `Bearer ${tok}` } : {};
}

/**
 * True unless the browser says the request comes from another origin. A
 * cookie-authenticated unsafe request from a sibling subdomain carries the
 * SameSite=Strict cookie (same site) but a different Origin: refusing it here
 * is the CSRF check, since the API's own guard skips requests that carry an
 * Authorization header (which this server adds from the cookie). Clients that
 * send neither header (curl, the CLI) are not browsers and carry no cookie
 * set by a victim's browser.
 */
export function sameOrigin(request: Request, url: URL): boolean {
  const origin = request.headers.get("origin");
  if (origin) {
    if (origin === url.origin) return true;
    // The panel may be opened by IP as well as by its configured ORIGIN:
    // the Host the browser addressed (nginx passes it) is the same origin too.
    const host = request.headers.get("host");
    try {
      return !!host && new URL(origin).host === host;
    } catch {
      return false;
    }
  }
  const site = request.headers.get("sec-fetch-site");
  return !site || site === "same-origin" || site === "none";
}
