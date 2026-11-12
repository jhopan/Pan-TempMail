#!/usr/bin/env bash
# TempMail By JhopanStore — Quick Setup (one-liner)
# Usage: curl -fsSL https://raw.githubusercontent.com/jhopan/TempMailByJhopanstore/main/deploy/linux/quick-setup.sh | sudo bash
#
# What this does:
#   1. Download binary from GitHub Releases
#   2. Clone repo (for scripts + worker)
#   3. Run install.sh
#   4. Generate secrets (crypto-safe)
#   5. Ask for domain + configure
#   6. Start service
#   7. Create admin user (ask email + password)
#   8. Print next steps (menutempmail → 8 for Cloudflare)
set -euo pipefail

BOLD='\033[1m'; GREEN='\033[0;32m'; YELLOW='\033[0;33m'; RED='\033[0;31m'; CYAN='\033[0;36m'; NC='\033[0m'
info()  { echo -e "${CYAN}ℹ${NC} $1"; }
ok()    { echo -e "${GREEN}✓${NC} $1"; }
warn()  { echo -e "${YELLOW}⚠${NC} $1"; }
err()   { echo -e "${RED}✗${NC} $1"; }
header() { echo -e "\n${BOLD}=== $1 ===${NC}"; }

REPO_URL="https://github.com/jhopan/TempMailByJhopanstore"
RAW_URL="https://raw.githubusercontent.com/jhopan/TempMailByJhopanstore/main"
RELEASE_URL="https://github.com/jhopan/TempMailByJhopanstore/releases/latest/download"
INSTALL_DIR="/opt/tempmail"
CONFIG_DIR="/etc/tempmail"
CLONE_DIR="/tmp/tempmail-repo"

# Must be root
[ "$(id -u)" -eq 0 ] || { err "Run as root: sudo bash $0"; exit 1; }

header "TempMail By JhopanStore — Quick Setup"
echo -e "      ${BOLD}Credit JhopanStore${NC}"
echo ""

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  BINARY="tempmail-linux-amd64" ;;
  aarch64|arm64) BINARY="tempmail-linux-arm64" ;;
  *) err "Unsupported architecture: $ARCH"; exit 1 ;;
esac
info "Architecture: $ARCH → $BINARY"

# Step 1: Download binary
header "Step 1/7: Download binary"
info "Downloading from GitHub Releases..."
if curl -fsSL "$RELEASE_URL/$BINARY" -o /tmp/tempmail-binary; then
  chmod +x /tmp/tempmail-binary
  ok "Binary downloaded"
else
  err "Failed to download binary. Check internet connection."
  exit 1
fi

# Step 2: Clone repo
header "Step 2/7: Clone repository"
if [ -d "$CLONE_DIR" ]; then
  warn "Repo already exists at $CLONE_DIR, pulling latest..."
  cd "$CLONE_DIR" && git pull --quiet || true
else
  info "Cloning repo..."
  git clone --depth 1 "$REPO_URL" "$CLONE_DIR" --quiet
  ok "Repo cloned"
fi

# Step 3: Install
header "Step 3/7: Install"
cd "$CLONE_DIR"
info "Running install.sh..."
bash deploy/linux/install.sh || true
ok "Install complete"

# Place binary
mkdir -p "$INSTALL_DIR"
cp /tmp/tempmail-binary "$INSTALL_DIR/tempmail"
chown tempmail:tempmail "$INSTALL_DIR/tempmail" 2>/dev/null || true
chmod +x "$INSTALL_DIR/tempmail"
ok "Binary placed at $INSTALL_DIR/tempmail"

# Step 4: Configure
header "Step 4/7: Configure"
ENV_FILE="$CONFIG_DIR/tempmail.env"
mkdir -p "$CONFIG_DIR"

# Ask domain
read -r -p "Domain untuk TempMail (contoh: domainkamu.com): " DOMAIN
[ -n "$DOMAIN" ] || { err "Domain wajib diisi."; exit 1; }
# Sanitize
DOMAIN=$(echo "$DOMAIN" | sed -E 's#^https?://##;s#/.*##;s/$//')

