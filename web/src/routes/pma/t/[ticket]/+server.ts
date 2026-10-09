import type { RequestHandler } from "./$types";
import { env } from "$env/dynamic/private";
import { createDecipheriv, createHash } from "crypto";
import { consumePmaRedemption } from "$lib/server/pma-redemptions";

function encryptionKey(): Buffer | null {
  const secret = env.SIGARDAPANEL_PMA_SECRET;
  return secret && secret.length >= 32
    ? createHash("sha256").update(secret).digest()
    : null;
}

export const GET: RequestHandler = async ({ params, cookies, request }) => {
  const key = encryptionKey();
  if (!key) return new Response(null, { status: 503 });

  try {
    const redemption = consumePmaRedemption(params.ticket);
    if (!redemption) throw new Error("invalid or consumed redemption");
    const sessionTicket = redemption.sessionTicket;
    const buf = Buffer.from(sessionTicket, "base64url");
    if (buf.length <= 28) throw new Error("invalid ticket");

    const decipher = createDecipheriv("aes-256-gcm", key, buf.subarray(0, 12));
    decipher.setAuthTag(buf.subarray(12, 28));
    const payload = JSON.parse(
      Buffer.concat([
        decipher.update(buf.subarray(28)),
        decipher.final(),
      ]).toString("utf8"),
    );

    if (!payload.exp || !payload.i || Date.now() >= payload.exp) {
      return new Response(null, {
        status: 302,
        headers: { Location: "/login" },
      });
    }

    if (!payload.u || typeof payload.p !== "string") {
      return new Response(null, {
        status: 302,
        headers: { Location: "/login" },
      });
    }

    const proto = request.headers.get("x-forwarded-proto") || "https";
    // Lax is enough (same-site navigation + nginx auth_request); Secure is
    // only dropped for plain-HTTP localhost access.
    cookies.set("pma_ticket", sessionTicket, {
      path: "/",
      httpOnly: true,
      sameSite: "lax",
      secure: proto === "https",
      maxAge: 3600,
    });

    const host = request.headers.get("host") || "cp.sigarda.dev";
    const urlToken = `sg_${createHash("sha256").update(sessionTicket).digest("base64url")}`;
    return new Response(null, {
      status: 302,
      headers: {
        Location: `${proto}://${host}/pma/index.php?token=${urlToken}`,
        "Cache-Control": "no-store",
        "Referrer-Policy": "no-referrer",
      },
    });
  } catch {
    return new Response(null, { status: 302, headers: { Location: "/login" } });
  }
};
