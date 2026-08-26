# TempMail By JhopanStore

Low-resource disposable email platform for manual use and legitimate test/automation workflows.

```text
Cloudflare Email Routing → Worker → Cloudflare Tunnel → TempMail → SQLite
```

No public port 25, Maddy, PostgreSQL, Node.js runtime, or Nginx is required in Cloudflare mode.

## Highlights

- Single Go binary: HTTP API, embedded vanilla web UI, direct/local SMTP mode, SQLite.
- Verified low-resource Armbian deployment: TempMail process around 20MB RSS.
- Automation API: create inbox, wait for email, inspect message, extract OTP/link.
- Multiple temp-mail domains in one instance.
- Domains can originate from multiple Cloudflare account profiles.
- One Cloudflare Tunnel can route all profile Workers to one TempMail host.
- Optional email copies: **OFF by default**.
- Linux menu: `menutempmail`.

## Status

| Component | State |
|---|---|
| Go core | Working |
| Embedded web UI | Working |
| SQLite/WAL/cleanup | Working |
| Direct/local SMTP | Working |
| Cloudflare HTTP receive endpoint | Working; HTTP simulated |
| Armbian systemd deployment | Verified |
| Cloudflare multi-profile menu | Implemented; real account setup pending |
| macOS launchd installer | Not implemented |
| Windows Service installer | Not implemented |
| GitHub Release bootstrap | Script exists; release pipeline/assets not published |

## Architecture

```text
Email to any@domain.example
        ↓
Cloudflare Email Routing catch-all
        ↓
Worker for that Cloudflare profile
        ├─ optional copies, only if explicitly configured
        └─ HTTPS → shared Cloudflare Tunnel
                       ↓
             TempMail By JhopanStore
             HTTP API + web UI + SQLite
```

The original recipient is preserved. Therefore these are separate inboxes:

```text
abc@domain.example
otp@domain.example
github-test@another-domain.example
```

## Quick local run

```bash
cd receiver
cp ../.env.example .env
# Set JWT_SECRET and DOMAINS in .env.
go run .
```

Open:

```text
http://127.0.0.1:8080/app/
```

## Verified Linux / Armbian deployment

Read [`docs/ARMBIAN.md`](docs/ARMBIAN.md) and [`docs/RUNBOOK.md`](docs/RUNBOOK.md).

On installed host:

```bash
menutempmail
```

Dashboard shows service health, memory, domains, inbox counts, Cloudflare profiles, tunnel state, Worker/routing status, and optional email copies.

Current verified host layout:

```text
/opt/tempmail/tempmail
/etc/tempmail/tempmail.env
/etc/tempmail/cloudflare-state.json
/opt/tempmail/data/tempmail.db
```

### Build locally, deploy binary

Build on Windows. Do not build on VPS/Armbian:

```bash
cd receiver
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o tempmail-linux-arm64 .
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tempmail-linux-amd64 .
```

## Cloudflare menu model

`menutempmail` supports separate account profiles:

```text
CF A
└─ domain-a.example → Worker A → shared tunnel → TempMail

CF B
└─ domain-b.example → Worker B → shared tunnel → TempMail
```

Recommended order:

```text
1. Add CF profile A in menu
2. Add active domain A
3. Login/create shared Tunnel web hostname
4. Deploy Worker A
5. Enable catch-all for domain A
6. Repeat with profile B/domain B
```

A domain removal from the menu only removes it from TempMail local configuration. It does not delete Cloudflare DNS, zone, or routing automatically.

## Optional email copies

Default:

```text
OFF
```

Without copies:

```text
anything@domain.example → Worker → TempMail only
```

Configure copies only from menu when needed. Multiple comma-separated destination addresses are supported. Cloudflare must verify each destination address.

## Automation API

```text
POST   /api/setup                        bootstrap first admin only
POST   /api/auth/login                   JWT login
POST   /api/inbox                        create inbox
GET    /api/inbox/:email                 list messages
GET    /api/inbox/:email/wait?timeout=60 wait for message
GET    /api/message/:id                  message + OTP codes + links
POST   /api/email/receive                Cloudflare Worker ingestion
GET    /sse/:email?token=JWT             live updates
GET    /health                           health check
GET    /api/admin/naming-rules           naming rules for auto-generated inboxes
PUT    /api/admin/naming-rules           update naming rules (admin JWT)
```

Example flow:

```bash
# Create an address.
POST /api/inbox
{"username":"test-123","domain":"domain.example"}

# Trigger a legitimate service/test mail.

# Wait up to 60 seconds; response includes extracted codes/links.
GET /api/inbox/test-123@domain.example/wait?timeout=60
```

## Development

```bash
cd receiver
go test ./...
go vet ./...
```

Docs for future development:

- [`AGENTS.md`](AGENTS.md) — repository rules and agent/developer instructions.
- [`docs/PRD.md`](docs/PRD.md) — product requirements.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — system design.
- [`docs/TDD.md`](docs/TDD.md) — test discipline.
- [`docs/SDD.md`](docs/SDD.md) — spec-driven change process.
- [`docs/RUNBOOK.md`](docs/RUNBOOK.md) — deploy, operation, test, recovery.
- [`docs/ARMBIAN.md`](docs/ARMBIAN.md) — Linux/Armbian deployment specifics.

## Security

- `JWT_SECRET` is mandatory; no hardcoded fallback.
- Worker ingress uses `X-Email-API-Key`.
- Keep Cloudflare API tokens, Worker secrets, DB, and admin credentials outside Git.
- Keep port 8080 private/LAN-only unless accessed through Cloudflare Tunnel/Tailscale.
- Email forwarding/copies are opt-in.

## License

MIT. Footer/credit: **TempMail By JhopanStore**.
