import { redirect } from "@sveltejs/kit";
import { API_URL } from "$lib/server/upstream";

/** Deployment switches from GET /api/v1/public/config (no auth). */
export interface PublicConfig {
  /** / and /pricing show the SaaS landing (SIGARDAPANEL_LANDING_PAGE). */
  landing_page: boolean;
  /** /register can create member accounts (SIGARDAPANEL_ALLOW_MEMBER_SIGNUP). */
  member_signup: boolean;
  /** Panel self-registration is open (SIGARDAPANEL_ALLOW_REGISTRATION). */
  panel_signup: boolean;
}

// Self-hosted defaults: also used when the API cannot be reached, so an API
// outage never exposes the SaaS marketing pages on a self-hosted panel.
const CLOSED: PublicConfig = {
  landing_page: false,
  member_signup: false,
  panel_signup: false,
};

const TTL_MS = 15_000;
let cached: { at: number; value: PublicConfig } | null = null;

export async function getPublicConfig(): Promise<PublicConfig> {
  if (cached && Date.now() - cached.at < TTL_MS) return cached.value;
  try {
    const res = await fetch(`${API_URL}/api/v1/public/config`, {
      signal: AbortSignal.timeout(3000),
    });
    if (!res.ok) return CLOSED;
    const d = (await res.json())?.data ?? {};
    const value: PublicConfig = {
      landing_page: d.landing_page === true,
      member_signup: d.member_signup === true,
      panel_signup: d.panel_signup === true,
    };
    cached = { at: Date.now(), value };
    return value;
  } catch {
    return CLOSED;
  }
}

/** Marketing pages that only exist on the SaaS (landing) deployment. */
export const MARKETING_PATHS = new Set(["/", "/pricing"]);

/**
 * Where a visitor of a marketing page goes when the landing is off: the
 * dashboard if they hold a session cookie (the dashboard itself sends an
 * expired session to /login), otherwise the login page.
 */
export function landingFallback(cookieHeader: string | null): string {
  return /(?:^|;\s*)sp_session=[^;]+/.test(cookieHeader ?? "")
    ? "/dashboard"
    : "/login";
}

/** Throws a redirect when the landing page is disabled. */
export async function guardMarketingPage(request: Request): Promise<void> {
  const cfg = await getPublicConfig();
  if (!cfg.landing_page) {
    redirect(302, landingFallback(request.headers.get("cookie")));
  }
}
