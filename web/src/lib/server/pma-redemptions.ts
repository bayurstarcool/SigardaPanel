import { randomBytes } from "crypto";

type Redemption = { sessionTicket: string; expiresAt: number };

const redemptions = new Map<string, Redemption>();

function pruneExpired(now = Date.now()) {
  for (const [token, entry] of redemptions) {
    if (entry.expiresAt <= now) redemptions.delete(token);
  }
}

export function createPmaRedemption(
  sessionTicket: string,
  expiresAt: number,
): string {
  pruneExpired();
  const token = randomBytes(32).toString("base64url");
  redemptions.set(token, { sessionTicket, expiresAt });
  return token;
}

export function consumePmaRedemption(token: string): Redemption | null {
  pruneExpired();
  const entry = redemptions.get(token);
  if (!entry) return null;
  redemptions.delete(token);
  return entry.expiresAt > Date.now() ? entry : null;
}
