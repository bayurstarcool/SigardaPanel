import { auth } from '$lib/auth.svelte';
import { toasts } from '$lib/toast.svelte';

/** Opens phpMyAdmin for a database in a new tab via a one-time ticket. */
export async function openPma(databaseId: number) {
  if (!auth.user) { window.location.href = '/login'; return; }
  try {
    // The HttpOnly session cookie authenticates the ticket request;
    // impersonation falls back to the explicit Bearer token.
    const headers: Record<string, string> = { 'Content-Type': 'application/json' };
    if (auth.token) headers['Authorization'] = `Bearer ${auth.token}`;
    const res = await fetch('/pma/ticket', {
      method: 'POST',
      headers,
      body: JSON.stringify({ database_id: databaseId }),
    });
    if (res.status === 401) { window.location.href = '/login'; return; }
    if (!res.ok) {
      toasts.error('phpMyAdmin gagal dibuka. Database harus punya user MySQL aktif.');
      return;
    }
    const { ticket } = await res.json();
    window.open(`/pma/t/${encodeURIComponent(ticket)}`, '_blank', 'noopener,noreferrer');
  } catch {
    toasts.error('phpMyAdmin gagal dibuka. Coba lagi.');
  }
}

export function fmtSize(bytes: number) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let i = 0;
  let size = bytes;
  while (size >= 1024 && i < units.length - 1) { size /= 1024; i++; }
  return `${size.toFixed(1)} ${units[i]}`;
}

export function canonicalHost(host: string) {
  const value = (host || '%').trim().toLowerCase();
  if (value === '127.0.0.1' || value === '::1') return 'localhost';
  return value || '%';
}

export function dbLabel(db: any) { return `${db.name}${db.domain ? ` (${db.domain})` : ''}`; }
export function siteLabel(site: any) { return site.domain || site.name || `Site #${site.id}`; }
