#!/bin/bash
set -e

# ============================================
# TempMailByJhopanStore - HTTPS Setup Script
# Let's Encrypt SSL with Certbot + Nginx
# ============================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log() { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err() { echo -e "${RED}[✗]${NC} $1"; }

echo -e "${CYAN}"
echo "============================================"
echo "  TempMailByJhopanStore - HTTPS Setup"
echo "============================================"
echo -e "${NC}"

# Check root
if [ "$EUID" -ne 0 ]; then
  err "Please run as root: sudo bash $0"
  exit 1
fi

# Check Nginx is running
if ! systemctl is-active --quiet nginx; then
  err "Nginx is not running. Run setup.sh first."
  exit 1
fi

# ============================================
# 1. Get domain and email
# ============================================
echo -e "${CYAN}[1/6] Domain Configuration${NC}"
echo ""

read -p "Enter your primary domain (e.g. mail.example.com): " DOMAIN
if [ -z "$DOMAIN" ]; then
  err "Domain is required"
  exit 1
fi

read -p "Enter your email for Let's Encrypt (e.g. admin@example.com): " LE_EMAIL
if [ -z "$LE_EMAIL" ]; then
  err "Email is required"
  exit 1
fi

echo ""
read -p "Add additional domains? (y/n): " ADD_MORE
EXTRA_DOMAINS=""
if [ "$ADD_MORE" = "y" ]; then
  while true; do
    read -p "  Extra domain (blank to stop): " EXTRA
    [ -z "$EXTRA" ] && break
    EXTRA_DOMAINS="$EXTRA_DOMAINS $EXTRA"
  done
fi

echo ""
echo "  Primary domain: $DOMAIN"
echo "  Email: $LE_EMAIL"
[ -n "$EXTRA_DOMAINS" ] && echo "  Extra domains: $EXTRA_DOMAINS"
echo ""
read -p "Proceed? (y/n): " CONFIRM
[ "$CONFIRM" != "y" ] && exit 0

# ============================================
# 2. Install Certbot
# ============================================
echo -e "\n${CYAN}[2/6] Installing Certbot${NC}"

if command -v certbot &> /dev/null; then
  log "Certbot already installed"
else
  apt-get update -qq
  apt-get install -y -qq certbot python3-certbot-nginx
  log "Certbot installed"
fi

# ============================================
# 3. Update Nginx config with server_name
# ============================================
echo -e "\n${CYAN}[3/6] Configuring Nginx${NC}"

NGINX_CONF="/etc/nginx/sites-available/tempmail"

# Build server_name line
SERVER_NAME="$DOMAIN"
for d in $EXTRA_DOMAINS; do
  SERVER_NAME="$SERVER_NAME $d"
done

# Check if server_name already configured
if grep -q "server_name _" "$NGINX_CONF" || grep -q "server_name localhost" "$NGINX_CONF"; then
  # Replace default server_name
  sed -i "s/server_name _;/server_name $SERVER_NAME;/" "$NGINX_CONF"
  sed -i "s/server_name localhost;/server_name $SERVER_NAME;/" "$NGINX_CONF"
  log "Nginx server_name updated: $SERVER_NAME"
elif grep -q "server_name" "$NGINX_CONF"; then
  # Update existing server_name
  sed -i "s/server_name .*/server_name $SERVER_NAME;/" "$NGINX_CONF"
  log "Nginx server_name updated: $SERVER_NAME"
else
  # Add server_name after listen directive
  sed -i "/listen 80/a\\    server_name $SERVER_NAME;" "$NGINX_CONF"
  log "Nginx server_name added: $SERVER_NAME"
fi

# Test and reload Nginx
nginx -t 2>/dev/null
if [ $? -eq 0 ]; then
  systemctl reload nginx
  log "Nginx reloaded"
else
  err "Nginx config test failed! Check $NGINX_CONF"
  exit 1
fi

