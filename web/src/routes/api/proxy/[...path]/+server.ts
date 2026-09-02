import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';

const API_URL = env.SIGARDAPANEL_API_URL || 'http://localhost:7700';

function buildUrl(base: string, request: Request): string {
  const url = new URL(request.url);
  const qs = url.search;
  return `${base}${qs}`;
}

export const GET: RequestHandler = async ({ params, request }) => {
  const url = buildUrl(`${API_URL}/api/v1/${params.path}`, request);
  const headers: Record<string, string> = {};
  const auth = request.headers.get('Authorization');
  if (auth) headers['Authorization'] = auth;

  try {
    const res = await fetch(url, { headers, cache: 'no-store' });
    const data = await res.json();
    return json(data, { status: res.status });
  } catch (e: any) {
    return json({ error: { message: e.message || 'proxy error' } }, { status: 502 });
  }
};

export const POST: RequestHandler = async ({ params, request }) => {
  const url = buildUrl(`${API_URL}/api/v1/${params.path}`, request);
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const auth = request.headers.get('Authorization');
  if (auth) headers['Authorization'] = auth;

  try {
    const body = await request.text();
    const res = await fetch(url, { method: 'POST', headers, body, cache: 'no-store' });
    const data = await res.json();
    return json(data, { status: res.status });
  } catch (e: any) {
    return json({ error: { message: e.message || 'proxy error' } }, { status: 502 });
  }
};

export const PUT: RequestHandler = async ({ params, request }) => {
  const url = buildUrl(`${API_URL}/api/v1/${params.path}`, request);
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const auth = request.headers.get('Authorization');
  if (auth) headers['Authorization'] = auth;

  try {
    const body = await request.text();
    const res = await fetch(url, { method: 'PUT', headers, body, cache: 'no-store' });
    const data = await res.json();
    return json(data, { status: res.status });
  } catch (e: any) {
    return json({ error: { message: e.message || 'proxy error' } }, { status: 502 });
  }
};

export const PATCH: RequestHandler = async ({ params, request }) => {
  const url = buildUrl(`${API_URL}/api/v1/${params.path}`, request);
  const headers: Record<string, string> = { 'Content-Type': 'application/json' };
  const auth = request.headers.get('Authorization');
  if (auth) headers['Authorization'] = auth;

  try {
    const body = await request.text();
    const res = await fetch(url, { method: 'PATCH', headers, body, cache: 'no-store' });
    const data = await res.json();
    return json(data, { status: res.status });
  } catch (e: any) {
    return json({ error: { message: e.message || 'proxy error' } }, { status: 502 });
  }
};

export const DELETE: RequestHandler = async ({ params, request }) => {
  const url = buildUrl(`${API_URL}/api/v1/${params.path}`, request);
  const headers: Record<string, string> = {};
  const auth = request.headers.get('Authorization');
  if (auth) headers['Authorization'] = auth;

  try {
    const res = await fetch(url, { method: 'DELETE', headers, cache: 'no-store' });
    const data = await res.json();
    return json(data, { status: res.status });
  } catch (e: any) {
    return json({ error: { message: e.message || 'proxy error' } }, { status: 502 });
  }
};
