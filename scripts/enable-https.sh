#!/bin/bash
set -e

# ============================================
# TempMailByJhopanStore - Enable HTTPS
# Let's Encrypt SSL with Nginx
# ============================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

log()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[✗]${NC} $1"; }
info() { echo -e "${CYAN}[i]${NC} $1"; }

echo ""
echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${CYAN}║   TempMailByJhopanStore - Enable HTTPS       ║${NC}"
echo -e "${BOLD}${CYAN}║   Let's Encrypt SSL Certificate              ║${NC}"
echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════╝${NC}"
echo ""

# Check root
if [ "$EUID" -ne 0 ]; then
  err "Please run as root: sudo bash scripts/enable-https.sh"
  exit 1
fi

# ============================================
# Step 1: Install Certbot
# ============================================
echo -e "${BOLD}[1/6] Installing Certbot...${NC}"

if command -v certbot &>/dev/null; then
  log "Certbot already installed"
else
  apt-get update -qq
  apt-get install -y -qq certbot python3-certbot-nginx
  log "Certbot installed"
fi

# ============================================
# Step 2: Get domain
# ============================================
echo ""
echo -e "${BOLD}[2/6] Domain Configuration${NC}"
echo ""
echo -e "${CYAN}Enter the domain for HTTPS (e.g., mail.example.com):${NC}"
read -p "Domain: " DOMAIN

if [ -z "$DOMAIN" ]; then
  err "Domain cannot be empty"
  exit 1
fi

# Validate domain format
if ! echo "$DOMAIN" | grep -qP '^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*\.[a-zA-Z]{2,}$'; then
  err "Invalid domain format"
  exit 1
fi

log "Domain: $DOMAIN"

# ============================================
# Step 3: Get email
# ============================================
echo ""
echo -e "${BOLD}[3/6] Let's Encrypt Email${NC}"
echo -e "${CYAN}Email for SSL notifications & renewal reminders:${NC}"
read -p "Email: " LE_EMAIL

if [ -z "$LE_EMAIL" ]; then
  err "Email cannot be empty"
  exit 1
fi

log "Email: $LE_EMAIL"

# ============================================
# Step 4: DNS check
# ============================================
echo ""
echo -e "${BOLD}[4/6] Verifying DNS...${NC}"

SERVER_IP=$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')
DOMAIN_IP=$(dig +short "$DOMAIN" 2>/dev/null | head -1)

info "Server IP:  $SERVER_IP"
info "Domain IP:  ${DOMAIN_IP:-NOT RESOLVED}"

if [ -z "$DOMAIN_IP" ]; then
  warn "Domain $DOMAIN does not resolve to any IP!"
  warn "Please create an A record pointing to $SERVER_IP"
  echo ""
  read -p "Continue anyway? (y/N): " CONTINUE
  if [ "$CONTINUE" != "y" ] && [ "$CONTINUE" != "Y" ]; then
    err "Aborted. Fix DNS first."
    exit 1
  fi
elif [ "$DOMAIN_IP" != "$SERVER_IP" ]; then
  warn "Domain resolves to $DOMAIN_IP but server is $SERVER_IP"
  warn "Certbot needs the domain to point to this server"
  echo ""
  read -p "Continue anyway? (y/N): " CONTINUE
  if [ "$CONTINUE" != "y" ] && [ "$CONTINUE" != "Y" ]; then
    err "Aborted. Fix DNS first."
    exit 1
  fi
else
  log "DNS verified: $DOMAIN -> $SERVER_IP"
fi

# ============================================
# Step 5: Update Nginx config
# ============================================
echo ""
echo -e "${BOLD}[5/6] Configuring Nginx for $DOMAIN...${NC}"

NGINX_CONF="/etc/nginx/sites-available/tempmail"

# Check if nginx config exists
if [ ! -f "$NGINX_CONF" ]; then
  err "Nginx config not found at $NGINX_CONF"
  err "Please run setup.sh first"
  exit 1
fi

# Update server_name in nginx config
sed -i "s/server_name _;/server_name $DOMAIN;/g" "$NGINX_CONF"
sed -i "s/server_name .*;/server_name $DOMAIN;/g" "$NGINX_CONF"

# Test nginx
nginx -t 2>/dev/null
if [ $? -ne 0 ]; then
  err "Nginx config test failed"
  exit 1
fi

systemctl reload nginx
log "Nginx updated and reloaded"

# ============================================
# Step 6: Get SSL Certificate
# ============================================
echo ""
echo -e "${BOLD}[6/6] Obtaining SSL Certificate...${NC}"

certbot --nginx \
  -d "$DOMAIN" \
  --email "$LE_EMAIL" \
  --agree-tos \
  --non-interactive \
  --redirect \
  --no-eff-email

if [ $? -eq 0 ]; then
  log "SSL certificate installed successfully!"
else
  err "Certbot failed. Check logs: journalctl -u certbot"
  exit 1
fi

# ============================================
# Post-setup: Auto-renewal
# ============================================
echo ""
echo -e "${BOLD}Setting up auto-renewal...${NC}"

# Certbot auto-renewal is already set up by the package
# But let's add a systemd timer for extra safety
cat > /etc/systemd/system/certbot-renew.service << 'RENEWEOF'
[Unit]
Description=Renew Let's Encrypt certificates
After=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/bin/certbot renew --quiet --deploy-hook "systemctl reload nginx"
RENEWEOF

cat > /etc/systemd/system/certbot-renew.timer << 'TIMEREOF'
[Unit]
Description=Check Let's Encrypt certificates daily

[Timer]
OnCalendar=*-*-* 03:00:00
RandomizedDelaySec=3600
Persistent=true

[Install]
WantedBy=timers.target
TIMEREOF

systemctl daemon-reload
systemctl enable --now certbot-renew.timer
log "Auto-renewal enabled (daily check at 3 AM)"

# ============================================
# Summary
# ============================================
echo ""
echo -e "${BOLD}${GREEN}╔══════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${GREEN}║   HTTPS Enabled Successfully!                ║${NC}"
echo -e "${BOLD}${GREEN}╚══════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  ${CYAN}URL:${NC}      https://$DOMAIN"
echo -e "  ${CYAN}SSL:${NC}      Let's Encrypt (auto-renewal)"
echo -e "  ${CYAN}Redirect:${NC} HTTP -> HTTPS"
echo ""
echo -e "  ${YELLOW}Test:${NC} curl -I https://$DOMAIN"
echo -e "  ${YELLOW}Cert:${NC} certbot certificates"
echo -e "  ${YELLOW}Renew:${NC} certbot renew --dry-run"
echo ""
