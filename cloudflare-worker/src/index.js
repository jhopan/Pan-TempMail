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
        // Optionally forward to a backup email
        if (env.FORWARD_LIST) {
          await message.forward(env.FORWARD_LIST);
        }
      } else {
        console.log(`Email forwarded: ${message.from} → ${message.to}`);
      }
    } catch (error) {
      console.error("Failed to forward email:", error);
      // Forward to backup email on failure
      if (env.FORWARD_LIST) {
        await message.forward(env.FORWARD_LIST);
      }
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
