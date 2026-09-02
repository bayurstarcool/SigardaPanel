import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';
import { randomBytes, createCipheriv, createHash } from 'crypto';
import { createPmaRedemption } from '$lib/server/pma-redemptions';

const API_URL = env.SIGARDAPANEL_API_URL || 'http://127.0.0.1:7700';

function encryptionKey(): Buffer | null {
  const secret = env.SIGARDAPANEL_PMA_SECRET;
  return secret && secret.length >= 32 ? createHash('sha256').update(secret).digest() : null;
}

export const POST: RequestHandler = async ({ request }) => {
  const key = encryptionKey();
  if (!key) return new Response(null, { status: 503 });

  const auth = request.headers.get('Authorization');
  const token = auth?.startsWith('Bearer ') ? auth.slice(7) : null;
  if (!token) return new Response(null, { status: 401 });

  let databaseId = 0;
  try {
    const body = await request.json();
    databaseId = Number(body.database_id);
  } catch {
    return new Response(null, { status: 400 });
  }
  if (!Number.isSafeInteger(databaseId) || databaseId <= 0) {
    return new Response(null, { status: 400 });
  }

  // Verify database exists
  const databaseRes = await fetch(`${API_URL}/api/v1/databases/${databaseId}`, {
    headers: { Authorization: `Bearer ${token}` }
  });
  if (databaseRes.status === 401) return new Response(null, { status: 401 });
  if (!databaseRes.ok) return new Response(null, { status: 404 });

  const databaseBody = await databaseRes.json();
  const database = databaseBody?.data ?? databaseBody;
  if (database?.type !== 'mysql' || !database?.name) {
    return new Response(null, { status: 409 });
  }

  // Get MySQL credentials based on user role
  // Admin/super_admin -> root, Regular user -> site user
  const credRes = await fetch(`${API_URL}/api/v1/databases/${databaseId}/pma-credentials`, {
    headers: { Authorization: `Bearer ${token}` }
  });
  if (!credRes.ok) return new Response(null, { status: 503 });

  const credBody = await credRes.json();
  const cred = credBody?.data ?? credBody;
  if (!cred?.username || typeof cred?.password !== 'string') {
    return new Response(null, { status: 503 });
  }

  // Encrypt credentials into session ticket
  const expires = Date.now() + 3600_000;
  const payload = JSON.stringify({
    u: cred.username,
    p: cred.password,
    d: database.name,
    i: databaseId,
    exp: expires
  });
  const iv = randomBytes(12);
  const cipher = createCipheriv('aes-256-gcm', key, iv);
  const encrypted = Buffer.concat([cipher.update(payload, 'utf8'), cipher.final()]);
  const tag = cipher.getAuthTag();
  const sessionTicket = Buffer.concat([iv, tag, encrypted]).toString('base64url');

  const ticket = createPmaRedemption(sessionTicket, Date.now() + 60_000);

  return new Response(JSON.stringify({ ticket }), {
    status: 200,
    headers: {
      'Content-Type': 'application/json',
      'Cache-Control': 'no-store'
    }
  });
};
