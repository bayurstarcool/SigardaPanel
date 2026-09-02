import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

const API_URL = 'http://localhost:7700';

export const POST: RequestHandler = async ({ request }) => {
  try {
    const body = await request.text();
    const res = await fetch(`${API_URL}/api/v1/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body,
    });
    const data = await res.json();
    return json(data, { status: res.status });
  } catch (e: any) {
    return json({ error: { message: e.message || 'proxy error' } }, { status: 502 });
  }
};
