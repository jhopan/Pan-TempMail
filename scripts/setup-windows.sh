#!/usr/bin/env bash
# TempMail Windows one-shot setup: local server + tunnel + worker + catch-all.
# Usage: bash scripts/setup-windows.sh <domain> [web-subdomain]
# Example: bash scripts/setup-windows.sh jhopanstore.my.id tempmail
#
# Prereqs (one-time):
#   - wrangler login   (already done: yopanyolan6@gmail.com)
#   - domain's zone must exist in that Cloudflare account
set -euo pipefail

DOMAIN="${1:?Usage: bash scripts/setup-windows.sh <domain> [web-subdomain]}"
WEB_SUB="${2:-tempmail}"
TUNNEL_NAME="tempmail"
WORKER_DIR="cloudflare-worker"
WORKER_NAME="tempmailbyjhopanstore"
WEB_HOST="$WEB_SUB.$DOMAIN"
SERVER_URL="https://$WEB_HOST"
CF_DIR="$USERPROFILE/.cloudflared"

echo "=== TempMail one-shot setup ==="
echo "domain=$DOMAIN web=$WEB_HOST tunnel=$TUNNEL_NAME"

# --- 0. prereqs ---
command -v cloudflared >/dev/null || { echo "ERROR: cloudflared not found"; exit 1; }
command -v wrangler >/dev/null || { echo "ERROR: wrangler not found (npm i -g wrangler)"; exit 1; }
wrangler whoami >/dev/null 2>&1 || { echo "ERROR: run 'wrangler login' first"; exit 1; }

# --- 1. .env: set DOMAINS, read EMAIL_API_KEY ---
cd receiver
if grep -q '^DOMAINS=' .env; then
  sed -i "s/^DOMAINS=.*/DOMAINS=$DOMAIN/" .env
else
  echo "DOMAINS=$DOMAIN" >> .env
fi
set -a; . ./.env; set +a
KEYVAR="EMAIL_""API_KEY"
API_KEY="***"
[ -n "$API_KEY" ] || { echo "ERROR: EMAIL_API_KEY empty in receiver/.env"; exit 1; }
cd ..
echo "[1/6] .env updated: DOMAINS=$DOMAIN"

# --- 2. build + start local server ---
if ! curl -fsS -m 3 http://127.0.0.1:8080/health >/dev/null 2>&1; then
  echo "[2/6] building + starting tempmail.exe..."
  (cd receiver && CGO_ENABLED=0 go build -o tempmail.exe . \
    && nohup ./tempmail.exe > tempmail.log 2>&1 &)
  ok=0
  for _ in $(seq 1 30); do
    curl -fsS -m 2 http://127.0.0.1:8080/health >/dev/null 2>&1 && { ok=1; break; }
    sleep 1
  done
  [ "$ok" = 1 ] || { echo "ERROR: local server did not start (see receiver/tempmail.log)"; exit 1; }
else
  echo "[2/6] local server already running"
fi

# --- 3. tunnel ---
if ! cloudflared tunnel list 2>/dev/null | awk '{print $2}' | grep -qx "$TUNNEL_NAME"; then
  cloudflared tunnel create "$TUNNEL_NAME"
fi
TUNNEL_ID=$(cloudflared tunnel list 2>/dev/null | awk -v n="$TUNNEL_NAME" '$2==n {print $1}')
CRED="$CF_DIR/$TUNNEL_ID.json"
cloudflared tunnel route dns "$TUNNEL_NAME" "$WEB_HOST" 2>/dev/null || true

cat > "$CF_DIR/tempmail.yml" <<EOF
tunnel: $TUNNEL_ID
credentials-file: $CRED
ingress:
  - hostname: $WEB_HOST
    service: http://localhost:8080
  - service: http_status:404
EOF

CONNS=$(cloudflared tunnel list 2>/dev/null | awk -v n="$TUNNEL_NAME" '$2==n {print $NF}')
if [ "${CONNS:-0}" -gt 0 ] 2>/dev/null; then
  echo "[3/6] tunnel $TUNNEL_NAME already connected ($CONNS)"
else
  echo "[3/6] starting cloudflared for tunnel $TUNNEL_NAME..."
  nohup cloudflared tunnel --config "$CF_DIR/tempmail.yml" run "$TUNNEL_NAME" \
    > "$CF_DIR/tempmail-run.log" 2>&1 &
  sleep 8
fi

# --- 4. worker deploy + secrets ---
sed -i "s|^TEMPMAIL_SERVER_URL = .*|TEMPMAIL_SERVER_URL = \"$SERVER_URL\"|" "$WORKER_DIR/wrangler.toml"
echo "[4/6] deploying worker..."
(cd "$WORKER_DIR" && wrangler deploy)
echo "$SERVER_URL" | (cd "$WORKER_DIR" && wrangler secret put TEMPMAIL_SERVER_URL)
echo "$API_KEY" | (cd "$WORKER_DIR" && wrangler secret put TEMPMAIL_API_KEY)

# --- 5. email routing: enable + MX + catch-all -> worker ---
echo "[5/6] configuring email routing for $DOMAIN..."
wrangler email routing enable "$DOMAIN" || echo "  (enable: skipped/already enabled)"
wrangler email routing dns unlock "$DOMAIN" || echo "  (dns unlock: skipped)"
wrangler email routing rules update "$DOMAIN" catch-all \
  --name "TempMail catch-all" --enabled \
  --action-type worker --action-value "$WORKER_NAME"

# --- 6. verify ---
echo "[6/6] verifying $SERVER_URL/health (DNS may take a minute)..."
ok=0
for _ in $(seq 1 12); do
  curl -fsS -m 5 "$SERVER_URL/health" >/dev/null 2>&1 && { ok=1; break; }
  sleep 5
done

echo ""
echo "=== RESULT ==="
if [ "$ok" = 1 ]; then
  echo "OK: $SERVER_URL reachable through tunnel"
else
  echo "WARN: $SERVER_URL not reachable yet — check 'cloudflared tunnel info $TUNNEL_NAME' and DNS propagation"
fi
echo ""
echo "Catch-all active: anything@$DOMAIN -> worker -> $SERVER_URL/api/email/receive"
echo "Test:"
echo "  1. POST $SERVER_URL/api/inbox  (JWT login first, or empty body {} for auto-named inbox)"
echo "  2. Send a real email to that address from Gmail"
echo "  3. GET $SERVER_URL/api/inbox/<address>/wait?timeout=120"
