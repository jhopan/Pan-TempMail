#!/bin/bash
set -e

# ============================================
# TempMailByJhopanStore - Setup Script
# Fresh install untuk VPS Ubuntu 22.04/24.04
# ============================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

log()  { echo -e "${GREEN}[✓]${NC} $1"; }
warn() { echo -e "${YELLOW}[!]${NC} $1"; }
err()  { echo -e "${RED}[✗]${NC} $1"; }
info() { echo -e "${CYAN}[i]${NC} $1"; }

banner() {
  echo ""
  echo -e "${CYAN}${BOLD}============================================${NC}"
  echo -e "${CYAN}${BOLD}  TempMailByJhopanStore - Setup${NC}"
  echo -e "${CYAN}${BOLD}============================================${NC}"
  echo ""
}

check_root() {
  if [ "$EUID" -ne 0 ]; then
    err "Script ini harus dijalankan sebagai root (sudo)"
    exit 1
  fi
}

# ------------------------------------------
# STEP 1: System Dependencies
# ------------------------------------------
install_system_deps() {
  info "Installing system dependencies..."
  apt update -qq && apt upgrade -y -qq
  apt install -y -qq curl wget git build-essential \
    software-properties-common apt-transport-https \
    ca-certificates gnupg lsb-release ufw \
    python3 python3-pip > /dev/null 2>&1
  log "System packages installed"
}

# ------------------------------------------
# STEP 2: PostgreSQL
# ------------------------------------------
install_postgresql() {
  if command -v psql &> /dev/null; then
    log "PostgreSQL already installed"
    return
  fi
  info "Installing PostgreSQL..."
  apt install -y -qq postgresql postgresql-contrib > /dev/null 2>&1
  systemctl enable postgresql > /dev/null 2>&1
  systemctl start postgresql
  log "PostgreSQL installed and started"
}

