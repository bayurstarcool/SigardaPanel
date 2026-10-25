import { json } from "@sveltejs/kit";
import type { RequestHandler } from "./$types";
import { API_URL, clientIpHeaders } from "$lib/server/upstream";

export const POST: RequestHandler = async ({ request }) => {
  try {
    const body = await request.text();
    const res = await fetch(`${API_URL}/api/v1/auth/login`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...clientIpHeaders(request),
      },
      body,
    });
    const data = await res.json();
    const out = json(data, { status: res.status });
    // Forward the HttpOnly session cookie so the browser holds the token,
    // not page JavaScript.
    const setCookie = res.headers.get("set-cookie");
    if (setCookie) out.headers.set("set-cookie", setCookie);
    return out;
  } catch (e: any) {
    return json(
      { error: { message: e.message || "proxy error" } },
      { status: 502 },
    );
  }
};