# Ask port (default 8081)
read -r -p "Port untuk TempMail [8081]: " PORT_INPUT
PORT="${PORT_INPUT:-8081}"

# Ask admin email + password
echo ""
info "Buat admin user:"
read -r -p "  Email admin: " ADMIN_EMAIL
[ -n "$ADMIN_EMAIL" ] || { err "Email admin wajib diisi."; exit 1; }
read -r -s -p "  Password admin (min 6): " ADMIN_PASS
echo ""
[ -n "$ADMIN_PASS" ] || { err "Password admin wajib diisi."; exit 1; }
[ ${#ADMIN_PASS} -ge 6 ] || { err "Password minimal 6 karakter."; exit 1; }

# Generate secrets
JWT_SECRET=$(openssl rand -hex 32)
EMAIL_API_KEY=$(openssl rand -hex 32)

cat > "$ENV_FILE" << EOF
DB_DRIVER=sqlite
DB_PATH=$INSTALL_DIR/data/tempmail.db
PORT=$PORT
SMTP_HOST=127.0.0.1
SMTP_PORT=2525
DOMAINS=$DOMAIN
SITE_NAME=TempMail By JhopanStore
CORS_ORIGINS=
CLEANUP_INTERVAL=1h
MESSAGE_RETENTION=24h
ATTACHMENTS_DIR=$INSTALL_DIR/data/attachments
GIN_MODE=release
JWT_SECRET=$JWT_SECRET
EMAIL_API_KEY=$EMAIL_API_KEY
EOF
chmod 640 "$ENV_FILE"
chown root:tempmail "$ENV_FILE" 2>/dev/null || true
ok "Config written to $ENV_FILE"
info "  Domain:     $DOMAIN"
info "  Port:       $PORT"
info "  JWT_SECRET: generated (crypto-safe)"
info "  API_KEY:    generated (crypto-safe)"

# Step 5: Start service
header "Step 5/7: Start service"
systemctl daemon-reload
systemctl enable tempmail
systemctl start tempmail
sleep 2
if systemctl is-active --quiet tempmail; then
  ok "Service running"
else
  err "Service failed to start. Check: journalctl -u tempmail"
  exit 1
fi

# Step 6: Health check
header "Step 6/7: Health check"
if curl -fsS "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then
  ok "Health check passed"
else
  warn "Health check failed. Service might still be starting."
fi

# Step 7: Create admin
header "Step 7/7: Create admin user"
RESULT=$(curl -s -X POST "http://127.0.0.1:$PORT/api/setup" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASS\"}" 2>/dev/null || echo "")

if echo "$RESULT" | grep -q "Admin user created"; then
  ok "Admin created: $ADMIN_EMAIL"
elif echo "$RESULT" | grep -q "already completed"; then
  warn "Admin already exists. Skip."
else
  err "Failed to create admin. Try manually:"
  echo "  curl -X POST http://127.0.0.1:$PORT/api/setup \\"
  echo "    -H 'Content-Type: application/json' \\"
  echo "    -d '{\"email\":\"$ADMIN_EMAIL\",\"password\": ***"
fi

# Done
echo ""
echo -e "${GREEN}${BOLD}==============================================${NC}"
echo -e "${GREEN}${BOLD}  TempMail By JhopanStore — Setup Complete!  ${NC}"
echo -e "${GREEN}${BOLD}        Credit JhopanStore                   ${NC}"
echo -e "${GREEN}${BOLD}==============================================${NC}"
echo ""
echo -e "${BOLD}Local:${NC}  http://127.0.0.1:$PORT/app/"
echo -e "${BOLD}Admin:${NC}  $ADMIN_EMAIL"
echo ""
echo -e "${YELLOW}${BOLD}NEXT STEP — Cloudflare setup:${NC}"
echo -e "  Run: ${BOLD}menutempmail${NC}"
echo -e "  Choose: ${BOLD}8${NC} (SETUP GUIDED)"
echo -e "  This will: Cloudflare API token -> tunnel + connector -> deploy worker -> Email Routing"
echo ""
echo -e "${BOLD}API Key for automation:${NC} $EMAIL_API_KEY"
echo -e "  (also in $ENV_FILE)"
echo ""
rm -f /tmp/tempmail-binary
