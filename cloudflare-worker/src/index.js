/**
 * TempMail Cloudflare Email Worker
 *
 * Receives emails from Cloudflare Email Routing and forwards them
 * to your VPS via HTTP POST (no port 25 needed).
 *
 * Deployed by the TempMail control panel or by the guided setup script, both of which
 * upload this file through the Cloudflare API and set the secrets below.
 *
 * Secrets: TEMPMAIL_SERVER_URL, TEMPMAIL_API_KEY, FORWARD_LIST, TEMPMAIL_SIGNING_SECRET
 */

/** HMAC-SHA256 in hex, matching the X-Email-Signature the server expects. */
async function signBody(secret, body) {
  const key = await crypto.subtle.importKey(
    "raw",
    new TextEncoder().encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const signature = await crypto.subtle.sign("HMAC", key, body);
  return [...new Uint8Array(signature)]
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("");
}

export default {
  async email(message, env, ctx) {
    // Read the raw email (RFC822 format)
    const rawEmail = await new Response(message.raw).text();

    // Build the payload for our Go server
    const payload = {
      from: message.from,
      to: message.to,
      raw: rawEmail,
      subject: message.headers.get("subject") || "",
    };

    const serverUrl = env.TEMPMAIL_SERVER_URL || "https://your-vps.com";
    const apiKey = env.TEMPMAIL_API_KEY || "";
    const signingSecret = env.TEMPMAIL_SIGNING_SECRET || "";

    // Optional copies. Empty by default: email goes only to TempMail.
    // Multiple destinations may be comma-separated in FORWARD_LIST.
    const destinations = (env.FORWARD_LIST || "")
      .split(",")
      .map((address) => address.trim())
      .filter(Boolean);
    for (const destination of destinations) {
      try {
        await message.forward(destination);
      } catch (error) {
        console.error(`Copy to ${destination} failed:`, error);
      }
    }

    const body = JSON.stringify(payload);
    const headers = {
      "Content-Type": "application/json",
      "X-Email-API-Key": apiKey,
    };
    if (signingSecret) {
      headers["X-Email-Signature"] = "sha256=" + (await signBody(signingSecret, new TextEncoder().encode(body)));
    }

    try {
      const response = await fetch(`${serverUrl}/api/email/receive`, {
        method: "POST",
        headers,
        body,
      });

      if (!response.ok) {
        console.error(`Server error: ${response.status} ${response.statusText}`);
      } else {
        console.log(`Email forwarded: ${message.from} -> ${message.to}`);
      }
    } catch (error) {
      console.error("Failed to send TempMail copy:", error);
    }
  },

  // Health check endpoint (opsional, untuk monitoring)
  async fetch(request, env) {
    return new Response(JSON.stringify({
      status: "ok",
      worker: "tempmail-email-forwarder",
      server: env.TEMPMAIL_SERVER_URL || "not-configured",
      signed: Boolean(env.TEMPMAIL_SIGNING_SECRET),
    }), {
      headers: { "Content-Type": "application/json" },
    });
  },
};
