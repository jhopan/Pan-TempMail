#!/usr/bin/env bash
# Pan-TempMail installer.
#
# Detects the environment (Linux, Termux, macOS, Windows shell), downloads the Go binary from
# the latest GitHub release, asks for the port, then either installs a systemd service (Linux,
# root) or lays down a portable install with the command to run.
#
# Usage:
#   sudo bash install.sh                 # Linux server, service install
#   bash install.sh --dry-run            # show what would happen, change nothing
#   bash install.sh --port 8081          # skip the port question
#   bash install.sh --version v1.1.0     # pin a release instead of latest
#   bash install.sh --os windows --dry-run
set -euo pipefail

REPO="${PAN_TM_REPO:-jhopan/Pan-TempMail}"
TOKEN="${PAN_TM_TOKEN:-}"
REL_BASE="https://github.com/${REPO}/releases"
APP_NAME="Pan-TempMail"
SITE_NAME="Pan-TempMail"

ROOT="/opt/tempmail"
ENV_DIR="/etc/tempmail"
APP_USER="tempmail"
SERVICE="tempmail.service"
DEFAULT_PORT=8080

PORT=""
PORT_ARG=""
VERSION="latest"
FORCE_OS=""
FORCE_ARCH=""
DRY_RUN=0
NO_SERVICE=0
UPDATE=0
FROM_FILE=""

usage() {
  cat <<'TXT'
Pan-TempMail installer

  --port N          port the app listens on (asks when omitted)
  --version TAG     release to install, for example v1.1.0 (default: latest)
  --update          update an existing server install (binary, scripts, helper, units)
  --from-file PATH  install this binary instead of downloading (implies no tarball fetch)
  --repo OWNER/NAME repository to download from (default jhopan/Pan-TempMail)
  --os NAME         override detection: linux | termux | macos | windows
  --arch NAME       override detection: amd64 | arm64
  --no-service      never install a systemd service, even as root
  --dry-run         print the plan and exit, change nothing
  -h, --help        this text

Update an existing install, one command:

  sudo /opt/tempmail/install.sh --update

Private fork? Set PAN_TM_TOKEN or write a read-only token to /etc/tempmail/github-token.

Linux as root installs /opt/tempmail + systemd units + the root helper.
Termux, macOS, Windows and non-root Linux get a portable install with a run command;
those service adapters are not implemented yet and the installer says so.
TXT
}

log() { printf '%s\n' "$*"; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --port) PORT=${2:-}; PORT_ARG=1; shift 2 ;;
    --version) VERSION=${2:-}; shift 2 ;;
    --os) FORCE_OS=${2:-}; shift 2 ;;
    --arch) FORCE_ARCH=${2:-}; shift 2 ;;
    --update) UPDATE=1; shift ;;
    --from-file) FROM_FILE=${2:-}; shift 2 ;;
    --repo) REPO=${2:-}; shift 2 ;;
    --no-service) NO_SERVICE=1; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "argumen tidak dikenal: $1 (pakai --help)" ;;
  esac
done

# Token opsional: dibutuhkan kalau repo-nya privat (bukan untuk repo publik).
if [ -z "$TOKEN" ] && [ -r /etc/tempmail/github-token ]; then
  TOKEN=$(tr -d '\r\n' < /etc/tempmail/github-token)
fi

# --- deteksi lingkungan -------------------------------------------------------

detect_os() {
  if [ -n "$FORCE_OS" ]; then printf '%s' "$FORCE_OS"; return; fi
  local kernel
  kernel=$(uname -s 2>/dev/null || echo unknown)
  case "$kernel" in
    Linux)
      if [ -n "${PREFIX:-}" ] && printf '%s' "$PREFIX" | tr 'A-Z' 'a-z' | grep -q termux; then
        printf 'termux'
      elif [ -d /data/data/com.termux ]; then
        printf 'termux'
      else
        printf 'linux'
      fi
      ;;
    Darwin) printf 'macos' ;;
    MINGW*|MSYS*|CYGWIN*|Windows_NT*|Windows*) printf 'windows' ;;
    *)
      # Bash di Windows melaporkan Windows_NT; OSTYPE menutup variasi lain.
      case "${OSTYPE:-}" in
        msys*|cygwin*|win32*) printf 'windows' ;;
        darwin*) printf 'macos' ;;
        linux*) printf 'linux' ;;
        *) printf 'unknown' ;;
      esac
      ;;
  esac
}

