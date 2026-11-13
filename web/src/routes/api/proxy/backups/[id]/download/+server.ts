import type { RequestHandler } from "./$types";
import { API_URL, authHeaders, clientIpHeaders } from "$lib/server/upstream";

// Binary-safe passthrough for backup downloads.
// The generic /api/proxy/[...path] handler does res.json() on every response,
// which corrupts binary payloads (zip archives) into 502s.
export const GET: RequestHandler = async ({ params, request }) => {
  // Normal sessions are cookie-only (the page links here with a plain
  // <a href>): derive the API token from the sp_session cookie.
  if (!/^\d+$/.test(params.id)) return new Response("invalid backup id", { status: 400 });
  const headers: Record<string, string> = { ...authHeaders(request), ...clientIpHeaders(request) };

  const upstream = await fetch(
    `${API_URL}/api/v1/backups/${params.id}/download`,
    {
      headers,
      cache: "no-store",
    },
  );

  if (!upstream.ok || !upstream.body) {
    // Error responses are JSON — surface status + body as-is.
    const text = await upstream.text().catch(() => "download failed");
    return new Response(text, { status: upstream.status });
  }

  const resHeaders = new Headers();
  const ct = upstream.headers.get("Content-Type");
  if (ct) resHeaders.set("Content-Type", ct);
  const cd = upstream.headers.get("Content-Disposition");
  if (cd) resHeaders.set("Content-Disposition", cd);
  const cl = upstream.headers.get("Content-Length");
  if (cl) resHeaders.set("Content-Length", cl);

  return new Response(upstream.body, { status: 200, headers: resHeaders });
};
