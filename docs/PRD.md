# PRD — TempMail By JhopanStore

## 1. Purpose

Self-hosted, ultra-light temporary email platform for manual use and legitimate test/automation flows. Must run with low RAM/CPU, receive mail without public port 25 through Cloudflare Email Routing, and preserve separate inboxes per recipient address.

## 2. Goals

- Single Go binary with embedded web UI and SQLite.
- Low resource baseline: target <100MB RAM including tunnel process on Linux.
- One instance supports many temp-mail domains.
- Multiple Cloudflare accounts/profiles may contribute domains to one TempMail instance.
- Per-domain catch-all routing: `anything@domain.example` reaches its own inbox.
- Automation API can create an inbox and wait for a message without client polling.
- Extract common OTP codes and links from received mail.
- Optional email copies are OFF by default.
- One interactive menu (`menutempmail`) handles Linux/Armbian setup and ongoing Cloudflare/domain operations.

## 3. Non-goals

- Sending outbound email.
- Full IMAP/SMTP mailbox service.
- Public anonymous inbox browsing.
- Bypassing site anti-abuse controls or using temp email for fraud.
- Claiming production adapters for Windows/macOS until implemented and tested.

## 4. Users

| User | Need |
|---|---|
| Owner/admin | Configure Cloudflare accounts, domains, Worker, Tunnel, and users. |
| Automation client | Create unique address, wait for verification mail, read OTP/link. |
| Manual user | Open web UI, inspect inbox, download attachment, clear data. |

## 5. Core flows

### 5.1 HTTP email flow — default for cloud hosts

```text
Sender
→ Cloudflare Email Routing catch-all
→ Worker for that Cloudflare profile
→ HTTPS Cloudflare Tunnel
→ POST /api/email/receive
→ parse + SQLite + SSE
→ web/API inbox
```

`message.to` is carried unchanged. This separates `abc@domain.example` from `otp@domain.example`.

### 5.2 Direct SMTP — optional

```text
Sender → port 25 → built-in Go SMTP → SQLite
```

Use only when public inbound port 25 is known to work. On Armbian default install SMTP binds to loopback `127.0.0.1:2525` for local tests only.

### 5.3 Optional email copy

```text
Worker → TempMail always
Worker → configured destination(s) only when owner explicitly enables copies
```

Empty destination list means OFF. A copy destination must be verified by Cloudflare before forwarding works.

## 6. Functional requirements

### TempMail app

- `POST /api/setup` creates first admin only when no users exist.
- `POST /api/auth/login` issues JWT.
- `POST /api/inbox` creates/extends inbox.
- `GET /api/inbox/:email/wait` waits up to bounded timeout.
- Message response includes parsed body, OTP codes, verification links, and attachment metadata.
- Inbox TTL default 24 hours; cleanup job purges stale data.
- Static UI is available under `/app/`.

### Menu

- Display dashboard before choices.
- Show OS/CPU, app health, memory, domain list, inbox/message count, backup status, Cloudflare profiles, tunnel, Worker state, routing state, optional copies.
- Add/select/remove Cloudflare profiles safely.
- Add/remove TempMail domains safely.
- Domain removal from menu never deletes Cloudflare zone/DNS/routing by default.
- Support worker deploy and routing enable per active profile/domain.

### Cloudflare

- Each profile stores account ID and token root-only on host.
- Each domain belongs to exactly one profile.
- Tunnel can serve one web hostname and receive Worker traffic from domains across profiles.
- Catch-all changes require explicit confirmation.

## 7. Security requirements

- `JWT_SECRET` required; never fallback to hardcoded secret.
- `EMAIL_API_KEY` must protect Worker receive endpoint.
- Never commit credentials, database files, cloudflared credentials, token files, or built binaries.
- Copies are explicit opt-in.
- Input validation at all HTTP boundaries.
- Run public app behind Cloudflare Tunnel/TLS. No router port-forward requirement.

## 8. Reliability requirements

- systemd restarts app after crash on Linux.
- SQLite WAL enabled.
- Daily SQLite backups retained seven days.
- Cloudflare Tunnel reconnects via service.
- Email copy is not assumed as primary storage; TempMail must report delivery failures in Worker logs.

## 9. Success criteria

- Fresh Linux/Armbian deployment reaches `/health`.
- Admin can login to `/app/`.
- Local SMTP or HTTP Worker simulation stores a message.
- OTP/link extraction succeeds for test message.
- `menutempmail` shows accurate dashboard state.
- A domain from CF A and a domain from CF B can route to the same TempMail instance after their respective Worker/routing setup.

## 10. Roadmap

1. Complete and test multi-profile Cloudflare control panel against real accounts.
2. Add schema-backed inbox ownership checks for non-admin users.
3. Add bounded raw-email size and signed Worker requests.
4. Add release pipeline with Linux arm64/amd64 binaries.
5. Implement/test launchd (macOS) and Windows Service adapters.
6. Add automatic Cloudflare routing status reconciliation.