detect_arch() {
  if [ -n "$FORCE_ARCH" ]; then printf '%s' "$FORCE_ARCH"; return; fi
  local machine
  machine=$(uname -m 2>/dev/null || echo unknown)
  case "$machine" in
    x86_64|amd64) printf 'amd64' ;;
    aarch64|arm64) printf 'arm64' ;;
    armv7l|armv7) printf 'armv7' ;;
    *) printf 'unknown' ;;
  esac
}

OS=$(detect_os)
ARCH=$(detect_arch)

case "$OS" in
  linux|termux|macos|windows) ;;
  *) die "sistem tidak dikenali. Dukungan: Linux, Termux, macOS, Windows (Git Bash/WSL). Pakai --os untuk memaksa." ;;
esac
case "$ARCH" in
  amd64|arm64) ;;
  armv7) die "armv7 tidak disediakan. Pakai arm64, atau build sendiri: cd receiver && GOARCH=arm go build ." ;;
  *) die "arsitektur tidak dikenali: $(uname -m 2>/dev/null || echo '?'). Pakai --arch amd64|arm64." ;;
esac

# nama aset rilis: linux/termux memakai build linux, macOS memakai build darwin
case "$OS" in
  linux|termux) ASSET_OS="linux" ;;
  macos) ASSET_OS="darwin" ;;
  windows) ASSET_OS="windows" ;;
esac
ASSET_EXT=""
[ "$ASSET_OS" = "windows" ] && ASSET_EXT=".exe"
ASSET="pan-tempmail-${ASSET_OS}-${ARCH}${ASSET_EXT}"

# Kombinasi yang benar-benar diterbitkan rilis; selain ini lebih baik berhenti sekarang.
case "${ASSET_OS}-${ARCH}" in
  linux-amd64|linux-arm64|darwin-amd64|darwin-arm64|windows-amd64) ;;
  windows-*) die "rilis belum membangun Windows ${ARCH}; yang tersedia: windows amd64" ;;
  *) die "kombinasi ${ASSET_OS}/${ARCH} tidak diterbitkan" ;;
esac

if [ "$VERSION" = "latest" ]; then
  DOWNLOAD_BASE="${REL_BASE}/latest/download"
else
  DOWNLOAD_BASE="${REL_BASE}/download/${VERSION}"
fi
BINARY_URL="${DOWNLOAD_BASE}/${ASSET}"
CHECKSUM_URL="${DOWNLOAD_BASE}/pan-tempmail-checksums.txt"
TARBALL_URL="${DOWNLOAD_BASE}/pan-tempmail.tar.gz"

# --- porta --------------------------------------------------------------------

valid_port() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
  esac
  [ "$1" -ge 1 ] && [ "$1" -le 65535 ]
}

ask_port() {
  local answer
  if [ -n "$PORT" ]; then
    valid_port "$PORT" || die "port tidak valid: $PORT (1-65535)"
    return
  fi
  if [ -t 0 ]; then
    printf 'Port aplikasi [%s]: ' "$DEFAULT_PORT"
    read -r answer || answer=""
    answer=${answer:-$DEFAULT_PORT}
  else
    answer=$DEFAULT_PORT
  fi
  valid_port "$answer" || die "port tidak valid: $answer (1-65535)"
  PORT=$answer
}

# --- unduhan ------------------------------------------------------------------

# Unduhan dengan batas waktu pendek: koneksi yang macet harus gagal cepat, bukan
# menggantung berjam-jam. github.com pada beberapa jaringan (termasuk ISP rumahan) bisa
# menolak koneksi sementara, jadi percobaan diulang beberapa kali.
CURL_NET=(--connect-timeout 15 --max-time 240 --retry 2 --retry-delay 2 --retry-connrefused)

