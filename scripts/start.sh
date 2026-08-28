#!/bin/bash
set -e

# ============================================
# TempMailByJhopanStore - Start Script
# Build & start all services
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
  echo -e "${CYAN}${BOLD}  TempMailByJhopanStore - Build & Start${NC}"
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
# Build Backend (Go)
# ------------------------------------------
build_backend() {
  info "Building Go backend..."
  export PATH=$PATH:/usr/local/go/bin
  cd "${PROJECT_DIR}/receiver"
  /usr/local/go/bin/go mod tidy 2>/dev/null
  /usr/local/go/bin/go build -o receiver-final .
  chmod +x receiver-final
  log "Backend built: receiver/receiver-final"
}

# ------------------------------------------
# Build Frontend (Next.js)
# ------------------------------------------
build_frontend() {
  info "Building Next.js frontend..."
  cd "${PROJECT_DIR}/frontend"
  npm install --production=false 2>/dev/null
  npm run build 2>/dev/null
  log "Frontend built successfully"
}

# ------------------------------------------
# Start Services
# ------------------------------------------
start_services() {
  info "Starting services..."

  # Stop existing services
  systemctl stop tempmail-frontend 2>/dev/null || true
  systemctl stop tempmail-receiver 2>/dev/null || true
  systemctl stop maddy 2>/dev/null || true

  # Start in correct order
  systemctl start postgresql
  log "PostgreSQL started"

  systemctl start maddy
  log "Maddy mail server started (SMTP :25)"

  systemctl start tempmail-receiver
  sleep 2
  if systemctl is-active --quiet tempmail-receiver; then
    log "Go backend started (API :8080, LMTP :8024)"
  else
    err "Backend failed to start! Check: journalctl -u tempmail-receiver"
  fi

  systemctl start tempmail-frontend
  sleep 3
  if systemctl is-active --quiet tempmail-frontend; then
    log "Next.js frontend started (Port :3000)"
  else
    err "Frontend failed to start! Check: journalctl -u tempmail-frontend"
  fi

  systemctl start nginx
  log "Nginx started (Port :80)"
}

# ------------------------------------------
# Enable auto-start
# ------------------------------------------
enable_services() {
  systemctl enable tempmail-receiver > /dev/null 2>&1
  systemctl enable tempmail-frontend > /dev/null 2>&1
  systemctl enable maddy > /dev/null 2>&1
  systemctl enable nginx > /dev/null 2>&1
  systemctl enable postgresql > /dev/null 2>&1
  log "All services enabled for auto-start"
}

# ------------------------------------------
# Status Check
# ------------------------------------------
show_status() {
  echo ""
  echo -e "${BOLD}Service Status:${NC}"
  echo "  ─────────────────────────────────"

  for svc in postgresql maddy tempmail-receiver tempmail-frontend nginx; do
    local status
    if systemctl is-active --quiet "$svc"; then
      status="${GREEN}● running${NC}"
    else
      status="${RED}● stopped${NC}"
    fi
    printf "  %-25s %b\n" "$svc" "$status"
  done

  echo ""
  local IP
  IP=$(hostname -I | awk '{print $1}')
  echo -e "  ${BOLD}Access:${NC} http://${IP}"
  echo -e "  ${BOLD}Login:${NC}  your@email.com / your-password"
  echo ""
}

# ============================================
# MAIN
# ============================================
banner
check_root

echo -e "${BOLD}Pilih aksi:${NC}"
echo "  1) Build & Start (full)"
echo "  2) Build Backend only"
echo "  3) Build Frontend only"
echo "  4) Restart all services"
echo "  5) Stop all services"
echo "  6) Status check"
echo ""
read -p "Pilihan [1-6]: " CHOICE

case $CHOICE in
  1)
    build_backend
    build_frontend
    start_services
    enable_services
    show_status
    ;;
  2)
    build_backend
    systemctl restart tempmail-receiver
    log "Backend rebuilt & restarted"
    ;;
  3)
    build_frontend
    systemctl restart tempmail-frontend
    log "Frontend rebuilt & restarted"
    ;;
  4)
    start_services
    show_status
    ;;
  5)
    systemctl stop tempmail-frontend
    systemctl stop tempmail-receiver
    systemctl stop maddy
    systemctl stop nginx
    log "All services stopped"
    ;;
  6)
    show_status
    ;;
  *)
    err "Pilihan tidak valid"
    exit 1
    ;;
esac
