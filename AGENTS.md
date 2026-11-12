# AGENTS.md — TempMail By JhopanStore

## Product

TempMail By JhopanStore is a low-resource temporary email platform.

- One Go binary: HTTP API, embedded vanilla web UI, SMTP test/direct mode, SQLite.
- Default production mail flow: Cloudflare Email Routing → Worker → Cloudflare Tunnel → `POST /api/email/receive`.
- Supports multiple Cloudflare profiles/accounts and multiple domains in one TempMail instance.
- Optional email copies are OFF by default.
- Web UI requires login (JWT). Automation scripts use `X-Email-API-Key` header.
- Naming rules (human-like Indonesian names) used for web UI auto-generate. Automation scripts can send custom `username` in `POST /api/inbox`.

Read in order before changing code:

1. `docs/PRD.md`
2. `docs/ARCHITECTURE.md`
3. `docs/TDD.md`
4. `docs/SDD.md`
5. `docs/RUNBOOK.md`

## Hard rules

- **Binaries are built via GitHub Actions** (`.github/workflows/release.yml`). Tag a release (`git tag v1.0.0 && git push origin v1.0.0`) to trigger build for Linux amd64, Linux arm64, and Windows amd64. Binaries are attached to the GitHub Release.
- **Do not build Go binaries on VPS/Debian.** Build locally for testing only; use GitHub Releases for production deployment.
- Use `CGO_ENABLED=0`; SQLite uses `modernc.org/sqlite`.
- Never commit secrets, tokens, `.env`, databases, backups, cloudflared credentials, admin credentials, or built binaries.
- Do not silently enable email copies or forwarding.
- Cloudflare profile tokens/account IDs must not cross profiles.
- **Cloudflare only through a per-profile API token.** Do not reintroduce wrangler, OAuth device flow, a browser tunnel login, or a Node.js dependency; tunnel creation goes through the app API and the connector is installed from the returned commands.
- Domain removal from menu must not delete Cloudflare DNS/zone/routing unless user explicitly asks.
- Do not claim Windows/macOS persistent-service support until adapters are written and tested.
- Do not expose `EMAIL_API_KEY` in any public API response (e.g. `/api/config`).
- Web UI must require JWT login. Do not add guest mode or API-key-based web access.
- **Open and verify web pages through the browser relay on port `9224`**, not a spawned Chromium: `browser.open({ url, app: { relay: true } })` (or `app: { cdp_url: "http://127.0.0.1:9224" }`). Use it for `/app/`, `/admin`, local test ports, and the production tunnel URL. Fall back to a managed/headless browser only when the relay is unavailable, and say so in the report.

## Build & Release (GitHub Actions)

```bash
# Trigger a release — tag and push
git tag v1.0.0
git push origin v1.0.0

# GitHub Actions automatically:
#   1. Builds tempmail-linux-amd64 (Debian/Ubuntu x86_64)
#   2. Builds tempmail-linux-arm64 (Raspberry Pi 4/5)
#   3. Builds tempmail-windows-amd64.exe (Windows x86_64)
#   4. Creates GitHub Release with binaries attached

# Download from:
#   https://github.com/jhopan/TempMailByJhopanstore/releases
```

### Manual build (local testing only)

```bash
cd receiver
go test ./...
go vet ./...

# Local run (JWT_SECRET, EMAIL_API_KEY, and DOMAINS in .env are mandatory)
cp ../.env.example .env   # first time only
go run .                  # UI at http://127.0.0.1:8080/app/

# Windows local cross-builds (for testing, not for production)
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o tempmail-linux-arm64 .
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tempmail-linux-amd64 .
```

### Validate non-Go code

```bash
bash -n deploy/linux/install.sh
bash -n deploy/linux/tempmailctl
bash -n deploy/linux/setup.sh
node --check cloudflare-worker/src/index.js
```

## Deploy to a new machine (using GitHub Release)

```bash
# 1. On the target machine, download binary from GitHub Releases
wget https://github.com/jhopan/TempMailByJhopanstore/releases/download/v1.0.0/tempmail-linux-amd64 -O /tmp/tempmail
chmod +x /tmp/tempmail

# 2. Clone repo (for scripts + worker)
git clone https://github.com/jhopan/TempMailByJhopanstore.git /tmp/tempmail-repo

# 3. Install
cd /tmp/tempmail-repo
sudo bash deploy/linux/install.sh
cp /tmp/tempmail /opt/tempmail/tempmail
chown tempmail:tempmail /opt/tempmail/tempmail

# 4. Configure
sudo nano /etc/tempmail/tempmail.env  # set PORT, DOMAINS, secrets

# 5. Start service
sudo systemctl start tempmail

# 6. Create admin
curl -X POST http://127.0.0.1:8081/api/setup \
  -H "Content-Type: application/json" \
  -d '{"email":"your@email.com","password":"your-password"}'

# 7. Cloudflare setup
menutempmail → 8 (SETUP GUIDED)
```

## Code constraints