fetch() {
  local url=$1 out=$2
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "${CURL_NET[@]}" "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -q --timeout=20 --tries=3 -O "$out" "$url"
  else
    die "butuh curl atau wget untuk mengunduh biner"
  fi
}

# fetch_via_api mengambil satu aset lewat API GitHub. Dipakai sebagai jalur cadangan
# (dan wajib untuk repo privat); api.github.com sering tetap terjangkau saat
# github.com menolak koneksi.
fetch_via_api() {
  local name=$1 out=$2 asset_id api
  [ -n "${PYTHON:-}" ] || PYTHON=$(command -v python3 || true)
  [ -n "$PYTHON" ] || return 1
  if [ "$VERSION" = "latest" ]; then
    api="https://api.github.com/repos/${REPO}/releases/latest"
  else
    api="https://api.github.com/repos/${REPO}/releases/tags/${VERSION}"
  fi
  local auth=()
  [ -n "$TOKEN" ] && auth=(-H "Authorization: Bearer ${TOKEN}")
  asset_id=$(curl -fsSL --connect-timeout 15 --max-time 60 "${auth[@]}" -H "Accept: application/vnd.github+json" "$api" \
    | "$PYTHON" -c 'import json,sys; name=sys.argv[1]; data=json.load(sys.stdin); print(next((a["id"] for a in data.get("assets", []) if a["name"] == name), ""))' "$name" 2>/dev/null || true)
  [ -n "$asset_id" ] || return 1
  curl -fsSL --connect-timeout 15 --max-time 240 --retry 2 --retry-delay 2 \
    "${auth[@]}" -H "Accept: application/octet-stream" \
    "https://api.github.com/repos/${REPO}/releases/assets/${asset_id}" -o "$out"
}

# fetch_asset mengunduh satu aset rilis. Repo publik memakai URL rilis langsung; kalau token
# tersedia (repo privat), lewat API aset dengan header Authorization.
fetch_asset() {
  local name=$1 out=$2
  if [ -n "$TOKEN" ]; then
    fetch_via_api "$name" "$out" || die "gagal mengunduh ${name} lewat API (repo privat: cek token di /etc/tempmail/github-token)"
    return
  fi
  if fetch "${DOWNLOAD_BASE}/${name}" "$out"; then
    return
  fi
  log "  koneksi ke github.com gagal, mencoba lewat api.github.com ..."
  fetch_via_api "$name" "$out" || die "gagal mengunduh ${name} dari rilis ${VERSION} (${REPO}); coba lagi atau pakai --from-file"
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | cut -d' ' -f1
  else printf ''; fi
}

verify_checksum() {
  local file=$1 sums=$2 want got
  [ -s "$sums" ] || return 0
  want=$(awk -v name="$ASSET" '$2 == name { print $1 }' "$sums" | head -n1)
  [ -n "$want" ] || { log "  checksum untuk $ASSET tidak ada di daftar, verifikasi dilewati"; return 0; }
  got=$(sha256_of "$file")
  if [ -z "$got" ]; then log "  sha256sum tidak tersedia, verifikasi dilewati"; return 0; fi
  [ "$want" = "$got" ] || die "checksum tidak cocok untuk $ASSET (unduhan rusak atau dipalsukan)"
  log "  checksum cocok"
}

is_root() { [ "$(id -u 2>/dev/null || echo 1)" = "0" ]; }

has_systemd() {
  [ -d /run/systemd/system ] && command -v systemctl >/dev/null 2>&1
}

# Mode update: port yang sudah dipakai env lebih penting daripada default/pertanyaan.
if [ "$UPDATE" = "1" ] && [ -z "$PORT_ARG" ] && [ -r "${ENV_DIR}/tempmail.env" ]; then
  EXISTING_PORT=$(awk -F= '/^PORT=/{print $2; exit}' "${ENV_DIR}/tempmail.env" | tr -d '\r')
  if valid_port "$EXISTING_PORT"; then
    PORT=$EXISTING_PORT
    log "port dari env yang ada: ${PORT}"
  fi
fi

PORT=$(printf '%s' "$PORT")
ask_port

