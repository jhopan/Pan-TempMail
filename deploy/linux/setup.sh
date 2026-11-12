#!/usr/bin/env bash
# TempMail guided setup (Linux target). Kontrol Cloudflare penuh lewat API token milik
# profil di database aplikasi: tanpa login browser, tanpa CLI Node.js, tanpa tooling eksternal.
# Usage: sudo bash setup.sh [domain] [web-host]
#   Bisa juga pakai env CLOUDFLARE_ACCOUNT_ID / CLOUDFLARE_API_TOKEN untuk melewati prompt.
set -euo pipefail

APP_ENV=/etc/tempmail/tempmail.env
APP_BIN=/opt/tempmail/tempmail
TUNNEL_NAME=tempmailbyjhopanstore
PROFILE_ID=cf-main

JWT=""
API_CODE=""
API_BODY=""
FAILED=0

die() { echo "ERROR: $*" >&2; exit 1; }

[ "$(id -u)" = 0 ] || die "Jalankan sebagai root: sudo bash setup.sh"

echo "=============================================="
echo "  Pan-TempMail - Guided Setup"
echo "        Credit JhopanStore"
echo "=============================================="

APP_PORT=$(grep '^PORT=' "$APP_ENV" 2>/dev/null | cut -d= -f2 || true)
APP_PORT=${APP_PORT:-8080}
APP_URL="http://127.0.0.1:$APP_PORT"

# api_call METHOD PATH [JSON] [quiet]
# Semua panggilan HTTP lewat sini. Status disimpan di API_CODE/API_BODY.
# Non-2xx: cetak error + detail dari aplikasi (kecuali mode quiet), return non-zero.
api_call() {
  local method=$1 path=$2 payload=${3:-} mode=${4:-}
  local out
  local -a args=(-sS -m 120 -X "$method" -w '\n%{http_code}')
  if [ -n "$JWT" ]; then
    args+=(-H "Authorization: Bearer $JWT")
  fi
  if [ -n "$payload" ]; then
    args+=(-H 'Content-Type: application/json' --data "$payload")
  fi
  out=$(curl "${args[@]}" "$APP_URL$path" || true)
  API_CODE=${out##*$'\n'}
  API_BODY=${out%$'\n'*}
  case "$API_CODE" in
    2*) return 0 ;;
  esac
  if [ "$mode" != quiet ]; then
    api_show_error "$method" "$path"
  fi
  return 1
}

# api_show_error METHOD PATH: cetak pesan error aplikasi beserta detail mentah Cloudflare.
api_show_error() {
  echo "GAGAL [$1 $2] HTTP ${API_CODE:-000}"
  if [ -z "$API_BODY" ]; then
    echo "  Tidak ada respons dari aplikasi. Cek: systemctl status tempmail"
    return 0
  fi
  printf '%s' "$API_BODY" | python3 -c '
import json, sys
raw = sys.stdin.read()
try:
    data = json.loads(raw)
except Exception:
    print("  " + raw[:400].strip())
    raise SystemExit(0)
if isinstance(data, dict) and (data.get("error") or data.get("detail")):
    for key in ("error", "detail"):
        if data.get(key):
            print("  " + key + ": " + str(data[key]))
else:
    print("  " + raw[:400].strip())
' 2>/dev/null || echo "  $API_BODY"
}

# json_get JSON key [key ...]: ambil nilai dari respons, kosong kalau tidak ada.
json_get() {
  python3 -c '
import json, sys
try:
    node = json.loads(sys.argv[1])
except Exception:
    raise SystemExit(0)
for key in sys.argv[2:]:
    if isinstance(node, dict):
        node = node.get(key)
    elif isinstance(node, list) and key.isdigit() and int(key) < len(node):
        node = node[int(key)]
    else:
        node = None
    if node is None:
        raise SystemExit(0)
print(node if not isinstance(node, (dict, list)) else json.dumps(node))
' "$@" 2>/dev/null || true
}

json_body() {
  # json_body KEY=value ...: bikin JSON objek.
  #   nilai diawali @  -> dibaca dari environment (supaya token tidak muncul di argv)
  #   KEY:raw=literal  -> nilai JSON apa adanya, misalnya true
  python3 -c '
import json, os, sys
out = {}
for pair in sys.argv[1:]:
    key, _, value = pair.partition("=")
    if key.endswith(":raw"):
        if value != "":
            out[key[:-4]] = json.loads(value)
        continue
    if value.startswith("@"):
        value = os.environ.get(value[1:], "")
    if value != "":
        out[key] = value
print(json.dumps(out))
' "$@" 2>/dev/null || true
}

