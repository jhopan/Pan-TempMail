#!/usr/bin/env bash
# TempMail universal ARM64 installer.
# Run as root after copying this directory and a locally-built ARM64 binary.
set -euo pipefail

ROOT=/opt/tempmail
ENV_DIR=/etc/tempmail
APP_USER=tempmail
BINARY=${1:-./tempmail-linux-arm64}
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../.." && pwd)

[ "$(id -u)" = 0 ] || { echo "Run as root"; exit 1; }
[ -x "$BINARY" ] || { echo "Missing executable: $BINARY"; exit 1; }
[ -f "$REPO_ROOT/cloudflare-worker/src/index.js" ] || { echo "Missing cloudflare-worker source beside deployment package"; exit 1; }

apt-get update -qq
apt-get install -y -qq sqlite3 ca-certificates curl python3
id -u "$APP_USER" >/dev/null 2>&1 || useradd --system --home "$ROOT" --shell /usr/sbin/nologin "$APP_USER"

install -d -o "$APP_USER" -g "$APP_USER" -m 0750 "$ROOT/data"
install -d -o "$APP_USER" -g "$APP_USER" -m 0750 "$ROOT/data/attachments" "$ROOT/backups" "$ROOT/worker" "$ENV_DIR"
install -o root -g root -m 0755 "$BINARY" "$ROOT/tempmail"
install -o root -g root -m 0755 "$SCRIPT_DIR/tempmailctl" /usr/local/sbin/tempmailctl
install -o root -g root -m 0755 "$SCRIPT_DIR/menutempmail" /usr/local/bin/menutempmail
install -o root -g root -m 0755 "$SCRIPT_DIR/setup.sh" "$ROOT/setup.sh"
install -o root -g root -m 0644 "$SCRIPT_DIR/tempmail.service" /etc/systemd/system/tempmail.service
install -o root -g root -m 0644 "$SCRIPT_DIR/tempmail-backup.service" /etc/systemd/system/tempmail-backup.service
install -o root -g root -m 0644 "$SCRIPT_DIR/tempmail-backup.timer" /etc/systemd/system/tempmail-backup.timer

# Root helper: the app stays unprivileged and only writes validated requests into the spool.
install -o root -g root -m 0755 "$SCRIPT_DIR/tempmail-root" /usr/local/sbin/tempmail-root
install -o root -g root -m 0644 "$SCRIPT_DIR/tempmail-root.service" /etc/systemd/system/tempmail-root.service
install -o root -g root -m 0644 "$SCRIPT_DIR/tempmail-root.path" /etc/systemd/system/tempmail-root.path
install -o root -g root -m 0644 "$SCRIPT_DIR/tempmail-root.timer" /etc/systemd/system/tempmail-root.timer
install -d -o "$APP_USER" -g "$APP_USER" -m 0750 "$ROOT/data/helper" "$ROOT/data/helper/in" "$ROOT/data/helper/out"
install -d -o root -g root -m 0700 "$ROOT/data/helper/state"

cp -a "$REPO_ROOT/cloudflare-worker/." "$ROOT/worker/"
chown -R root:root "$ROOT/worker"

if [ ! -f "$ENV_DIR/tempmail.env" ]; then
  cat >"$ENV_DIR/tempmail.env" <<EOF
DB_DRIVER=sqlite
DB_PATH=$ROOT/data/tempmail.db
PORT=8080
SMTP_HOST=127.0.0.1
SMTP_PORT=2525
DOMAINS=local.invalid
SITE_NAME=TempMail By JhopanStore
CORS_ORIGINS=
CLEANUP_INTERVAL=1h
MESSAGE_RETENTION=24h
ATTACHMENTS_DIR=$ROOT/data/attachments
GIN_MODE=release
JWT_SECRET=$(openssl rand -hex 32)
EMAIL_API_KEY=$(openssl rand -hex 32)
CLOUDFLARE_API_BASE=https://api.cloudflare.com/client/v4
CF_WORKER_NAME=tempmailbyjhopanstore
CF_WORKER_COMPAT_DATE=2024-01-01
CF_TUNNEL_NAME=tempmailbyjhopanstore
EOF
  chown root:"$APP_USER" "$ENV_DIR/tempmail.env"
  chmod 640 "$ENV_DIR/tempmail.env"
fi

if ! command -v cloudflared >/dev/null 2>&1; then
  arch=$(dpkg --print-architecture)
  case "$arch" in
    arm64) package=cloudflared-linux-arm64.deb ;;
    amd64) package=cloudflared-linux-amd64.deb ;;
    *) echo "Unsupported architecture for automatic cloudflared install: $arch"; exit 1 ;;
  esac
  curl -fsSL "https://github.com/cloudflare/cloudflared/releases/latest/download/$package" -o /tmp/cloudflared.deb
  dpkg -i /tmp/cloudflared.deb
  rm -f /tmp/cloudflared.deb
fi

systemctl daemon-reload
systemctl enable --now tempmail.service tempmail-backup.timer
systemctl enable --now tempmail-root.path tempmail-root.timer
APP_PORT=$(grep '^PORT=' "$ENV_DIR/tempmail.env" | cut -d= -f2)
APP_PORT=${APP_PORT:-8080}
curl -fsS "http://127.0.0.1:$APP_PORT/health"
printf '\nReady. LAN UI: http://<device-lan-ip>:%s/app/\n' "$APP_PORT"
printf 'Run sudo tempmailctl for guided Cloudflare/Tunnel/Worker setup.\n'
printf 'Buat admin pertama di browser: http://127.0.0.1:%s/app/login.html (form "Buat admin pertama").\n' "$APP_PORT"
printf 'Panel admin (/app/admin.html -> Server) bisa restart service, backup, log, dan tunnel.\n'
printf 'Helper root: tempmail-root.path + tempmail-root.timer, spool %s/data/helper/in\n' "$ROOT"
