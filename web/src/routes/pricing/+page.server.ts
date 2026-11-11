import type { PageServerLoad } from "./$types";
import { guardMarketingPage } from "$lib/server/publicConfig";

// Pricing is part of the SaaS marketing site (SIGARDAPANEL_LANDING_PAGE).
export const load: PageServerLoad = async ({ request }) => {
  await guardMarketingPage(request);
  return {};
};