# mode: server (systemd) atau portable
MODE="portable"
if [ "$OS" = "linux" ] && is_root && has_systemd && [ "$NO_SERVICE" = "0" ]; then
  MODE="server"
fi

log ""
log "${APP_NAME} installer"
log "  sistem     : ${OS} (${ARCH})"
log "  mode       : ${MODE}$([ "$MODE" = "server" ] && printf ' (systemd, root)' || printf ' (tanpa service adapter)')"
log "  versi      : ${VERSION}"
log "  port       : ${PORT}"
log "  biner      : ${BINARY_URL}"
[ "$MODE" = "server" ] && log "  skrip      : ${TARBALL_URL}"
log ""

if [ "$DRY_RUN" = "1" ]; then
  if [ "$MODE" = "server" ]; then
    log "dry-run: akan memasang"
    log "  biner        -> ${ROOT}/tempmail"
    log "  user/dirs    -> ${APP_USER}, ${ROOT}/data, ${ENV_DIR}"
    log "  skrip+helper -> /usr/local/sbin/tempmailctl, /usr/local/bin/menutempmail, /usr/local/sbin/tempmail-root"
    log "  unit         -> ${SERVICE}, tempmail-backup.timer, tempmail-root.path, tempmail-root.timer"
    log "  env          -> ${ENV_DIR}/tempmail.env (PORT=${PORT}, rahasia dibuat aplikasi saat start)"
    log "  cloudflared  -> dipasang bila belum ada"
  else
    log "dry-run: akan memasang biner ${ASSET} ke lokasi pengguna dan mencetak perintah menjalankan."
    case "$OS" in
      termux) log "  catatan: Termux tidak punya systemd; jalankan di foreground (atau lewat termux-services)" ;;
      macos) log "  catatan: adapter launchd belum ada; jalankan di foreground" ;;
      windows) log "  catatan: Windows Service belum ada; jalankan lewat PowerShell/CMD" ;;
      linux) log "  catatan: bukan root atau tanpa systemd; jalankan di foreground" ;;
    esac
  fi
  exit 0
fi

TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/pan-tempmail.XXXXXX")
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

if [ -n "$FROM_FILE" ]; then
  [ -f "$FROM_FILE" ] || die "berkas tidak ditemukan: $FROM_FILE"
  log "memakai biner lokal ${FROM_FILE} (tanpa unduh)"
  cp "$FROM_FILE" "${TMP_DIR}/${ASSET}"
else
  log "mengunduh biner ${ASSET} ..."
  fetch_asset "$ASSET" "${TMP_DIR}/${ASSET}"
fi
log "mengunduh daftar checksum ..."
if ! fetch "$CHECKSUM_URL" "${TMP_DIR}/checksums.txt" 2>/dev/null; then
  log "  daftar checksum tidak tersedia, verifikasi dilewati"
  : > "${TMP_DIR}/checksums.txt"
fi
verify_checksum "${TMP_DIR}/${ASSET}" "${TMP_DIR}/checksums.txt"

