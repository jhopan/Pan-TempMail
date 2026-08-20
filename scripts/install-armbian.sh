#!/usr/bin/env bash
# Bootstrap TempMail from a GitHub Release. Run as root.
# Requires a published release asset named tempmail-linux-arm64 and source archive.
set -euo pipefail

REPO=${TEMPMAIL_REPO:-jhopan/TempMailByJhopanstore}
VERSION=${TEMPMAIL_VERSION:-latest}
ARCH=$(dpkg --print-architecture)

case "$ARCH" in
  arm64) ASSET=tempmail-linux-arm64 ;;
  amd64) ASSET=tempmail-linux-amd64 ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

[ "$(id -u)" = 0 ] || { echo "Run: sudo bash"; exit 1; }
command -v curl >/dev/null || { apt-get update -qq; apt-get install -y -qq curl; }

if [ "$VERSION" = latest ]; then
  API_URL="https://api.github.com/repos/$REPO/releases/latest"
else
  API_URL="https://api.github.com/repos/$REPO/releases/tags/$VERSION"
fi

# GitHub token is optional for public releases. For private releases, export GITHUB_TOKEN first.
AUTH=()
[ -n "${GITHUB_TOKEN:-}" ] && AUTH=(-H "Authorization: Bearer $GITHUB_TOKEN")

json=$(curl -fsSL "${AUTH[@]}" "$API_URL") || {
  echo "No accessible GitHub Release. Publish release assets first, or use local deploy package."
  exit 1
}
tag=$(printf '%s' "$json" | python3 -c 'import json,sys; print(json.load(sys.stdin)["tag_name"])')
url=$(printf '%s' "$json" | python3 - "$ASSET" <<'PY'
import json, sys
asset = sys.argv[1]
for item in json.load(sys.stdin)['assets']:
    if item['name'] == asset:
        print(item['browser_download_url'])
        break
PY
)
[ -n "$url" ] || { echo "Release $tag lacks asset: $ASSET"; exit 1; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl -fsSL "${AUTH[@]}" -o "$work/tempmail" "$url"
chmod 755 "$work/tempmail"

# Download source archive for installer/menu/Worker files.
archive="https://github.com/$REPO/archive/refs/tags/$tag.tar.gz"
curl -fsSL "${AUTH[@]}" -o "$work/source.tgz" "$archive"
tar -xzf "$work/source.tgz" -C "$work"
src=$(find "$work" -maxdepth 1 -type d -name 'TempMailByJhopanstore-*' | head -1)
[ -n "$src" ] || { echo "Cannot unpack source archive."; exit 1; }

cp "$work/tempmail" "$src/receiver/$ASSET"
chmod 755 "$src/receiver/$ASSET"
cd "$src/deploy/armbian"
./install.sh "../../receiver/$ASSET"

echo "Installed TempMail release $tag"
echo "Next: menutempmail"