# ============================================
# 4. Obtain SSL Certificate
# ============================================
echo -e "\n${CYAN}[4/6] Obtaining SSL Certificate${NC}"

CERTBOT_ARGS="--nginx -d $DOMAIN --non-interactive --agree-tos -m $LE_EMAIL"

for d in $EXTRA_DOMAINS; do
  CERTBOT_ARGS="$CERTBOT_ARGS -d $d"
done

echo "  Running: certbot $CERTBOT_ARGS"
echo ""

certbot $CERTBOT_ARGS

if [ $? -eq 0 ]; then
  log "SSL certificate obtained!"
else
  err "Certbot failed. Check:"
  echo "  - DNS A record points to this server"
  echo "  - Port 80 is accessible from internet"
  echo "  - Domain is spelled correctly"
  exit 1
fi

# ============================================
# 5. Setup auto-renewal
# ============================================
echo -e "\n${CYAN}[5/6] Setting up auto-renewal${NC}"

# Certbot auto-renewal timer should already be active
if systemctl is-active --quiet certbot.timer 2>/dev/null; then
  log "Auto-renewal timer already active"
else
  systemctl enable certbot.timer 2>/dev/null || true
  systemctl start certbot.timer 2>/dev/null || true
  log "Auto-renewal enabled"
fi

# Add reload hook
HOOK_DIR="/etc/letsencrypt/renewal-hooks/deploy"
mkdir -p "$HOOK_DIR"
cat > "$HOOK_DIR/reload-nginx.sh" << 'HOOKEOF'
#!/bin/bash
systemctl reload nginx
HOOKEOF
chmod +x "$HOOK_DIR/reload-nginx.sh"
log "Renewal hook: nginx will reload after cert renewal"

# ============================================
# 6. Security hardening
# ============================================
echo -e "\n${CYAN}[6/6] Security Hardening${NC}"

# Add security headers to Nginx
SSL_CONF="/etc/nginx/sites-available/tempmail"

# Check if ssl block exists (certbot should have created it)
if grep -q "listen 443 ssl" "$SSL_CONF"; then
  log "SSL block found in Nginx config"
  
  # Add security headers if not present
  if ! grep -q "Strict-Transport-Security" "$SSL_CONF"; then
    # Add HSTS and security headers in the server block with SSL
    python3 << PYEOF
with open("$SSL_CONF", "r") as f:
    content = f.read()

headers = """
    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # Hide server info
    server_tokens off;
"""

# Insert after listen 443 ssl line
if "listen 443 ssl" in content and "Strict-Transport-Security" not in content:
    content = content.replace(
        "listen 443 ssl",
        "listen 443 ssl\n" + headers,
        1
    )

with open("$SSL_CONF", "w") as f:
    f.write(content)
PYEOF
    log "Security headers added"
  fi
fi

# Redirect HTTP to HTTPS (if not already done by certbot)
if ! grep -q "return 301 https" "$SSL_CONF"; then
  warn "HTTP->HTTPS redirect may need manual setup"
fi

# Test and reload
nginx -t 2>/dev/null
if [ $? -eq 0 ]; then
  systemctl reload nginx
  log "Nginx reloaded with SSL"
else
  warn "Nginx config test issue - check manually"
fi

# ============================================
# Done!
# ============================================
echo -e "\n${GREEN}============================================"
echo "  HTTPS Setup Complete!"
echo "============================================${NC}"
echo ""
echo -e "  ${CYAN}URL:${NC} https://$DOMAIN"
echo -e "  ${CYAN}SSL:${NC} Let's Encrypt (auto-renews every 90 days)"
echo -e "  ${CYAN}HSTS:${NC} Enabled (1 year)"
echo ""
echo "  Certificate files:"
echo "    /etc/letsencrypt/live/$DOMAIN/fullchain.pem"
echo "    /etc/letsencrypt/live/$DOMAIN/privkey.pem"
echo ""
echo "  Test renewal:"
echo "    sudo certbot renew --dry-run"
echo ""
