import type { Handle } from "@sveltejs/kit";
import {
  MARKETING_PATHS,
  getPublicConfig,
  landingFallback,
} from "$lib/server/publicConfig";

const SECURITY_HEADERS: Record<string, string> = {
  "X-Content-Type-Options": "nosniff",
  "X-Frame-Options": "DENY",
  "Referrer-Policy": "strict-origin-when-cross-origin",
  "Permissions-Policy": "camera=(), microphone=(), geolocation=(), payment=()",
  "Cross-Origin-Opener-Policy": "same-origin",
};

function withSecurityHeaders(response: Response): Response {
  for (const [name, value] of Object.entries(SECURITY_HEADERS)) {
    if (!response.headers.has(name)) response.headers.set(name, value);
  }
  return response;
}

// Baseline security headers for every response served by the SvelteKit app.
// Headers a route already set (e.g. Referrer-Policy: no-referrer on the PMA
// redirect) are never overridden.
//
// Self-hosted panels (SIGARDAPANEL_LANDING_PAGE not true) have no marketing
// site: a full page load of / or /pricing is answered with a plain 302 before
// any HTML is sent, so the SaaS landing never flashes. Client-side navigations
// (data requests) are redirected by the pages' +page.server.ts load instead.
export const handle: Handle = async ({ event, resolve }) => {
  if (
    event.request.method === "GET" &&
    !event.isDataRequest &&
    MARKETING_PATHS.has(event.url.pathname)
  ) {
    const cfg = await getPublicConfig();
    if (!cfg.landing_page) {
      return withSecurityHeaders(
        new Response(null, {
          status: 302,
          headers: {
            Location: landingFallback(event.request.headers.get("cookie")),
            "Cache-Control": "no-store",
          },
        }),
      );
    }
  }
  return withSecurityHeaders(await resolve(event));
};