if [ "$MODE" = "portable" ]; then
  case "$OS" in
    windows)
      DEST_DIR="${LOCALAPPDATA:-$HOME}/Pan-TempMail"
      BIN_PATH="${DEST_DIR}/pan-tempmail.exe"
      ENV_FILE="${DEST_DIR}/tempmail.env"
      DATA_DIR="${DEST_DIR}/data"
      RUN_CMD="\"${BIN_PATH}\""
      ;;
    termux)
      DEST_DIR="${HOME}/pan-tempmail"
      BIN_PATH="${DEST_DIR}/pan-tempmail"
      ENV_FILE="${DEST_DIR}/tempmail.env"
      DATA_DIR="${DEST_DIR}/data"
      RUN_CMD="${BIN_PATH}"
      ;;
    *)
      DEST_DIR="${HOME}/.local/bin"
      BIN_PATH="${DEST_DIR}/pan-tempmail"
      ENV_FILE="${HOME}/.config/pan-tempmail/tempmail.env"
      DATA_DIR="${HOME}/.local/share/pan-tempmail/data"
      RUN_CMD="${BIN_PATH}"
      ;;
  esac
  mkdir -p "$DEST_DIR" "$DATA_DIR" "$(dirname "$ENV_FILE")"
  install -m 0755 "${TMP_DIR}/${ASSET}" "$BIN_PATH" 2>/dev/null || { cp "${TMP_DIR}/${ASSET}" "$BIN_PATH"; chmod 755 "$BIN_PATH"; }

  if [ ! -f "$ENV_FILE" ]; then
    {
      printf '# Pan-TempMail (portable install). Password admin tidak disimpan di sini.\n'
      printf 'PORT=%s\n' "$PORT"
      printf 'SITE_NAME=%s\n' "$SITE_NAME"
      printf 'DB_PATH=%s/tempmail.db\n' "$DATA_DIR"
      printf 'ATTACHMENTS_DIR=%s/attachments\n' "$DATA_DIR"
      printf 'GIN_MODE=release\n'
    } > "$ENV_FILE"
    chmod 600 "$ENV_FILE"
    log "env baru: ${ENV_FILE}"
  else
    log "env sudah ada, dibiarkan: ${ENV_FILE}"
  fi

  log ""
  log "selesai. Biner: ${BIN_PATH}"
  log "jalankan:"
  log "  set -a; . \"${ENV_FILE}\"; set +a"
  log "  ${RUN_CMD}"
  log "lalu buka http://127.0.0.1:${PORT}/app/login.html untuk membuat admin pertama."
  case "$OS" in
    termux) log "Termux: jalankan di foreground, atau pakai termux-services bila ingin otomatis." ;;
    macos) log "macOS: adapter launchd belum dibuat, jadi belum ada install sebagai service." ;;
    windows) log "Windows: Windows Service belum dibuat; jalankan dari terminal." ;;
  esac
  log "akses dari HP lain di LAN: http://<ip-lan>:${PORT}/app/"
  exit 0
fi
# --- mode server (Linux, root, systemd) ---------------------------------------

[ "$(id -u)" = 0 ] || die "mode server butuh root"

OLD_VERSION="?"
if [ "$UPDATE" = "1" ]; then
  [ -x "$ROOT/tempmail" ] || die "belum ada instalasi di ${ROOT}; jalankan tanpa --update"
  OLD_VERSION=$("$ROOT/tempmail" --version 2>/dev/null || echo "?")
  log "update dari versi ${OLD_VERSION} ..."
fi

if [ "$UPDATE" = "0" ] || ! command -v sqlite3 >/dev/null 2>&1 || ! command -v python3 >/dev/null 2>&1; then
  if ! command -v apt-get >/dev/null 2>&1; then
    die "distro ini belum didukung otomatis (butuh apt). Pasang manual: sqlite3, ca-certificates, curl, python3, openssl"
  fi
  log "memasang paket sistem ..."
  apt-get update -qq
  apt-get install -y -qq sqlite3 ca-certificates curl python3 openssl >/dev/null
fi

log "menyiapkan user dan direktori ..."
id -u "$APP_USER" >/dev/null 2>&1 || useradd --system --home "$ROOT" --shell /usr/sbin/nologin "$APP_USER"
install -d -o "$APP_USER" -g "$APP_USER" -m 0750 "$ROOT/data"
install -d -o "$APP_USER" -g "$APP_USER" -m 0750 "$ROOT/data/attachments" "$ROOT/backups" "$ROOT/worker" "$ENV_DIR"

SOURCE_DIR=""
if [ -z "$FROM_FILE" ]; then
  log "mengunduh skrip pendukung (tarball) ..."
  fetch_asset "pan-tempmail.tar.gz" "${TMP_DIR}/pan-tempmail.tar.gz"
  mkdir -p "${TMP_DIR}/src"
  tar -xzf "${TMP_DIR}/pan-tempmail.tar.gz" -C "${TMP_DIR}/src"
  SRC_DIR="${TMP_DIR}/src"
  [ -d "${SRC_DIR}/deploy/linux" ] || die "tarball rilis tidak memuat deploy/linux"
  SOURCE_DIR="${SRC_DIR}/deploy/linux"
  WORKER_DIR="${SRC_DIR}/cloudflare-worker"
