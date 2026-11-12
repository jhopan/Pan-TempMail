#!/usr/bin/env bash
# Compatibility shim: the one-liner that used to live here now forwards to install.sh,
# which detects the environment, downloads the latest release and asks for the port.
set -euo pipefail

URL="https://raw.githubusercontent.com/jhopan/Pan-TempMail/main/deploy/linux/install.sh"
TMP="$(mktemp "${TMPDIR:-/tmp}/pan-tempmail-install.XXXXXX")"
trap 'rm -f "$TMP"' EXIT

if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$URL" -o "$TMP"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$TMP" "$URL"
else
  echo "butuh curl atau wget" >&2
  exit 1
fi

exec bash "$TMP" "$@"