setup_database() {
  info "Setting up database..."

  read -p "  DB password for 'tempmail' user: " -s DB_PASS
  echo ""

  sudo -u postgres psql -c "CREATE DATABASE tempmail;" 2>/dev/null || true
  sudo -u postgres psql -c "CREATE USER tempmail WITH PASSWORD '${DB_PASS}';" 2>/dev/null || true
  sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE tempmail TO tempmail;" 2>/dev/null
  sudo -u postgres psql -c "ALTER USER tempmail CREATEDB;" 2>/dev/null

  sudo -u postgres psql -d tempmail -c "
    CREATE TABLE IF NOT EXISTS inboxes (
      id SERIAL PRIMARY KEY,
      email TEXT NOT NULL UNIQUE,
      type TEXT NOT NULL,
      owner_id INTEGER,
      expires_at TIMESTAMP,
      created_at TIMESTAMP DEFAULT NOW()
    );
    CREATE TABLE IF NOT EXISTS messages (
      id SERIAL PRIMARY KEY,
      inbox_id INTEGER REFERENCES inboxes(id) ON DELETE CASCADE,
      sender TEXT,
      subject TEXT,
      html_body TEXT,
      text_body TEXT,
      received_at TIMESTAMP DEFAULT NOW()
    );
    CREATE TABLE IF NOT EXISTS attachments (
      id SERIAL PRIMARY KEY,
      message_id INTEGER REFERENCES messages(id) ON DELETE CASCADE,
      filename TEXT,
      content_type TEXT,
      size INTEGER,
      data BYTEA
    );
    CREATE TABLE IF NOT EXISTS users (
      id SERIAL PRIMARY KEY,
      email VARCHAR(255) UNIQUE NOT NULL,
      password_hash VARCHAR(255) NOT NULL,
      role VARCHAR(50) NOT NULL DEFAULT 'user',
      created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
      updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
  " 2>/dev/null

  # Create admin user
  local HASH
  HASH=$(python3 -c "import bcrypt; print(bcrypt.hashpw(b'your-password', bcrypt.gensalt()).decode())")
  sudo -u postgres psql -d tempmail -c \
    "INSERT INTO users (email, password_hash, role) VALUES ('your@email.com', '${HASH}', 'admin') ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash;" 2>/dev/null

  # Grant table privileges
  sudo -u postgres psql -d tempmail -c "GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO tempmail;" 2>/dev/null
  sudo -u postgres psql -d tempmail -c "GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO tempmail;" 2>/dev/null

  log "Database setup complete"
  echo "    Admin: your@email.com / your-password"
}

# ------------------------------------------
# STEP 3: Go
# ------------------------------------------
install_go() {
  if command -v go &> /dev/null; then
    log "Go already installed: $(go version)"
    return
  fi
  info "Installing Go 1.25..."
  wget -q https://go.dev/dl/go1.25.0.linux-amd64.tar.gz -O /tmp/go.tar.gz
  tar -C /usr/local -xzf /tmp/go.tar.gz
  rm /tmp/go.tar.gz
  echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile.d/go.sh
  export PATH=$PATH:/usr/local/go/bin
  log "Go installed: $(/usr/local/go/bin/go version)"
}

# ------------------------------------------
# STEP 4: Node.js
# ------------------------------------------
install_nodejs() {
  if command -v node &> /dev/null; then
    log "Node.js already installed: $(node --version)"
    return
  fi
  info "Installing Node.js 20..."
  curl -fsSL https://deb.nodesource.com/setup_20.x | bash - > /dev/null 2>&1
  apt install -y -qq nodejs > /dev/null 2>&1
  log "Node.js installed: $(node --version)"
}

# ------------------------------------------
# STEP 5: Maddy Mail Server
# ------------------------------------------
install_maddy() {
  if command -v maddy &> /dev/null; then
    log "Maddy already installed"
    return
  fi
  info "Installing Maddy mail server..."
  local MADDY_URL="https://github.com/foxcpp/maddy/releases/download/v0.7.1/maddy-0.7.1-x86_64-linux.tar.gz"
  wget -q "$MADDY_URL" -O /tmp/maddy.tar.gz
  tar xzf /tmp/maddy.tar.gz -C /tmp/
  cp /tmp/maddy-*/maddy /usr/local/bin/
  cp /tmp/maddy-*/maddy.1 /usr/share/man/man1/ 2>/dev/null || true
  rm -rf /tmp/maddy*
  useradd -r -m -d /var/lib/maddy -s /sbin/nologin maddy 2>/dev/null || true
  mkdir -p /etc/maddy /var/lib/maddy
  log "Maddy installed"
}

# ------------------------------------------
# STEP 6: Nginx
# ------------------------------------------
install_nginx() {
  if command -v nginx &> /dev/null; then
    log "Nginx already installed"
    return
  fi
  info "Installing Nginx..."
  apt install -y -qq nginx > /dev/null 2>&1
  systemctl enable nginx > /dev/null 2>&1
  log "Nginx installed"
}

# ------------------------------------------
# STEP 7: Configure Services
# ------------------------------------------
configure_services() {
  info "Configuring services..."

  # --- .env ---
  read -p "  Server IP address: " SERVER_IP
  read -p "  Primary domain (e.g. mail.example.com): " DOMAIN

  local JWT_SECRET
  JWT_SECRET=*** 9b7d1dde5e66918fff76e5f6963fc328c5840ec5715deb43bbb4b816a900d9

  cat > "${PROJECT_DIR}/receiver/.env" << EOF
DB_HOST=localhost
DB_PORT=5432
DB_USER=tempmail
DB_PASSWORD=***
DB_NAME=tempmail
JWT_SECRET=***
PORT=8080
LMTP_PORT=8024
DOMAINS=${DOMAIN}
EOF
  log "Backend .env created"

  # --- Maddy ---
  cp "${PROJECT_DIR}/config/maddy/maddy.conf" /etc/maddy/maddy.conf
  sed -i "s/your-domain.com/${DOMAIN}/g" /etc/maddy/maddy.conf
  chown -R maddy:maddy /etc/maddy /var/lib/maddy
  log "Maddy configured"

  # Replace placeholders in frontend
  sed -i "s/__PRIMARY_DOMAIN__/${DOMAIN}/g" "${PROJECT_DIR}/frontend/src/app/page.tsx"

  # Replace placeholders in add-domain.sh
  sed -i "s/__SERVER_IP__/${SERVER_IP}/g" "${PROJECT_DIR}/scripts/add-domain.sh"
  sed -i "s/__MAIL_HOST__/mail.${DOMAIN}/g" "${PROJECT_DIR}/scripts/add-domain.sh"
  sed -i "s/__PRIMARY_DOMAIN__/${DOMAIN}/g" "${PROJECT_DIR}/scripts/add-domain.sh"

  # Replace placeholders in router.go CORS
  sed -i "s|http://localhost:3000|http://${SERVER_IP}, http://${SERVER_IP}:3000, http://localhost:3000|g" "${PROJECT_DIR}/receiver/api/router.go"

  log "All placeholders replaced"

  # --- Nginx ---
  cp "${PROJECT_DIR}/config/nginx/tempmail" /etc/nginx/sites-available/tempmail
  sed -i "s/server_name .*/server_name ${SERVER_IP};/" /etc/nginx/sites-available/tempmail
  ln -sf /etc/nginx/sites-available/tempmail /etc/nginx/sites-enabled/tempmail
  rm -f /etc/nginx/sites-enabled/default
  nginx -t 2>/dev/null
  log "Nginx configured"

  # --- Systemd ---
  sed "s|/opt/mail-platform|${PROJECT_DIR}|g; s|/opt/TempMailByJhopanStore|${PROJECT_DIR}|g" \
    "${PROJECT_DIR}/config/systemd/tempmail-receiver.service" > /etc/systemd/system/tempmail-receiver.service
  sed "s|/opt/mail-platform|${PROJECT_DIR}|g; s|/opt/TempMailByJhopanStore|${PROJECT_DIR}|g" \
    "${PROJECT_DIR}/config/systemd/tempmail-frontend.service" > /etc/systemd/system/tempmail-frontend.service

  cat > /etc/systemd/system/maddy.service << EOF
[Unit]
Description=Maddy Mail Server
After=network.target

[Service]
Type=simple
User=maddy
Group=maddy
ExecStart=/usr/local/bin/maddy -config /etc/maddy/maddy.conf
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  log "Systemd services created"
}

# ------------------------------------------
# STEP 8: Firewall
# ------------------------------------------
setup_firewall() {
  info "Configuring firewall..."
  ufw allow 22/tcp > /dev/null 2>&1
  ufw allow 25/tcp > /dev/null 2>&1
  ufw allow 80/tcp > /dev/null 2>&1
  ufw allow 443/tcp > /dev/null 2>&1
  echo "y" | ufw enable > /dev/null 2>&1
  log "Firewall configured (22, 25, 80, 443)"
}

# ------------------------------------------
# STEP 9: Python bcrypt
# ------------------------------------------
install_python_deps() {
  info "Installing Python dependencies..."
  pip3 install bcrypt --break-system-packages > /dev/null 2>&1 || pip3 install bcrypt > /dev/null 2>&1
  log "Python bcrypt installed"
}

# ============================================
# MAIN
# ============================================
banner
check_root

echo -e "${BOLD}Langkah instalasi:${NC}"
echo "  1. System dependencies"
echo "  2. PostgreSQL + Database"
echo "  3. Go 1.25"
echo "  4. Node.js 20"
echo "  5. Maddy Mail Server"
echo "  6. Nginx"
echo "  7. Python bcrypt"
echo "  8. Configure services"
echo "  9. Firewall"
echo ""
read -p "Lanjutkan instalasi? (y/n): " CONFIRM
[ "$CONFIRM" != "y" ] && exit 0

echo ""
install_system_deps
install_postgresql
setup_database
install_go
install_nodejs
install_maddy
install_nginx
install_python_deps
configure_services
setup_cloudflare
setup_firewall

echo ""
echo -e "${GREEN}${BOLD}============================================${NC}"
echo -e "${GREEN}${BOLD}  Setup Complete!${NC}"
echo -e "${GREEN}${BOLD}============================================${NC}"
echo ""
echo -e "  Jalankan: ${CYAN}sudo bash scripts/start.sh${NC}"
echo -e "  Atau:     ${CYAN}sudo bash scripts/menu.sh${NC}"
echo ""
echo -e "  Admin login:"
echo -e "    Email:    ${YELLOW}your@email.com${NC}"
echo -e "    Password: ${YELLOW}your-password${NC}"
echo ""