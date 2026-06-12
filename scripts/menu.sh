#!/bin/bash

# ============================================
# TempMailByJhopanStore - Management Menu
# ============================================

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
[ "$EUID" -ne 0 ] && echo -e "${RED}Harus dijalankan sebagai root (sudo)${NC}" && exit 1

show_status() {
  echo ""
  echo -e "${BOLD}  Service Status:${NC}"
  for svc in postgresql maddy tempmail-receiver tempmail-frontend nginx; do
    if systemctl is-active --quiet "$svc"; then
      printf "    ${GREEN}●${NC} %-25s running\n" "$svc"
    else
      printf "    ${RED}●${NC} %-25s ${RED}stopped${NC}\n" "$svc"
    fi
  done
}

show_logs() {
  echo -e "\n${BOLD}  Pilih service:${NC}"
  echo "    1) tempmail-receiver (Backend)"
  echo "    2) tempmail-frontend (Frontend)"
  echo "    3) maddy (Mail Server)"
  echo "    4) nginx (Web Server)"
  read -p "  Pilihan [1-4]: " svc_choice
  case $svc_choice in
    1) journalctl -u tempmail-receiver -n 50 --no-pager ;;
    2) journalctl -u tempmail-frontend -n 50 --no-pager ;;
    3) journalctl -u maddy -n 50 --no-pager ;;
    4) journalctl -u nginx -n 50 --no-pager ;;
    *) echo "Invalid" ;;
  esac
}

add_user() {
  echo ""
  read -p "  Email user baru: " NEW_EMAIL
  read -p "  Password: " -s NEW_PASS && echo ""
  read -p "  Role (admin/user) [user]: " NEW_ROLE
  [ -z "$NEW_ROLE" ] && NEW_ROLE="user"

  local HASH
  HASH=$(python3 -c "import bcrypt; print(bcrypt.hashpw(b'${NEW_PASS}', bcrypt.gensalt()).decode())")
  sudo -u postgres psql -d tempmail -c \
    "INSERT INTO users (email, password_hash, role) VALUES ('${NEW_EMAIL}', '${HASH}', '${NEW_ROLE}');" 2>/dev/null

  if [ $? -eq 0 ]; then
    echo -e "  ${GREEN}✓ User '${NEW_EMAIL}' (${NEW_ROLE}) berhasil dibuat${NC}"
  else
    echo -e "  ${RED}✗ Gagal membuat user${NC}"
  fi
}

list_users() {
  echo ""
  echo -e "  ${BOLD}Users:${NC}"
  sudo -u postgres psql -d tempmail -c "SELECT id, email, role, created_at FROM users ORDER BY id;" 2>/dev/null
}

delete_user() {
  read -p "  User ID untuk dihapus: " DEL_ID
  sudo -u postgres psql -d tempmail -c "DELETE FROM users WHERE id = ${DEL_ID};" 2>/dev/null
  echo -e "  ${GREEN}✓ User ID ${DEL_ID} dihapus${NC}"
}

reset_password() {
  read -p "  User ID: " RST_ID
  read -p "  Password baru: " -s RST_PASS && echo ""
  local HASH
  HASH=$(python3 -c "import bcrypt; print(bcrypt.hashpw(b'${RST_PASS}', bcrypt.gensalt()).decode())")
  sudo -u postgres psql -d tempmail -c "UPDATE users SET password_hash = '${HASH}', updated_at = NOW() WHERE id = ${RST_ID};" 2>/dev/null
  echo -e "  ${GREEN}✓ Password direset untuk user ID ${RST_ID}${NC}"
}

add_domain() {
  echo ""
  if [ -f "${PROJECT_DIR}/scripts/add-domain.sh" ]; then
    bash "${PROJECT_DIR}/scripts/add-domain.sh"
  else
    echo -e "  ${RED}✗ Script add-domain.sh tidak ditemukan${NC}"
  fi
}

backup_db() {
  local BACKUP_FILE="/tmp/tempmail-backup-$(date +%Y%m%d-%H%M%S).sql"
  sudo -u postgres pg_dump tempmail > "$BACKUP_FILE"
  echo -e "  ${GREEN}✓ Backup saved: ${BACKUP_FILE}${NC}"
  ls -lh "$BACKUP_FILE"
}

# ============================================
# MAIN MENU
# ============================================
while true; do
  clear
  echo ""
  echo -e "${CYAN}${BOLD}  ╔══════════════════════════════════════════╗${NC}"
  echo -e "${CYAN}${BOLD}  ║   TempMailByJhopanStore - Menu Utama     ║${NC}"
  echo -e "${CYAN}${BOLD}  ╚══════════════════════════════════════════╝${NC}"

  show_status

  echo ""
  echo -e "  ${BOLD}─── Services ───${NC}"
  echo "    1) Build & Start All"
  echo "    2) Restart All Services"
  echo "    3) Stop All Services"
  echo "    4) View Logs"
  echo ""
  echo -e "  ${BOLD}─── User Management ───${NC}"
  echo "    5) List Users"
  echo "    6) Add User"
  echo "    7) Delete User"
  echo "    8) Reset Password"
  echo ""
  echo -e "  ${BOLD}─── Domain ───${NC}"
  echo "    9) Add Domain"
  echo ""
  echo -e "  ${BOLD}─── Database ───${NC}"
  echo "   10) Backup Database"
  echo ""
  echo -e "    ${RED}0) Exit${NC}"
  echo ""
  read -p "  Pilihan: " choice

  case $choice in
    1) bash "${PROJECT_DIR}/scripts/start.sh" ;;
    2) systemctl restart postgresql maddy tempmail-receiver tempmail-frontend nginx && echo -e "  ${GREEN}✓ All services restarted${NC}" ;;
    3) systemctl stop tempmail-frontend tempmail-receiver maddy nginx && echo -e "  ${GREEN}✓ All services stopped${NC}" ;;
    4) show_logs ;;
    5) list_users ;;
    6) add_user ;;
    7) delete_user ;;
    8) reset_password ;;
    9) add_domain ;;
    10) backup_db ;;
    0) echo "Bye!"; exit 0 ;;
    *) echo -e "  ${RED}Pilihan tidak valid${NC}" ;;
  esac

  echo ""
  read -p "  Tekan Enter untuk kembali..."
done