else
  log "melewati skrip dan helper (mode --from-file hanya mengganti biner)"
fi

# Biner lama disimpan sebelum ditimpa, supaya update yang gagal bisa dibalik.
if [ -f "$ROOT/tempmail" ]; then
  install -d -o root -g root -m 0750 "$ROOT/backups/binaries"
  cp -p "$ROOT/tempmail" "$ROOT/tempmail.prev"
  cp -p "$ROOT/tempmail" "$ROOT/backups/binaries/tempmail-$(date +%Y%m%d-%H%M%S)"
fi
install -o root -g root -m 0755 "${TMP_DIR}/${ASSET}" "$ROOT/tempmail"

if [ -n "$SOURCE_DIR" ]; then
  # Saat update, installer yang sedang berjalan TIDAK ditimpa: salinan dari rilis hanya
  # disimpan sebagai .release, supaya perbaikan pada installer yang aktif tidak dibatalkan
  # oleh tarball yang lebih lama.
  if [ "$UPDATE" = "1" ] && [ -f "$ROOT/install.sh" ]; then
    install -o root -g root -m 0644 "$SOURCE_DIR/install.sh" "$ROOT/install.sh.release"
    log "installer tetap dipakai: ${ROOT}/install.sh (salinan rilis: ${ROOT}/install.sh.release)"
  else
    install -o root -g root -m 0755 "$SOURCE_DIR/install.sh" "$ROOT/install.sh"
  fi
  install -o root -g root -m 0755 "$SOURCE_DIR/tempmailctl" /usr/local/sbin/tempmailctl
  install -o root -g root -m 0755 "$SOURCE_DIR/menutempmail" /usr/local/bin/menutempmail
  install -o root -g root -m 0755 "$SOURCE_DIR/setup.sh" "$ROOT/setup.sh"
  install -o root -g root -m 0644 "$SOURCE_DIR/tempmail.service" /etc/systemd/system/tempmail.service
  install -o root -g root -m 0644 "$SOURCE_DIR/tempmail-backup.service" /etc/systemd/system/tempmail-backup.service
  install -o root -g root -m 0644 "$SOURCE_DIR/tempmail-backup.timer" /etc/systemd/system/tempmail-backup.timer
  install -o root -g root -m 0755 "$SOURCE_DIR/tempmail-root" /usr/local/sbin/tempmail-root
  install -o root -g root -m 0644 "$SOURCE_DIR/tempmail-root.service" /etc/systemd/system/tempmail-root.service
  install -o root -g root -m 0644 "$SOURCE_DIR/tempmail-root.path" /etc/systemd/system/tempmail-root.path
  install -o root -g root -m 0644 "$SOURCE_DIR/tempmail-root.timer" /etc/systemd/system/tempmail-root.timer
  install -d -o "$APP_USER" -g "$APP_USER" -m 0750 "$ROOT/data/helper" "$ROOT/data/helper/in" "$ROOT/data/helper/out"
  install -d -o root -g root -m 0700 "$ROOT/data/helper/state"
  if [ -d "$WORKER_DIR" ]; then
    cp -a "$WORKER_DIR/." "$ROOT/worker/"
    chown -R root:root "$ROOT/worker"
  fi
fi

if [ ! -f "$ENV_DIR/tempmail.env" ]; then
  {
    printf 'DB_DRIVER=sqlite\n'
    printf 'DB_PATH=%s/data/tempmail.db\n' "$ROOT"
    printf 'PORT=%s\n' "$PORT"
    printf 'SMTP_HOST=127.0.0.1\n'
    printf 'SMTP_PORT=2525\n'
    printf 'DOMAINS=local.invalid\n'
    printf 'SITE_NAME=%s\n' "$SITE_NAME"
    printf 'CORS_ORIGINS=\n'
    printf 'CLEANUP_INTERVAL=1h\n'
    printf 'MESSAGE_RETENTION=24h\n'
    printf 'ATTACHMENTS_DIR=%s/data/attachments\n' "$ROOT"
    printf 'GIN_MODE=release\n'
    printf 'CLOUDFLARE_API_BASE=https://api.cloudflare.com/client/v4\n'
    printf 'CF_WORKER_NAME=tempmailbyjhopanstore\n'
    printf 'CF_WORKER_COMPAT_DATE=2024-01-01\n'
    printf 'CF_TUNNEL_NAME=tempmailbyjhopanstore\n'
    printf '# JWT_SECRET dan EMAIL_API_KEY sengaja kosong: aplikasi membuatnya sekali\n'
    printf '# secara acak ke %s/data/secrets.env (0600) saat start pertama.\n' "$ROOT"
  } > "$ENV_DIR/tempmail.env"
  chown root:"$APP_USER" "$ENV_DIR/tempmail.env"
  chmod 640 "$ENV_DIR/tempmail.env"
  log "env baru: ${ENV_DIR}/tempmail.env (PORT=${PORT})"
