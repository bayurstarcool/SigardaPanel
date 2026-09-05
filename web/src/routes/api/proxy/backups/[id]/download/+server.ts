import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';

const API_URL = env.SIGARDAPANEL_API_URL || 'http://localhost:7700';

// Binary-safe passthrough for backup downloads.
// The generic /api/proxy/[...path] handler does res.json() on every response,
// which corrupts binary payloads (zip archives) into 502s.
export const GET: RequestHandler = async ({ params, request }) => {
  const auth = request.headers.get('Authorization');
  const headers: Record<string, string> = {};
  if (auth) headers['Authorization'] = auth;

  const upstream = await fetch(`${API_URL}/api/v1/backups/${params.id}/download`, {
    headers,
    cache: 'no-store'
  });

  if (!upstream.ok || !upstream.body) {
    // Error responses are JSON — surface status + body as-is.
    const text = await upstream.text().catch(() => 'download failed');
    return new Response(text, { status: upstream.status });
  }

  const resHeaders = new Headers();
  const ct = upstream.headers.get('Content-Type');
  if (ct) resHeaders.set('Content-Type', ct);
  const cd = upstream.headers.get('Content-Disposition');
  if (cd) resHeaders.set('Content-Disposition', cd);
  const cl = upstream.headers.get('Content-Length');
  if (cl) resHeaders.set('Content-Length', cl);

  return new Response(upstream.body, { status: 200, headers: resHeaders });
};
