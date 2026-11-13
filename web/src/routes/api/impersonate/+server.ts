import { json } from "@sveltejs/kit";
import type { RequestHandler } from "./$types";
import {
  API_URL,
  IMPERSONATION_COOKIE,
  clientIpHeaders,
  impersonationToken,
  sessionToken,
} from "$lib/server/upstream";

// Impersonation sessions are kept in an HttpOnly cookie (like sp_session) so
// they survive page reloads without the token ever reaching page JavaScript.
// The proxy prefers it over sp_session while it is set.

function sameOrigin(request: Request, url: URL): boolean {
  const origin = request.headers.get("origin");
  if (origin) return origin === url.origin;
  const site = request.headers.get("sec-fetch-site");
  return !site || site === "same-origin" || site === "none";
}

// POST {user_id}: start impersonating with the admin's own session.
export const POST: RequestHandler = async ({ request, cookies, url }) => {
  if (!sameOrigin(request, url)) return json({ error: { message: "cross-site request refused" } }, { status: 403 });
  const admin = sessionToken(request);
  if (!admin) return json({ error: { message: "not authenticated" } }, { status: 401 });
  if (impersonationToken(request)) {
    return json({ error: { message: "already impersonating: stop first" } }, { status: 400 });
  }
  let userId = 0;
  try {
    userId = Number((await request.json())?.user_id);
  } catch {}
  if (!Number.isSafeInteger(userId) || userId <= 0) {
    return json({ error: { message: "invalid user id" } }, { status: 400 });
  }
  try {
    const res = await fetch(`${API_URL}/api/v1/users/${userId}/impersonate`, {
      method: "POST",
      headers: { Authorization: `Bearer ${admin}`, "Content-Type": "application/json", ...clientIpHeaders(request) },
      cache: "no-store",
    });
    const data = await res.json().catch(() => null);
    if (!res.ok || !data?.data?.token) {
      return json(data ?? { error: { message: "impersonation failed" } }, { status: res.ok ? 502 : res.status });
    }
    const { token, user, impersonated_by, expires_in } = data.data;
    cookies.set(IMPERSONATION_COOKIE, token, {
      path: "/",
      httpOnly: true,
      sameSite: "strict",
      secure: url.protocol === "https:",
      maxAge: Number(expires_in) > 0 ? Number(expires_in) : 24 * 3600,
    });
    return json({ data: { user, impersonated_by } });
  } catch (e: any) {
    return json({ error: { message: e?.message || "proxy error" } }, { status: 502 });
  }
};

// DELETE: stop impersonating. The cookie is always cleared, even when the
// session already expired or the API cannot be reached.
export const DELETE: RequestHandler = async ({ request, cookies, url }) => {
  if (!sameOrigin(request, url)) return json({ error: { message: "cross-site request refused" } }, { status: 403 });
  const imp = impersonationToken(request);
  cookies.delete(IMPERSONATION_COOKIE, { path: "/", httpOnly: true, sameSite: "strict", secure: url.protocol === "https:" });
  if (imp) {
    try {
      await fetch(`${API_URL}/api/v1/auth/stop-impersonation`, {
        method: "POST",
        headers: { Authorization: `Bearer ${imp}`, "Content-Type": "application/json", ...clientIpHeaders(request) },
        cache: "no-store",
      });
    } catch {}
  }
  return json({ data: { ok: true } });
};
