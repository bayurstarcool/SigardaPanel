import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';

const API_URL = env.SIGARDAPANEL_API_URL || 'http://localhost:7700';

export const GET: RequestHandler = async ({ request }) => {
  const headers: Record<string, string> = {};
  const auth = request.headers.get('Authorization');
  if (auth) headers['Authorization'] = auth;

  try {
    const res = await fetch(`${API_URL}/api/v1/metrics`, { headers, cache: 'no-store' });
    const data = await res.json();
    return json(data, { status: res.status });
  } catch (e: any) {
    return json({ error: { message: e.message || 'proxy error' } }, { status: 502 });
  }
};
