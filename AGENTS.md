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

- Build Go binaries locally on Windows. Never build on VPS/Debian.
- Use `CGO_ENABLED=0`; SQLite uses `modernc.org/sqlite`.
- Never commit secrets, tokens, `.env`, databases, backups, cloudflared credentials, admin credentials, or built binaries.
- Do not silently enable email copies or forwarding.
- Cloudflare profile tokens/account IDs must not cross profiles.
- Domain removal from menu must not delete Cloudflare DNS/zone/routing unless user explicitly asks.
- Do not claim Windows/macOS persistent-service support until adapters are written and tested.
- Do not expose `EMAIL_API_KEY` in any public API response (e.g. `/api/config`).
- Web UI must require JWT login. Do not add guest mode or API-key-based web access.

## Commands

```bash
cd receiver
go test ./...
go vet ./...

# Local run (JWT_SECRET, EMAIL_API_KEY, and DOMAINS in .env are mandatory)
cp ../.env.example .env   # first time only
go run .                  # UI at http://127.0.0.1:8080/app/

# Windows local cross-builds
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o tempmail-linux-arm64 .
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tempmail-linux-amd64 .

# Validate non-Go code
bash -n deploy/linux/install.sh
bash -n deploy/linux/tempmailctl
bash -n deploy/linux/setup.sh
node --check cloudflare-worker/src/index.js

# Worker deploy (from cloudflare-worker/)
npm run deploy            # wrangler deploy
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
- Embedded frontend is vanilla HTML/JS, not React/Next.js.
- Cache-bust JS changes with `?v=N` in `index.html` script tag.
- Update PRD/architecture/runbook when API, deployment, menu, Cloudflare, or security behavior changes.

## Pitfalls

- `frontend/` (Next.js), old `scripts/`, `config/` (maddy/nginx/systemd), `plan.md` are pre-rewrite legacy. Live UI is `receiver/web/static` via `go:embed`. Do not build or modify legacy dirs unless explicitly asked.
- `deploy/armbian/` is legacy; use `deploy/linux/` for all Linux deployments.
- No `*_test.go` files exist yet; `go test ./...` passes trivially. New tests go under `receiver/` packages per `docs/TDD.md`.
- Built binaries (`receiver/tempmail.exe`, `tempmail-linux*`) sit in the repo dir but are gitignored — never commit them.
- SMTP test/local mode listens on `:2525` (not 25) per `docs/ARCHITECTURE.md`.
- `receiver/handler` (auth) and `receiver/handlers` (inbox/email) are separate packages; `receiver/events` exists only to break the `api ↔ handlers` import cycle.
- Cloudflare Tunnel buffers SSE responses. Config must include `disableChunkedEncoding: true` in `config.yml`.
- `wrangler whoami` always exits 0 (even when not authenticated). Must grep output for "You are logged in".
- `wrangler` in non-interactive mode: always use `CI=true` and `</dev/null` to prevent hangs.
- Wrangler OAuth in headless mode: use `--browser=false --device` flag (device auth flow, no localhost callback).
- Cloudflare Email Routing enable fails if non-CF MX records exist. Delete them from CF dashboard first.
- `wrangler email routing rules update catch-all` does not support `worker` action type. Use Cloudflare API directly for catch-all with worker action.

## Cloudflare deployment facts

```text
Worker name:    tempmailbyjhopanstore (fixed)
Tunnel name:    tempmailbyjhopanstore (default, configurable)
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
Config:     /etc/tempmail/tempmail.env (PORT, DOMAINS, JWT_SECRET, EMAIL_API_KEY)
CF state:    /etc/tempmail/cloudflare-state.json
Menu:        menutempmail (launches /usr/local/sbin/tempmailctl)
DB:         /opt/tempmail/data/tempmail.db
Port:       read from PORT in /etc/tempmail/tempmail.env (default 8081)
```

### Deploy binary to target

```bash
# Build on Windows
cd receiver
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tempmail-linux-amd64 .

# Deploy to target (use laptop-debian, fallback to laptop-debian1)
scp tempmail-linux-amd64 laptop-debian:/tmp/tn
ssh laptop-debian 'systemctl stop tempmail && cp /tmp/tn /opt/tempmail/tempmail && chown tempmail:tempmail /opt/tempmail/tempmail && chmod +x /opt/tempmail/tempmail && rm /tmp/tn && systemctl start tempmail'
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
# Flow: deps → wrangler login (device auth) → tunnel login → create tunnel → deploy worker → email routing → save state
```

### SSH targets

```text
Primary:   ssh laptop-debian   (192.168.10.21)
Fallback:  ssh laptop-debian1  (192.168.1.57)
Hostname:  jhopan (same physical machine, two network interfaces)
```

If `laptop-debian` times out, use `laptop-debian1`.

Run the relevant runbook verification before reporting deploy complete.