login_help() {
  echo
  echo "Set atau reset kredensial admin lewat menu:"
  echo "  sudo tempmailctl  ->  A (Kelola admin)  ->  3 (Reset password admin)"
  echo "Setelah itu jalankan skrip ini lagi."
}

connector_help() {
  echo "Konektor juga bisa dipasang ulang dari panel admin:"
  echo "  https://$WEB_HOST/admin  ->  bagian Tunnel  ->  tombol Pasang/Update"
  echo "Kalau masih belum bisa: hapus tunnel $TUNNEL_NAME di dashboard Cloudflare"
  echo "(Zero Trust -> Networks -> Tunnels), lalu jalankan skrip ini lagi."
}

# install_connector JSON: jalankan perintah root dari aplikasi satu per satu.
install_connector() {
  local body=$1 cmd shown i=0 failed=0
  echo "Menjalankan perintah konektor dari aplikasi (token konektor tidak dicetak):"
  while IFS= read -r cmd; do
    [ -n "$cmd" ] || continue
    case "$cmd" in
      \#*) echo "  info:${cmd#\#}"; continue ;;
    esac
    i=$((i + 1))
    shown=$cmd
    case "$cmd" in
      *"cloudflared service install "*) shown="${cmd%%cloudflared service install *}cloudflared service install <token-konektor disembunyikan>" ;;
    esac
    echo "  [$i] $shown"
    if ! bash -c "${cmd#sudo }"; then
      case "$cmd" in
        *"systemctl status"*) echo "  (perintah status gagal, tidak masalah)" ;;
        *) echo "  PERINGATAN: perintah di atas gagal."; failed=1 ;;
      esac
    fi
  done < <(printf '%s' "$body" | python3 -c '
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    data = {}
for command in data.get("commands", []) or []:
    print(command)
' 2>/dev/null || true)
  if [ "$failed" = 0 ] && systemctl is-active --quiet cloudflared; then
    echo "Konektor cloudflared aktif."
    return 0
  fi
  echo "Service cloudflared belum aktif. Jalankan perintah ini sebagai root"
  echo "PERINGATAN: teks di bawah memuat token konektor, jangan dibagikan atau disimpan di chat:"
  printf '%s' "$body" | python3 -c '
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    data = {}
for command in data.get("commands", []) or []:
    print("  " + command)
' 2>/dev/null || true
  connector_help
  return 1
}

echo
echo "== [1/7] Periksa prasyarat =="
[ -f "$APP_ENV" ] || die "$APP_ENV tidak ada. Jalankan deploy/linux/install.sh dulu."
[ -x "$APP_BIN" ] || die "$APP_BIN tidak ada. Jalankan deploy/linux/install.sh dulu."
MISSING=""
command -v python3 >/dev/null 2>&1 || MISSING="$MISSING python3"
command -v curl >/dev/null 2>&1 || MISSING="$MISSING curl"
if [ -n "$MISSING" ]; then
  echo "Install paket yang kurang:$MISSING"
  apt-get update -qq
  # shellcheck disable=SC2086
  apt-get install -y -qq $MISSING ca-certificates
fi
if ! command -v cloudflared >/dev/null 2>&1; then
  echo "Install cloudflared dari paket GitHub..."
  arch=$(dpkg --print-architecture)
  case "$arch" in
    arm64) pkg=cloudflared-linux-arm64.deb ;;
    amd64) pkg=cloudflared-linux-amd64.deb ;;
    *) die "Arsitektur tidak didukung: $arch" ;;
  esac
  curl -fsSL "https://github.com/cloudflare/cloudflared/releases/latest/download/$pkg" -o /tmp/cf.deb
  dpkg -i /tmp/cf.deb
  rm -f /tmp/cf.deb
fi
if ! curl -fsS --max-time 5 "$APP_URL/health" >/dev/null 2>&1; then
  echo "Aplikasi belum menjawab di $APP_URL, coba start service tempmail..."
  systemctl restart tempmail >/dev/null 2>&1 || true
  for _ in $(seq 1 15); do
    curl -fsS --max-time 5 "$APP_URL/health" >/dev/null 2>&1 && break
    sleep 1
  done
