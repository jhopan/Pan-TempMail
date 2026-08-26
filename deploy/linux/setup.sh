#!/usr/bin/env bash
# TempMail guided setup (Linux target). Run as root after cloning the repo.
# Flow: install deps -> wrangler login -> cf tunnel login -> tunnel+worker+routing.
# Usage: sudo bash setup.sh [domain] [web-host]   (domain ditanya kalau tidak diberi)
set -euo pipefail

[ "$(id -u)" = 0 ] || { echo "Jalankan sebagai root: sudo bash setup.sh"; exit 1; }

echo "=============================================="
echo "  TempMail By JhopanStore — Guided Setup"
echo "        Credit JhopanStore"
echo "=============================================="
if [ $# -ge 1 ]; then
  DOMAIN=$1
else
  read -r -p "Domain untuk TempMail (contoh: domainkamu.com): " DOMAIN
fi
[ -n "$DOMAIN" ] || { echo "Domain wajib diisi."; exit 1; }
WEB_HOST=${2:-tempmail.$DOMAIN}
echo "Domain : $DOMAIN"
echo "Web    : https://$WEB_HOST"
read -r -p "Nama tunnel Cloudflare [tempmailbyjhopanstore]: " TUNNEL_INPUT
TUNNEL_NAME=${TUNNEL_INPUT:-tempmailbyjhopanstore}
echo "Tunnel : $TUNNEL_NAME"

STATE=/etc/tempmail/cloudflare-state.json
APP_ENV=/etc/tempmail/tempmail.env
WORKER_DIR=/opt/tempmail/worker

pause() { read -r -p "$1" _; }

wrangler_logged_in() {
  (cd "$WORKER_DIR" && npx wrangler whoami 2>&1) | grep -q "You are logged in"
}

echo
echo "== [1/7] Install dependencies =="
apt-get update -qq
apt-get install -y -qq curl ca-certificates sqlite3 python3
if ! command -v node >/dev/null 2>&1 || ! command -v npm >/dev/null 2>&1; then
  echo "Install Node.js + npm..."
  apt-get install -y -qq nodejs npm
fi
if ! command -v cloudflared >/dev/null 2>&1; then
  echo "Install cloudflared..."
  arch=$(dpkg --print-architecture)
  case "$arch" in
    arm64) pkg=cloudflared-linux-arm64.deb ;;
    amd64) pkg=cloudflared-linux-amd64.deb ;;
    *) echo "Arsitektur tidak didukung: $arch"; exit 1 ;;
  esac
  curl -fsSL "https://github.com/cloudflare/cloudflared/releases/latest/download/$pkg" -o /tmp/cf.deb
  dpkg -i /tmp/cf.deb; rm -f /tmp/cf.deb
fi
echo "Dependencies OK: node $(node -v), cloudflared $(cloudflared --version 2>&1 | head -1)"

echo
echo "== [2/7] Login Wrangler (Cloudflare OAuth) =="
if wrangler_logged_in; then
  echo "Sudah login. Skip."
else
  attempt=0
  until wrangler_logged_in; do
    attempt=$((attempt+1))
    [ $attempt -le 3 ] || { echo "Login wrangler 3x gagal."; exit 1; }
    logf=$(mktemp)
    (cd "$WORKER_DIR" && npx wrangler login --browser=false --device) >"$logf" 2>&1 &
    pid=$!
    url=""; code=""
    for _ in $(seq 1 15); do
      url=$(grep -oE 'https://dash\.cloudflare\.com/oauth2/device/verify[^ "]*' "$logf" | head -1 || true)
      [ -n "$url" ] && break; sleep 1
    done
    echo
    echo "===== LOGIN Wrangler ====="
    echo "Buka URL ini di browser (HP/laptop manapun):"
    echo
    echo "  ${url:-https://dash.cloudflare.com/oauth2/device/verify}"
    echo
    code=$(grep -A2 'enter the code' "$logf" | grep -oE '[A-Za-z0-9]{8}' | head -1 || true)
    [ -n "$code" ] && echo "Masukkan kode: $code" && echo
    echo "Klik Continue / Authorize, lalu tekan ENTER di sini."
    read -r -p "" _
    wait "$pid" 2>/dev/null || true
    rm -f "$logf"
  done
  echo "Wrangler login OK."
fi
ACCOUNT_ID=$(cd "$WORKER_DIR" && npx wrangler whoami 2>/dev/null | grep -oE '[a-f0-9]{32}' | head -1)
[ -n "$ACCOUNT_ID" ] || { echo "Gagal ambil Account ID. Ulangi login."; exit 1; }
echo "Account ID: $ACCOUNT_ID"

echo
echo "== [3/7] Login Cloudflare Tunnel =="
if [ -f /root/.cloudflared/cert.pem ]; then
  echo "Sudah login (cert.pem ada). Skip."
