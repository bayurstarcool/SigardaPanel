import { env } from "$env/dynamic/private";

/** Base URL of the Go API (override with SIGARDAPANEL_API_URL). */
export const API_URL = env.SIGARDAPANEL_API_URL || "http://127.0.0.1:7700";

/**
 * Headers that carry the real client IP to the API. nginx must always set
 * X-Real-IP ($remote_addr) on the locations that reach this app, otherwise a
 * client-supplied value would be forwarded. Without it the API sees every
 * browser as 127.0.0.1 and its per-IP login limiter becomes global.
 */
export function clientIpHeaders(request: Request): Record<string, string> {
  const ip = request.headers.get("x-real-ip");
  return ip ? { "X-Real-IP": ip } : {};
}
