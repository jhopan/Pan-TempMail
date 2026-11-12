/**
 * TempMail Cloudflare Email Worker
 *
 * Receives emails from Cloudflare Email Routing and forwards them
 * to your VPS via HTTP POST (no port 25 needed).
 *
 * Deploy: npx wrangler deploy
 * Setup: Cloudflare Dashboard → Email → Routing Rules → Catch-all → Send to Worker
 */

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

    // POST to our VPS endpoint
    const serverUrl = env.TEMPMAIL_SERVER_URL || "https://your-vps.com";
    const apiKey = env.TEMPMAIL_API_KEY || "";

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

    try {
      const response = await fetch(`${serverUrl}/api/email/receive`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Email-API-Key": apiKey,
        },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        console.error(`Server error: ${response.status} ${response.statusText}`);
      } else {
        console.log(`Email forwarded: ${message.from} → ${message.to}`);
      }
    } catch (error) {
      console.error("Failed to send TempMail copy:", error);
    }
  },

  // Health check endpoint (optional — for monitoring)
  async fetch(request, env) {
    return new Response(JSON.stringify({
      status: "ok",
      worker: "tempmail-email-forwarder",
      server: env.TEMPMAIL_SERVER_URL || "not-configured",
    }), {
      headers: { "Content-Type": "application/json" },
    });
  },
};
