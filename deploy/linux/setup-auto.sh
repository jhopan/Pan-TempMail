#!/usr/bin/env bash
# TempMail one-shot Cloudflare setup (Linux target). Run as root on the device.
# Auth: wrangler login (OAuth) + cloudflared tunnel login. No API token needed.
# Usage: bash setup-auto.sh <domain> [web-host]
# Example: bash setup-auto.sh jhopanstore.my.id tempmail.jhopanstore.my.id
set -euo pipefail

[ "$(id -u)" = 0 ] || { echo "Run as root"; exit 1; }
[ $# -ge 1 ] || { echo "Usage: $0 <domain> [web-host]"; exit 1; }

DOMAIN=$1; WEB_HOST=${2:-tempmail.$1}
TUNNEL_NAME=tempmail-jhopanstore
STATE=/etc/tempmail/cloudflare-state.json
APP_ENV=/etc/tempmail/tempmail.env
WORKER_DIR=/opt/tempmail/worker

echo "== 1/7 Wrangler login (buka URL di browser, klik Allow) =="
npx wrangler whoami >/dev/null 2>&1 || npx wrangler login
ACCOUNT_ID=$(npx wrangler whoami 2>/dev/null | grep -oE '[a-f0-9]{32}' | head -1)
[ -n "$ACCOUNT_ID" ] || { echo "Gagal ambil Account ID dari wrangler whoami"; exit 1; }
echo "Account ID: $ACCOUNT_ID"

echo "== 2/7 Tunnel login (buka URL di browser, klik Authorize) =="
[ -f /root/.cloudflared/cert.pem ] || cloudflared tunnel login

echo "== 3/7 Create tunnel + ingress + DNS route =="
TUNNEL_ID=$(cloudflared tunnel list --output json 2>/dev/null | python3 -c 'import json,sys
for t in json.load(sys.stdin):
    if t["name"]=="'"$TUNNEL_NAME"'": print(t["id"]); break' || true)
if [ -z "${TUNNEL_ID:-}" ]; then
  cloudflared tunnel create "$TUNNEL_NAME"
  TUNNEL_ID=$(cloudflared tunnel list --output json | python3 -c 'import json,sys
for t in json.load(sys.stdin):
    if t["name"]=="'"$TUNNEL_NAME"'": print(t["id"]); break')
fi
APP_PORT=$(grep '^PORT=' "$APP_ENV" | cut -d= -f2); APP_PORT=${APP_PORT:-8080}
cat >/etc/cloudflared/config.yml <<EOF
tunnel: $TUNNEL_ID
credentials-file: /root/.cloudflared/$TUNNEL_ID.json
ingress:
  - hostname: $WEB_HOST
    service: http://127.0.0.1:$APP_PORT
  - service: http_status:404
EOF
cloudflared tunnel route dns "$TUNNEL_NAME" "$WEB_HOST"
cloudflared service install 2>/dev/null || true
systemctl enable --now cloudflared

echo "== 4/7 Deploy worker + secrets =="
set -a; . "$APP_ENV"; set +a
KEYVAR="EMAIL_""API_KEY"
API_KEY="***"
(cd "$WORKER_DIR" \
  && export CLOUDFLARE_ACCOUNT_ID="$ACCOUNT_ID" \
  && printf '%s\n' "https://$WEB_HOST" | npx wrangler secret put TEMPMAIL_SERVER_URL \
  && printf '%s\n' "$API_KEY" | npx wrangler secret put TEMPMAIL_API_KEY \
  && printf '%s\n' "" | npx wrangler secret put FORWARD_LIST \
  && npx wrangler deploy)

echo "== 5/7 Enable Email Routing + DNS unlock =="
npx wrangler email routing enable "$DOMAIN" || true
npx wrangler email routing dns unlock "$DOMAIN" || true

echo "== 6/7 Set catch-all rule -> worker =="
npx wrangler email routing rules update "$DOMAIN" catch-all \
  --name "TempMail catch-all" --enabled \
  --action-type worker --action-value tempmailbyjhopanstore

echo "== 7/7 Activate domain + verify =="
install -d -o root -g root -m 0700 /etc/tempmail
[ -f "$STATE" ] || printf '{"active_profile":"","profiles":{},"domains":{},"tunnel":{"id":"","account":"","web_hosts":[]}}\n' >"$STATE"
D="$DOMAIN" T="$TUNNEL_ID" W="$WEB_HOST" A="$ACCOUNT_ID" python3 - "$STATE" <<'PY'
import json, os, sys
d = json.load(open(sys.argv[1]))
d['active_profile'] = 'wrangler-oauth'
d['profiles']['wrangler-oauth'] = {'label': 'wrangler OAuth', 'account_id': os.environ['A'],
                                   'token': '', 'worker_deployed': True}
d['domains'][os.environ['D']] = {'profile': 'wrangler-oauth', 'zone_id': '',
                                 'enabled': True, 'routing': True}
d['tunnel'].update({'id': os.environ['T'], 'web_host': os.environ['W'], 'account': 'cf tunnel'})
open(sys.argv[1], 'w').write(json.dumps(d, indent=2) + '\n')
PY
chmod 600 "$STATE"
tmp=$(mktemp)
grep -v '^DOMAINS=' "$APP_ENV" >"$tmp" || true
printf 'DOMAINS=%s\n' "$DOMAIN" >>"$tmp"
install -o root -g tempmail -m 0640 "$tmp" "$APP_ENV"; rm -f "$tmp"
systemctl restart tempmail
sleep 2
curl -fsS "http://127.0.0.1:${APP_PORT}/health" >/dev/null && echo "Local OK"
curl -fsS "https://$WEB_HOST/health" && echo && echo "Tunnel OK"

echo
echo "SELESAI."
echo "Web   : https://$WEB_HOST/app/"
echo "Email : apa-aja@$DOMAIN -> Worker -> Tunnel -> TempMail"
echo "Test  : kirim email dari Gmail ke alamat acak @$DOMAIN, cek https://$WEB_HOST/app/"