else
  log "env sudah ada, dibiarkan: ${ENV_DIR}/tempmail.env"
fi

if ! command -v cloudflared >/dev/null 2>&1; then
  case "$ARCH" in
    amd64) CF_PKG="cloudflared-linux-amd64.deb" ;;
    arm64) CF_PKG="cloudflared-linux-arm64.deb" ;;
  esac
  log "memasang cloudflared (${CF_PKG}) ..."
  if fetch "https://github.com/cloudflare/cloudflared/releases/latest/download/${CF_PKG}" "${TMP_DIR}/cloudflared.deb"; then
    dpkg -i "${TMP_DIR}/cloudflared.deb" >/dev/null || log "  dpkg gagal; pasang nanti dari panel Server"
  else
    log "  unduhan cloudflared gagal; pasang nanti dari panel Server"
  fi
fi

systemctl daemon-reload
if [ "$UPDATE" = "1" ]; then
  systemctl restart "$SERVICE"
else
  systemctl enable --now "$SERVICE" tempmail-backup.timer tempmail-root.path tempmail-root.timer
fi

healthy=0
for _ in $(seq 1 20); do
  if curl -fsS "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then healthy=1; break; fi
  sleep 1
done

NEW_VERSION=$("$ROOT/tempmail" --version 2>/dev/null || echo "?")
# Berkas VERSION memuat versi yang benar-benar terpasang, bukan kata "latest".
printf '%s\n' "$NEW_VERSION" > "$ROOT/VERSION"
chmod 0644 "$ROOT/VERSION"

if [ "$healthy" = "0" ] && [ -f "$ROOT/tempmail.prev" ]; then
  log "health check gagal; mengembalikan biner sebelumnya ..."
  cp -p "$ROOT/tempmail.prev" "$ROOT/tempmail"
  systemctl restart "$SERVICE"
  for _ in $(seq 1 15); do
    if curl -fsS "http://127.0.0.1:${PORT}/health" >/dev/null 2>&1; then healthy=2; break; fi
    sleep 1
  done
  if [ "$healthy" = "2" ]; then
    die "update gagal (biner ${NEW_VERSION} tidak sehat); sudah dikembalikan ke versi ${OLD_VERSION} dan service jalan lagi"
  fi
  die "update gagal dan rollback juga tidak sehat; cek: journalctl -u ${SERVICE} -n 50 --no-pager"
fi

if [ "$healthy" = "1" ]; then
  log "health check: OK"
else
  log "health check belum menjawab; cek: systemctl status ${SERVICE} --no-pager"
fi

log ""
if [ "$UPDATE" = "1" ]; then
  log "selesai: ${OLD_VERSION} -> ${NEW_VERSION} (port ${PORT})"
  log "riwayat biner lama: ${ROOT}/backups/binaries/"
else
  log "selesai: versi ${NEW_VERSION} terpasang (port ${PORT})"
fi
log "langkah berikutnya di browser:"
log "  1. http://<ip-lan>:${PORT}/app/login.html  -> buat admin pertama (kalau belum ada)"
log "  2. http://<ip-lan>:${PORT}/admin           -> Setup: API token Cloudflare, tunnel, domain"
log "update berikutnya: sudo ${ROOT}/install.sh --update"
log "menu server: sudo menutempmail   (opsi 3 untuk menempel API token profil)"
