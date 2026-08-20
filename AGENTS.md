# AGENTS.md — TempMail By JhopanStore

## Product

TempMail By JhopanStore is a low-resource temporary email platform.

- One Go binary: HTTP API, embedded vanilla web UI, SMTP test/direct mode, SQLite.
- Default production mail flow: Cloudflare Email Routing → Worker → Cloudflare Tunnel → `POST /api/email/receive`.
- Supports multiple Cloudflare profiles/accounts and multiple domains in one TempMail instance.
- Optional email copies are OFF by default.

Read in order before changing code:

1. `docs/PRD.md`
2. `docs/ARCHITECTURE.md`
3. `docs/TDD.md`
4. `docs/SDD.md`
5. `docs/RUNBOOK.md`

## Hard rules

- Build Go binaries locally on Windows. Never build on VPS or Armbian.
- Use `CGO_ENABLED=0`; SQLite uses `modernc.org/sqlite`.
- Never commit secrets, tokens, `.env`, databases, backups, cloudflared credentials, admin credentials, or built binaries.
- Do not touch a VPS deployment while task targets Armbian, and vice versa.
- Do not silently enable email copies or forwarding.
- Cloudflare profile tokens/account IDs must not cross profiles.
- Domain removal from menu must not delete Cloudflare DNS/zone/routing unless user explicitly asks.
- Do not claim Windows/macOS persistent-service support until adapters are written and tested.

## Commands

```bash
cd receiver
go test ./...
go vet ./...

# Windows local cross-builds
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o tempmail-linux-arm64 .
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tempmail-linux-amd64 .

# Validate non-Go code
bash -n deploy/armbian/install.sh
bash -n deploy/armbian/tempmailctl
node --check cloudflare-worker/src/index.js
```

## Code constraints

- Run `gofmt` and Go tests on changed Go code.
- DB queries with parameters must use `placeholder()` for SQLite/PostgreSQL compatibility.
- API paths use `/api/*`; SSE is `/sse/:email?token=JWT`.
- Worker HTTP endpoint requires `X-Email-API-Key` whenever configured.
- Preserve raw recipient `to`; it identifies the target inbox.
- Embedded frontend is vanilla HTML/JS, not React/Next.js.
- Update PRD/architecture/runbook when API, deployment, menu, Cloudflare, or security behavior changes.

## Linux deployment facts

```text
Binary:   /opt/tempmail/tempmail
Config:   /etc/tempmail/tempmail.env
CF state: /etc/tempmail/cloudflare-state.json
Menu:     menutempmail
DB:       /opt/tempmail/data/tempmail.db
```

After deploying binary:

```bash
sudo systemctl restart tempmail
curl -fsS http://127.0.0.1:8080/health
```

Run the relevant runbook verification before reporting deploy complete.