- Run `gofmt` and Go tests on changed Go code.
- DB queries with parameters must use `placeholder()` (`receiver/repository/db_compat.go`) for SQLite/PostgreSQL compatibility.
- API paths use `/api/*`; SSE is `/sse/:email?token=JWT`.
- Auto-generated inbox names come from configurable naming rules (`settings` table, key `naming_rules`; admin API `GET/PUT /api/admin/naming-rules`; logic in `receiver/naming`).
- `POST /api/inbox` accepts optional `username` field. If empty, server generates from naming rules. If provided, server uses it directly.
- Worker HTTP endpoint requires `X-Email-API-Key` whenever configured.
- Automation endpoints accept `X-Email-API-Key` header (no JWT login required for scripts).
- Web UI endpoints require JWT (`Authorization: Bearer <token>`). No API-key fallback for web.
- `/api/config` returns only `domains` and `site_name`. Never return `api_key` or secrets.
- Preserve raw recipient `to`; it identifies the target inbox.
- Embedded frontend is vanilla HTML/JS (no CDN, no Tailwind, pure inline CSS), not React/Next.js.
- Cache-bust JS changes with `?v=N` in `index.html` script tag.
- Update PRD/architecture/runbook when API, deployment, menu, Cloudflare, or security behavior changes.

## Pitfalls

- `frontend/` (Next.js), `scripts/`, `config/` (maddy/nginx/systemd), `deploy/armbian/`, and `plan.md` were pre-rewrite legacy, and have been deleted. Docs `docs/DEPLOYMENT.md` and `docs/ARMBIAN.md` (which described the old flow) were deleted as superseded; use `README.md` and `docs/RUNBOOK.md`.
- Tests live next to the code they cover (`receiver/cloudflare`, `receiver/worker`, `receiver/handlers`, `receiver/repository`, `receiver/web`) and run without network or Linux: `cd receiver && go test ./... && go vet ./...`.
- Built binaries are gitignored — never commit them. Use GitHub Releases instead.
- SMTP test/local mode listens on `:2525` (not 25) per `docs/ARCHITECTURE.md`.
- `receiver/handler` (auth) and `receiver/handlers` (inbox/email) are separate packages; `receiver/events` exists only to break the `api ↔ handlers` import cycle.
- Cloudflare Tunnel buffers SSE responses. Config must include `disableChunkedEncoding: true` in `config.yml`.
- Cloudflare Email Routing enable fails if non-CF MX records exist. The panel's `unlock DNS` removes them; otherwise delete them in the Cloudflare dashboard first.
- Catch-all with a `worker` action only works through the Cloudflare API (`PUT /zones/{zone}/email/routing/rules/catch_all`), which the app already does.

## Cloudflare deployment facts

```text
Worker name:    tempmailbyjhopanstore (fixed, hardcoded)
Tunnel name:    tempmailbyjhopanstore (fixed, hardcoded)
wrangler.toml:  workers_dev=false, preview_urls=false, no [vars] section
Secrets:        TEMPMAIL_SERVER_URL, TEMPMAIL_API_KEY, FORWARD_LIST (via wrangler secret put)
Tunnel config:  /etc/cloudflared/config.yml (disableChunkedEncoding: true for SSE)
CF cert:        /root/.cloudflared/cert.pem
Wrangler token: /root/.wrangler/config/default.toml (device auth)
```

## Linux deployment facts

```text
Binary:     /opt/tempmail/tempmail
Worker dir:  /opt/tempmail/worker/ (wrangler.toml + src/)
Config:      /etc/tempmail/tempmail.env (PORT, DOMAINS, JWT_SECRET, EMAIL_API_KEY, ADMIN_EMAIL, ADMIN_PASS)
CF state:    /etc/tempmail/cloudflare-state.json
Menu:        menutempmail (launches /usr/local/sbin/tempmailctl)
DB:         /opt/tempmail/data/tempmail.db
Port:       read from PORT in /etc/tempmail/tempmail.env (default 8081)
```

### Deploy binary to target (from GitHub Release)

```bash
# Download from GitHub Releases
wget https://github.com/jhopan/TempMailByJhopanstore/releases/download/v1.0.0/tempmail-linux-amd64 -O /tmp/tempmail

# Install
sudo cp /tmp/tempmail /opt/tempmail/tempmail
sudo chown tempmail:tempmail /opt/tempmail/tempmail
sudo chmod +x /opt/tempmail/tempmail
sudo systemctl restart tempmail
```

After deploying binary:

```bash
sudo systemctl restart tempmail
curl -fsS http://127.0.0.1:8081/health
```

### Setup guided (on target host)

```bash
menutempmail
# Choose 8 — SETUP GUIDED
# Flow: deps -> minta domain/web host/Account ID/API token -> simpan profil cf-main -> buat tunnel + jalankan perintah konektor -> tambah domain + routing catch-all (unlock DNS) -> deploy Worker -> verifikasi health
```

### SSH targets

```text
Primary:   ssh laptop-debian   (192.168.x.x)
Fallback:  ssh laptop-debian1  (192.168.x.x)
Hostname:  jhopan (same physical machine, two network interfaces)
```

If `laptop-debian` times out, use `laptop-debian1`.

Run the relevant runbook verification before reporting deploy complete.

<!-- antislop:start -->
## antislop

Untuk pekerjaan UI, teks, aksesibilitas, layout mobile, atau komentar kode: baca `DESIGN.md` (arah
desain) lalu terapkan aturan antislop.

- Core (aktif): `C:\Users\ACER\.agents\skills\antislop\SKILL.md`
- Sub-skill **belum terpasang**. Letakkan folder dari rilis yang cocok dengan core ini di
  `C:\Users\ACER\.agents\skills\antislop\skills\<nama>\` sebelum dipakai:
  `antislop-ui`, `antislop-layoutmobile`, `antislop-human` (butuh `contrast-check.py`),
  `antislop-copywriting`, `antislop-code`.
- Mode kerja: **DURING**, terapkan aturan sambil menulis dan tutup dengan laporan Delivery Gate.
<!-- antislop:end -->
