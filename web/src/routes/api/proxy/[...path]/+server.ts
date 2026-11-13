import { json } from "@sveltejs/kit";
import type { RequestHandler } from "./$types";
import { API_URL, authHeaders, clientIpHeaders } from "$lib/server/upstream";

// Generic pass-through to the Go API. Bodies and responses are forwarded
// as bytes with their own content type, so JSON calls, multipart uploads
// (file manager) and binary downloads all work; Set-Cookie is passed on
// (logout clears the session cookie).
const forwardedResponseHeaders = [
  "content-type",
  "content-disposition",
  "content-length",
  "cache-control",
  "x-file-name",
  "x-request-id",
];

async function forward(method: string, params: { path: string }, request: Request): Promise<Response> {
  const url = `${API_URL}/api/v1/${params.path}${new URL(request.url).search}`;
  const headers: Record<string, string> = { ...authHeaders(request), ...clientIpHeaders(request) };
  let body: ArrayBuffer | undefined;
  if (method !== "GET" && method !== "DELETE") {
    headers["Content-Type"] = request.headers.get("content-type") || "application/json";
    body = await request.arrayBuffer();
  }
  try {
    const res = await fetch(url, { method, headers, body, cache: "no-store", redirect: "manual" });
    const out = new Headers();
    for (const h of forwardedResponseHeaders) {
      const v = res.headers.get(h);
      if (v) out.set(h, v);
    }
    for (const c of res.headers.getSetCookie?.() ?? []) out.append("set-cookie", c);
    return new Response(res.body, { status: res.status, headers: out });
  } catch (e: any) {
    return json({ error: { message: e?.message || "proxy error" } }, { status: 502 });
  }
}

export const GET: RequestHandler = ({ params, request }) => forward("GET", params, request);
export const POST: RequestHandler = ({ params, request }) => forward("POST", params, request);
export const PUT: RequestHandler = ({ params, request }) => forward("PUT", params, request);
export const PATCH: RequestHandler = ({ params, request }) => forward("PATCH", params, request);
export const DELETE: RequestHandler = ({ params, request }) => forward("DELETE", params, request);