else
  attempt=0
  until [ -f /root/.cloudflared/cert.pem ]; do
    attempt=$((attempt+1))
    [ $attempt -le 3 ] || { echo "Tunnel login 3x gagal."; exit 1; }
    logf=$(mktemp)
    cloudflared tunnel login >"$logf" 2>&1 &
    pid=$!
    url=""
    for _ in $(seq 1 15); do
      url=$(grep -oE 'https://dash\.cloudflare\.com/argotunnel[^ "]+' "$logf" | head -1 || true)
      [ -n "$url" ] && break; sleep 1
    done
    echo
    echo "===== LOGIN CF Tunnel ====="
    echo "Buka URL ini di browser (HP/laptop manapun):"
    echo
    echo "  ${url:-https://dash.cloudflare.com/argotunnel}"
    echo
    echo "Pilih zona domain kamu, klik Authorize."
    echo "Muncul 'Success'? Tekan ENTER di sini."
    read -r -p "" _
    wait "$pid" 2>/dev/null || true
    rm -f "$logf"
  done
  echo "CF Tunnel login OK (cert.pem ada)."
fi
[ -f /root/.cloudflared/cert.pem ] || { echo "cert.pem tidak ada. Ulangi tunnel login."; exit 1; }

echo
echo "== [4/7] Buat tunnel + ingress + DNS route =="
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
mkdir -p /etc/cloudflared
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
echo "Tunnel $TUNNEL_NAME aktif -> https://$WEB_HOST"

echo
echo "== [5/7] Deploy worker + secrets =="
# Baca EMAIL_API_KEY langsung dari env file (hindari shell expansion di script ini)
_api_key=$(grep '^EMAIL_API_KEY=' "$APP_ENV" | cut -d= -f2-)
_server_url="https://$WEB_HOST"
(cd "$WORKER_DIR" \
  && export CLOUDFLARE_ACCOUNT_ID="$ACCOUNT_ID" \
  && printf '%s\n' "$_server_url" | npx wrangler secret put TEMPMAIL_SERVER_URL \
  && printf '%s\n' "$_api_key" | npx wrangler secret put TEMPMAIL_API_KEY \
  && printf '%s\n' "" | npx wrangler secret put FORWARD_LIST \
  && npx wrangler deploy)
echo "Worker deployed."

echo
echo "== [6/7] Aktifkan Email Routing + catch-all =="
cd "$WORKER_DIR"
npx wrangler email routing enable "$DOMAIN" || true
npx wrangler email routing dns unlock "$DOMAIN" || true
npx wrangler email routing rules update "$DOMAIN" catch-all \
  --name "TempMail catch-all" --enabled \
  --action-type worker --action-value tempmailbyjhopanstore
echo "Catch-all *@$DOMAIN -> worker aktif."

echo
echo "== [7/7] Simpan state + activate domain + verify =="
install -d -o root -g root -m 0700 /etc/tempmail
[ -f "$STATE" ] || printf '{"active_profile":"","profiles":{},"domains":{},"tunnel":{"id":"","account":"","web_hosts":[]}}\n' >"$STATE"
D="$DOMAIN" T="$TUNNEL_ID" W="$WEB_HOST" A="$ACCOUNT_ID" N="$TUNNEL_NAME" python3 - "$STATE" <<'PY'
import json, os, sys
d = json.load(open(sys.argv[1]))
d['active_profile'] = 'wrangler-oauth'
d['profiles']['wrangler-oauth'] = {'label': 'wrangler OAuth', 'account_id': os.environ['A'],
                                   'token': '', 'worker_deployed': True}
d['domains'][os.environ['D']] = {'profile': 'wrangler-oauth', 'zone_id': '',
                                 'enabled': True, 'routing': True}
d['tunnel'].update({'id': os.environ['T'], 'name': os.environ['N'],
                    'web_host': os.environ['W'], 'account': 'cf tunnel'})
open(sys.argv[1], 'w').write(json.dumps(d, indent=2) + '\n')
PY
chmod 600 "$STATE"
tmp=$(mktemp)
grep -v '^DOMAINS=' "$APP_ENV" >"$tmp" || true
printf 'DOMAINS=%s\n' "$DOMAIN" >>"$tmp"
install -o root -g tempmail -m 0640 "$tmp" "$APP_ENV"; rm -f "$tmp"
systemctl restart tempmail
sleep 2
curl -fsS "http://127.0.0.1:${APP_PORT}/health" >/dev/null && echo "Local  : OK"
curl -fsS "https://$WEB_HOST/health" >/dev/null && echo "Tunnel : OK"

echo
echo "=============================================="
echo "  SELESAI"
echo "  Web   : https://$WEB_HOST/app/"
echo "  Email : apa-aja@$DOMAIN -> Worker -> Tunnel -> TempMail"
echo "  Test  : kirim email dari Gmail ke alamat acak @$DOMAIN"
echo "=============================================="
