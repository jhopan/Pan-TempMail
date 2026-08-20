#!/usr/bin/env bash
# TempMail universal ARM64 installer.
# Run as root after copying a locally-built tempmail-linux-arm64 binary here.
set -euo pipefail

ROOT=/opt/tempmail
ENV_DIR=/etc/tempmail
APP_USER=tempmail
BINARY=${1:-./tempmail-linux-arm64}

[ "$(id -u)" = 0 ] || { echo "Run as root"; exit 1; }
[ -x "$BINARY" ] || { echo "Missing executable: $BINARY"; exit 1; }

apt-get update -qq
apt-get install -y -qq sqlite3 ca-certificates
id -u "$APP_USER" >/dev/null 2>&1 || useradd --system --home "$ROOT" --shell /usr/sbin/nologin "$APP_USER"

install -d -o "$APP_USER" -g "$APP_USER" -m 0750 "$ROOT/data/attachments" "$ROOT/backups" "$ENV_DIR"
install -o root -g root -m 0755 "$BINARY" "$ROOT/tempmail"
install -o root -g root -m 0644 tempmail.service /etc/systemd/system/tempmail.service
install -o root -g root -m 0644 tempmail-backup.service /etc/systemd/system/tempmail-backup.service
install -o root -g root -m 0644 tempmail-backup.timer /etc/systemd/system/tempmail-backup.timer

if [ ! -f "$ENV_DIR/tempmail.env" ]; then
  cat >"$ENV_DIR/tempmail.env" <<EOF
DB_DRIVER=sqlite
DB_PATH=$ROOT/data/tempmail.db
PORT=8080
SMTP_HOST=127.0.0.1
SMTP_PORT=2525
DOMAINS=local.invalid
SITE_NAME=TempMail
CORS_ORIGINS=
CLEANUP_INTERVAL=1h
MESSAGE_RETENTION=24h
ATTACHMENTS_DIR=$ROOT/data/attachments
GIN_MODE=release
JWT_SECRET=$(openssl rand -hex 32)
EMAIL_API_KEY=$(openssl rand -hex 32)
EOF
  chown root:"$APP_USER" "$ENV_DIR/tempmail.env"
  chmod 640 "$ENV_DIR/tempmail.env"
fi

systemctl daemon-reload
systemctl enable --now tempmail.service tempmail-backup.timer
curl -fsS http://127.0.0.1:8080/health
printf '\nReady. LAN UI: http://<device-lan-ip>:8080/app/\n'
printf 'Create first admin only once: POST /api/setup\n'