fi
curl -fsS --max-time 5 "$APP_URL/health" >/dev/null 2>&1 || die "Aplikasi tidak hidup di $APP_URL. Cek: systemctl status tempmail"
echo "Aplikasi hidup di $APP_URL"
echo "cloudflared $(cloudflared --version 2>&1 | head -1)"

echo
echo "== [2/7] Kredensial Cloudflare =="
if [ $# -ge 1 ]; then
  DOMAIN=$1
else
  read -r -p "Domain untuk TempMail (contoh: domainkamu.com): " DOMAIN || true
fi
DOMAIN=$(printf '%s' "$DOMAIN" | sed -E 's#^https?://##; s#/+$##' | tr 'A-Z' 'a-z')
[ -n "$DOMAIN" ] || die "Domain wajib diisi."
if [ $# -ge 2 ]; then
  WEB_HOST=$2
elif [ -z "${WEB_HOST:-}" ]; then
  read -r -p "Web host untuk panel (default tempmail.$DOMAIN): " WEB_HOST || true
fi
WEB_HOST=$(printf '%s' "${WEB_HOST:-}" | sed -E 's#^https?://##; s#/+$##' | tr 'A-Z' 'a-z')
WEB_HOST=${WEB_HOST:-tempmail.$DOMAIN}
ACCOUNT_ID=${ACCOUNT_ID:-${CLOUDFLARE_ACCOUNT_ID:-}}
if [ -z "$ACCOUNT_ID" ]; then
  read -r -p "Cloudflare Account ID: " ACCOUNT_ID || true
fi
ACCOUNT_ID=$(printf '%s' "$ACCOUNT_ID" | tr -d '[:space:]')
[ -n "$ACCOUNT_ID" ] || die "Account ID wajib diisi."
CF_TOKEN=${CLOUDFLARE_API_TOKEN:-}
if [ -n "$CF_TOKEN" ]; then
  echo "API token dibaca dari env CLOUDFLARE_API_TOKEN."
else
  cat <<'TXT'
API token butuh permission:
  - Account Settings:Read    (verifikasi Account ID)
  - Zone:Read                (cari zone domain)
  - Zone Settings:Edit       (aktifkan Email Routing + baca status DNS routing)
  - DNS:Edit                 (buat CNAME tunnel + record Email Routing)
  - Email Routing Rules:Edit (aktifkan routing + catch-all)
  - Workers Scripts:Edit     (deploy Worker)
  - Cloudflare Tunnel:Edit   (buat tunnel + ambil token konektor)
Buat di https://dash.cloudflare.com -> My Profile -> API Tokens -> Create Token
TXT
  read -r -s -p "Cloudflare API token (tidak akan ditampilkan): " CF_TOKEN || true
  echo
fi
[ -n "$CF_TOKEN" ] || die "API token wajib diisi."
# Diekspor hanya sebentar supaya json_body bisa membacanya tanpa menaruh token di argv.
export CF_TOKEN
echo "Domain : $DOMAIN"
echo "Web    : https://$WEB_HOST"
echo "Tunnel : $TUNNEL_NAME"
echo "Profil : $PROFILE_ID"

echo
echo "== [3/7] Login ke aplikasi =="
ADMIN_EMAIL=$(grep '^ADMIN_EMAIL=' "$APP_ENV" 2>/dev/null | cut -d= -f2- || true)
ADMIN_PASS=$(grep '^ADMIN_PASS=' "$APP_ENV" 2>/dev/null | cut -d= -f2- || true)
if [ -z "$ADMIN_EMAIL" ]; then
  read -r -p "Email admin: " ADMIN_EMAIL
fi
if [ -z "$ADMIN_PASS" ]; then
  read -r -s -p "Password admin (tidak ditampilkan): " ADMIN_PASS || true
  echo
fi
if [ -z "$ADMIN_EMAIL" ] || [ -z "$ADMIN_PASS" ]; then
  echo "Email dan password admin wajib diisi."
  login_help
  exit 1
fi
LOGIN_BODY=$(TM_EMAIL="$ADMIN_EMAIL" TM_PASS="$ADMIN_PASS" python3 -c 'import json, os
print(json.dumps({"email": os.environ["TM_EMAIL"], "password": os.environ["TM_PASS"]}))' 2>/dev/null || true)
if ! api_call POST /api/auth/login "$LOGIN_BODY" quiet; then
  echo "Login admin gagal, kredensial di $APP_ENV ditolak:"
  LOGIN_BODY=""
  api_show_error POST /api/auth/login
  login_help
  exit 1
fi
JWT=$(json_get "$API_BODY" token)
LOGIN_BODY=""
[ -n "$JWT" ] || { echo "Respons login tidak memuat token."; login_help; exit 1; }
echo "Login OK (JWT hanya hidup di variabel skrip ini)."

echo
echo "== [4/7] Profil Cloudflare ($PROFILE_ID) =="
PROFILE_BODY=$(json_body "id=$PROFILE_ID" "label=Cloudflare API token" "account_id=$ACCOUNT_ID" "token=@CF_TOKEN" "activate:raw=true")
if api_call POST /api/admin/cloudflare/profiles "$PROFILE_BODY" quiet; then
  echo "Profil $PROFILE_ID dibuat dan langsung diaktifkan."
else
  if [ "$API_CODE" = 409 ]; then
    echo "Profil $PROFILE_ID sudah ada, putar token dan Account ID-nya."
    UPDATE_BODY=$(json_body "account_id=$ACCOUNT_ID" "token=@CF_TOKEN")
    api_call PUT "/api/admin/cloudflare/profiles/$PROFILE_ID" "$UPDATE_BODY" || die "Gagal memperbarui profil $PROFILE_ID."
    api_call POST "/api/admin/cloudflare/profiles/$PROFILE_ID/activate" "" quiet || echo "Peringatan: profil $PROFILE_ID tidak bisa diaktifkan."
    echo "Profil $PROFILE_ID diperbarui."
  else
    api_show_error POST /api/admin/cloudflare/profiles
    echo "Periksa Account ID dan permission API token di atas."
    exit 1
  fi
fi
PROFILE_BODY=""
UPDATE_BODY=""
unset CF_TOKEN

echo
echo "== [5/7] Tunnel + konektor =="
TUNNEL_BODY=$(json_body "name=$TUNNEL_NAME" "web_host=$WEB_HOST")
TUNNEL_ID=""
if api_call POST /api/admin/cloudflare/tunnel/create "$TUNNEL_BODY" quiet; then
  TUNNEL_ID=$(json_get "$API_BODY" id)
  echo "Tunnel $TUNNEL_NAME siap dipakai."
  install_connector "$API_BODY" || FAILED=1
else
  CREATE_CODE=$API_CODE
  CREATE_BODY=$API_BODY
  if api_call GET /api/admin/cloudflare/tunnel "" quiet; then
    TUNNEL_ID=$(json_get "$API_BODY" id)
  fi
  if [ -z "$TUNNEL_ID" ]; then
    API_CODE=$CREATE_CODE
    API_BODY=$CREATE_BODY
    api_show_error POST /api/admin/cloudflare/tunnel/create
    echo "Periksa permission API token (butuh Cloudflare Tunnel:Edit) dan Account ID."
    exit 1
  fi
  if systemctl is-active --quiet cloudflared; then
    echo "Tunnel $TUNNEL_NAME sudah ada dan konektor cloudflared aktif."
  else
    echo "Tunnel $TUNNEL_NAME sudah ada, tapi service cloudflared tidak aktif."
    API_CODE=$CREATE_CODE
    API_BODY=$CREATE_BODY
    api_show_error POST /api/admin/cloudflare/tunnel/create
    connector_help
    FAILED=1
  fi
fi
TUNNEL_PUT=$(json_body "web_host=$WEB_HOST" "name=$TUNNEL_NAME" "id=${TUNNEL_ID:-}")
api_call PUT /api/admin/cloudflare/tunnel "$TUNNEL_PUT" || die "Gagal menyimpan pengaturan tunnel ke panel."
TUNNEL_BODY=""
TUNNEL_PUT=""
echo "Panel sekarang mencatat tunnel $TUNNEL_NAME untuk web host $WEB_HOST."

echo
echo "== [6/7] Domain + Email Routing =="
DOMAIN_BODY=$(json_body "domain=$DOMAIN")
if api_call POST /api/admin/cloudflare/domains "$DOMAIN_BODY" quiet; then
  echo "Domain $DOMAIN terdaftar dan zone-nya ditemukan."
else
  if [ "$API_CODE" = 409 ]; then
    echo "Domain $DOMAIN sudah terdaftar, lanjut ke Email Routing."
  else
    api_show_error POST /api/admin/cloudflare/domains
    echo "Pastikan domain ada di akun Cloudflare dengan Account ID di atas"
    echo "dan status zone-nya Active, lalu jalankan skrip ini lagi."
    exit 1
  fi
fi
if api_call POST "/api/admin/cloudflare/domains/$DOMAIN/routing" '{"routing":true}' quiet; then
  echo "Email Routing aktif dan catch-all diarahkan ke Worker."
else
  api_show_error POST "/api/admin/cloudflare/domains/$DOMAIN/routing"
  echo "Yang perlu dibetulkan operator:"
  echo "  1. Cek status zone $DOMAIN sudah Active (nameserver Cloudflare sudah dipakai)."
  echo "  2. Hapus record MX lama yang bukan milik Cloudflare, misalnya mail.$DOMAIN."
  echo "     Dashboard: https://dash.cloudflare.com -> $DOMAIN -> Email -> Email Routing."
  echo "  3. Cek permission API token: Zone:Read dan Email Routing Rules:Edit."
  echo "Setelah dibetulkan, jalankan skrip ini lagi."
  FAILED=1
fi

echo
echo "== [7/7] Deploy Worker + verifikasi =="
WORKER_NAME=$(grep '^CF_WORKER_NAME=' "$APP_ENV" 2>/dev/null | cut -d= -f2- || true)
WORKER_NAME=${WORKER_NAME:-tempmailbyjhopanstore}
if api_call POST "/api/admin/cloudflare/profiles/$PROFILE_ID/deploy-worker" '{}' quiet; then
  WORKER_DEPLOYED=$(json_get "$API_BODY" modified_on)
  echo "Worker $WORKER_NAME terunggah (secrets: TEMPMAIL_SERVER_URL, TEMPMAIL_API_KEY, FORWARD_LIST)."
  if [ -n "$WORKER_DEPLOYED" ]; then
    echo "Versi Worker: $WORKER_DEPLOYED"
  fi
else
  api_show_error POST "/api/admin/cloudflare/profiles/$PROFILE_ID/deploy-worker"
  echo "Cek permission API token (Workers Scripts:Edit) dan web host tunnel di panel."
  FAILED=1
fi

echo
echo "Verifikasi:"
if curl -fsS --max-time 10 "$APP_URL/health" >/dev/null 2>&1; then
  echo "  Lokal /health          : OK"
else
  echo "  Lokal /health          : GAGAL (cek: systemctl status tempmail)"
  FAILED=1
fi
if api_call GET /api/config "" quiet; then
  echo "  Lokal /api/config      : OK"
  case "$API_BODY" in
    *"$DOMAIN"*) echo "  Domain di /api/config  : $DOMAIN terdaftar" ;;
    *) echo "  Domain di /api/config  : $DOMAIN belum muncul, buka ulang setelah routing aktif" ;;
  esac
else
  echo "  Lokal /api/config      : GAGAL"
  api_show_error GET /api/config
  FAILED=1
fi
if curl -fsS --max-time 15 "https://$WEB_HOST/health" >/dev/null 2>&1; then
  echo "  Publik https://$WEB_HOST : OK"
else
  echo "  Publik https://$WEB_HOST : belum bisa dicek, pastikan CNAME $WEB_HOST -> tunnel aktif"
fi

echo
echo "=============================================="
if [ "$FAILED" = 0 ]; then
  echo "  SELESAI"
else
  echo "  SELESAI SEBAGIAN - ada langkah yang gagal, baca pesan di atas"
fi
echo "  Domain  : $DOMAIN (Email Routing -> Worker $WORKER_NAME)"
echo "  Worker  : $WORKER_NAME di akun $ACCOUNT_ID"
echo "  Tunnel  : $TUNNEL_NAME -> http://127.0.0.1:$APP_PORT"
echo "  Web     : https://$WEB_HOST/app/"
echo "  Panel   : https://$WEB_HOST/admin"
echo "  Email   : apa-aja@$DOMAIN -> Worker -> Tunnel -> TempMail"
echo "  Test    : kirim email dari Gmail ke alamat acak @$DOMAIN"
echo "=============================================="
[ "$FAILED" = 0 ] || exit 1
