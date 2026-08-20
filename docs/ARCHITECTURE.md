# Architecture — TempMail By JhopanStore

## Runtime topology

```text
                          optional copies: OFF by default
                                        ┌─────────────────────┐
                                        │ configured mailbox  │
                                        └─────────────────────┘
                                                  ▲
                                                  │ optional
Sender → CF Email Routing → Profile Worker ──────┼────→ Cloudflare Tunnel
                                                  │              │
                                                  └──────────────┘
                                                                 ▼
                                                        TempMail Go binary
                                                        HTTP :8080
                                                        SMTP :2525 local test
                                                                 │
                          ┌──────────────────────────────────────┼────────────────────────────┐
                          ▼                                      ▼                            ▼
                    embedded web UI                         automation API                 SQLite WAL
                     /app/                                  /api/inbox                  data/tempmail.db
```

No Maddy, PostgreSQL, Node.js runtime, Nginx, or public port 25 is required for Cloudflare mode.

## Go packages

| Package | Responsibility |
|---|---|
| `receiver/main.go` | config load, DB, SMTP start, HTTP server, graceful shutdown |
| `receiver/api` | Gin routes, CORS, SSE handler/rate limits |
| `receiver/events` | shared SSE broker; exists to prevent `api ↔ handlers` import cycle |
| `receiver/handlers` | inbox, message, attachment, admin bootstrap, HTTP email receive |
| `receiver/smtp` | direct/local SMTP ingestion |
| `receiver/parser` | RFC822/MIME parsing |
| `receiver/repository` | SQL operations; DB-dialect placeholders |
| `receiver/database` | SQLite default, optional PostgreSQL connection/migrations |
| `receiver/cleanup` | TTL purge and SQLite optimization |
| `receiver/web` | `go:embed` static UI |

## Data model

```text
users
  id, email, password_hash, role

inboxes
  id, email UNIQUE, type, owner_id, expires_at, created_at

messages
  id, inbox_id, sender, subject, html_body, text_body, codes, links, received_at

attachments
  id, message_id, filename, content_type, size, data
```

`owner_id` exists in schema but access enforcement is not complete. Treat multi-user privacy as an active roadmap item.

## API map

```text
GET    /health
GET    /api/config
POST   /api/setup
POST   /api/auth/login
POST   /api/email/receive                 Worker auth: X-Email-API-Key

POST   /api/inbox                         JWT
GET    /api/inbox/:email                  JWT
GET    /api/inbox/:email/wait             JWT
DELETE /api/inbox/:email                  JWT
GET    /api/message/:id                   JWT
DELETE /api/message/:id                   JWT
GET    /api/message/:id/attachments       JWT
GET    /api/attachment/:id                JWT
GET    /sse/:email?token=JWT
```

## Cloudflare multi-profile model

```text
Profile CF A
  API token A + account ID A
  domain-a.example → Worker A → shared tunnel URL → TempMail

Profile CF B
  API token B + account ID B
  domain-b.example → Worker B → shared tunnel URL → TempMail
```

Each domain is bound to one profile. Workers are deployed per profile. Armbian can use a single Cloudflare Tunnel web hostname as the Worker destination for all profiles, provided each Worker knows the same URL and `EMAIL_API_KEY`.

## Files persisted on verified Linux host

```text
/opt/tempmail/tempmail                    binary
/opt/tempmail/data/tempmail.db            SQLite
/opt/tempmail/backups/                    daily DB copies
/opt/tempmail/worker/                     Worker source used by menu
/etc/tempmail/tempmail.env                secrets/config, root:tempmail 0640
/etc/tempmail/cloudflare-state.json       profile tokens/config, root-only 0600
/etc/cloudflared/config.yml               named tunnel config
/usr/local/bin/menutempmail               menu launcher
```

## Critical design constraints

- `modernc.org/sqlite` is pure Go. Keep `CGO_ENABLED=0` to support Windows → Linux cross-compilation.
- SQLite queries must run through `placeholder()` when parameters are used.
- Email Worker raw mail comes from `await new Response(message.raw).text()`.
- `POST /api/email/receive` must preserve original recipient (`to`) because recipient is inbox identity.
- Static UI must call API routes under `/api/`; only SSE is outside `/api`.
- The tunnel hostname is web/API transport. It is not necessarily a temp-mail domain.
- Email copies must remain optional and off by default.

## Platform support state

| Platform | Binary build | Persistent service |
|---|---:|---:|
| Debian/Ubuntu/Armbian Linux | verified | verified systemd |
| Linux amd64 | build supported | systemd template expected; not separately verified |
| macOS | build supported | launchd not implemented |
| Windows | build supported | Windows Service not implemented |
| WSL | build supported | foreground/Task Scheduler only; systemd depends on setup |

Do not document an unverified service adapter as available.
