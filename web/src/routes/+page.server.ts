import type { PageServerLoad } from "./$types";
import { guardMarketingPage } from "$lib/server/publicConfig";

// SaaS landing only when SIGARDAPANEL_LANDING_PAGE=true; self-hosted panels
// go to /login (or /dashboard). Full page loads are already redirected in
// hooks.server.ts; this covers client-side navigations.
export const load: PageServerLoad = async ({ request }) => {
  await guardMarketingPage(request);
  return {};
};
