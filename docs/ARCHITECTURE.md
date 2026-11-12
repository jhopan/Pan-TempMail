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
| `receiver/web` | `go:embed` static UI: inbox (`/app/`) plus the sidebar admin panel (`/admin`, landing panel `Dashboard`) |

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

cf_profiles
  id, label, account_id, token (AES-256-GCM, key from JWT_SECRET), worker_deployed, created_at, updated_at

cf_domains
  domain UNIQUE, profile_id, zone_id, enabled, routing, created_at, updated_at

settings
  key, value    -- cf_active_profile, cf_tunnel_name, cf_tunnel_id, cf_tunnel_web_host, cf_copies, naming_rules
```

`owner_id` is enforced: an inbox created through the web UI belongs to the logged-in user, and a
non-admin user can only read, wait on, delete, or follow (SSE) their own inboxes. `admin` sees
everything, and automation callers authenticated with `X-Email-API-Key` are unrestricted. Inboxes with
no owner (created by automation) are readable only by admin or API key.

Cloudflare state lives in SQLite, so the admin panel and `menutempmail` read and write the same records. `/etc/tempmail/cloudflare-state.json` is legacy input only, imported once through the panel or the menu.

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

/api/admin/cloudflare/status              admin JWT
/api/admin/cloudflare/profiles[/:id]      admin JWT
/api/admin/cloudflare/profiles/:id/activate
/api/admin/cloudflare/profiles/:id/deploy-worker
/api/admin/cloudflare/profiles/:id/worker
/api/admin/cloudflare/domains[/:domain]   admin JWT
/api/admin/cloudflare/domains/:domain/routing
/api/admin/cloudflare/tunnel[/create]     admin JWT
/api/admin/cloudflare/copies              admin JWT
/api/admin/cloudflare/import              admin JWT
/api/admin/system/status                  admin JWT
/api/admin/system/action                  admin JWT (via root helper)
```

Domain list for the web UI comes from `cf_domains` (`enabled` and `routing` set), with `DOMAINS` from the env file as fallback, so adding a domain takes effect without a service restart.

## Cloudflare multi-profile model

```text
Profile CF A (stored in SQLite, token encrypted)
  API token A + account ID A
  domain-a.example → Worker A → shared tunnel URL → TempMail

Profile CF B
  API token B + account ID B
  domain-b.example → Worker B → shared tunnel URL → TempMail
```

Each domain is bound to one profile. Workers are deployed per profile through the Cloudflare REST API
(`receiver/cloudflare`), so no `wrangler` or Node.js is needed on the host. One Cloudflare Tunnel
hostname serves as the Worker destination for all profiles, provided each Worker receives the same URL
and `EMAIL_API_KEY`. The tunnel connector itself stays a root-managed `cloudflared` service; the app
reads its state through the Cloudflare API and never writes `/etc/cloudflared`.

## Privilege separation (root helper)

The app must not be root: restarting services, reading logs, running backups, and installing the
tunnel connector all need root on the host, so those live in a separate root component that the app
can only *ask*, never command.

```text
TempMail app (User=tempmail, NoNewPrivileges=true, ProtectSystem=full, RW only /opt/tempmail/data)
  writes  /opt/tempmail/data/helper/in/<id>.json        request, 0640, tempmail:tempmail
        │
tempmail-root.path  (DirectoryNotEmpty -> starts the service)
tempmail-root.timer (retry every 30s, safety net)
        ▼
/usr/local/sbin/tempmail-root --once                    root, oneshot, Python
  validate: JSON schema, 4 KiB cap, owner == tempmail, no symlink, action allowlist, arg rules
  execute:  systemctl / journalctl / cloudflared, argument list only, never a shell
  write     /opt/tempmail/data/helper/out/<id>.json      result, 0640, root:tempmail
        ▼
TempMail app reads the result, deletes it, returns it to the admin panel
```

Rules that keep this safe:

- No `sudoers` entry and no change to the app's systemd sandbox; the app never gains root.
- Allowlist only: `service-status`, `restart-service`, `backup-now`, `logs`, `doctor`, `tunnel-status`, `tunnel-install`, `tunnel-restart`, `tunnel-logout`. Anything else is rejected.
- Parameters are validated (`lines` 1-500, tunnel token charset, `RESET` confirmation for logout) and passed as an argument list, never interpolated into a shell.
- Cooldowns per mutating action (`restart-service` 10s, `tunnel-install` 30s, `tunnel-logout` 60s, `backup-now` 10s) block request loops.
- The helper always answers: a rejected request still gets a `rejected` result, then the request file is removed, so the spool cannot wedge.
- `HELPER_DIR` belongs to the app user, `helper/state` belongs to root, results are 0640 root:tempmail.
- With the helper units disabled the panel still works and reports `helper belum dipasang`; service/log/tunnel actions are refused with `501`.

## Files persisted on verified Linux host

```text
/opt/tempmail/tempmail                    binary
/opt/tempmail/data/tempmail.db            SQLite, includes cf_profiles and cf_domains
/opt/tempmail/backups/                    daily DB copies
/opt/tempmail/worker/                     Worker source, reference copy for cloudflare-worker/src/index.js
/etc/tempmail/tempmail.env                secrets/config, root:tempmail 0640
/etc/tempmail/cloudflare-state.json       legacy input for the one-time import, root-only 0600
/opt/tempmail/data/helper/{in,out}        root helper spool, tempmail:tempmail 0750
/opt/tempmail/data/helper/state           helper cooldown state, root:root 0700
/usr/local/sbin/tempmail-root             root helper (validated server actions)
/etc/cloudflared/config.yml               named tunnel config (root managed)
/usr/local/bin/menutempmail               menu launcher
```

## Critical design constraints

- `modernc.org/sqlite` is pure Go. Keep `CGO_ENABLED=0` to support Windows → Linux cross-compilation.
- SQLite queries must run through `placeholder()` when parameters are used.
- Email Worker raw mail comes from `await new Response(message.raw).text()`.
- `POST /api/email/receive` must preserve original recipient (`to`) because recipient is inbox identity.
- Ingestion is capped by `MAX_RAW_EMAIL_BYTES` (default 5 MiB) on both the Worker and SMTP paths; larger payloads get `413` before parsing.
- When `WORKER_SIGNING_SECRET` is set, `/api/email/receive` also requires `X-Email-Signature: sha256=<hmac>` over the raw body, and the Worker receives the same value as the `TEMPMAIL_SIGNING_SECRET` binding during deploy. With the secret empty the endpoint only needs `EMAIL_API_KEY` and logs that once at the first request.
- Static UI must call API routes under `/api/`; only SSE is outside `/api`.
- Static assets under `/app` are served with `Cache-Control: no-cache, must-revalidate`, and every panel in `admin.html` is `hidden` except the default one, so a stale cached page cannot render as an empty screen when `admin-nav.js` asks for a panel it does not have. If a panel is missing from the markup, the panel script shows a notice and reloads once.
- The Cloudflare zone overrides asset caching with `max-age=14400` for JS/CSS, so **bump `?v=N` in `index.html` and `admin.html` on every JS change**; otherwise a browser can keep an old script for four hours even though the origin already served the new one.
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
