import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';
import { createDecipheriv, createHash } from 'crypto';

function encryptionKey(): Buffer | null {
  const secret = env.SIGARDAPANEL_PMA_SECRET;
  return secret && secret.length >= 32 ? createHash('sha256').update(secret).digest() : null;
}

export const GET: RequestHandler = async ({ cookies }) => {
  const key = encryptionKey();
  const ticket = cookies.get('pma_ticket');
  if (!key || !ticket) return new Response(null, { status: 401 });

  try {
    const buf = Buffer.from(ticket, 'base64url');
    if (buf.length <= 28) throw new Error('invalid ticket');

    const decipher = createDecipheriv('aes-256-gcm', key, buf.subarray(0, 12));
    decipher.setAuthTag(buf.subarray(12, 28));
    const payload = JSON.parse(
      Buffer.concat([decipher.update(buf.subarray(28)), decipher.final()]).toString('utf8')
    );
    if (!payload.exp || Date.now() >= payload.exp) {
      return new Response(null, { status: 401 });
    }

    if (!payload.u || typeof payload.p !== 'string') {
      return new Response(null, { status: 401 });
    }

    const authorization = Buffer.from(`${payload.u}:${payload.p}`, 'utf8').toString('base64');

    return new Response(null, {
      status: 200,
      headers: {
        'X-Pma-Authorization': `Basic ${authorization}`,
        'Cache-Control': 'no-store'
      }
    });
  } catch {
    return new Response(null, { status: 401 });
  }
};
