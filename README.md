# TempMail By JhopanStore

Low-resource disposable email platform for manual use and legitimate test/automation workflows.

```text
Cloudflare Email Routing → Worker → Cloudflare Tunnel → TempMail → SQLite
```

No public port 25, Maddy, PostgreSQL, Node.js runtime, or Nginx is required in Cloudflare mode.

## Highlights

- Single Go binary: HTTP API, embedded vanilla web UI, direct/local SMTP mode, SQLite.
- Verified low-resource Debian deployment: TempMail process around 20MB RSS.
- Web UI requires login (JWT). No guest mode — safe from public access.
- Automation API with API key auth (`X-Email-API-Key` header): create inbox, wait for email, extract OTP/link.
- Configurable naming rules: human-like Indonesian names (e.g. `langitbiru42`, `senjaindah17`), editable at runtime via admin API.
- Automation scripts can send custom `username` in `POST /api/inbox` — server uses it directly, no naming rules needed.
- Multiple temp-mail domains in one instance.
- Domains can originate from multiple Cloudflare account profiles.
- One Cloudflare Tunnel can route all profile Workers to one TempMail host.
- Optional email copies: **OFF by default**.
- Linux menu: `menutempmail`.

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

## Quick local run

```bash
cd receiver
cp ../.env.example .env
# Set JWT_SECRET, EMAIL_API_KEY, and DOMAINS in .env.
go run .
```

Open: `http://127.0.0.1:8080/app/`

## Linux deployment

### Prerequisites

- Debian/Ubuntu/Armbian x86_64 or arm64
- Root access
- Domain on Cloudflare (nameservers pointing to CF)

### Setup

```bash
# On the target host after clone:
sudo bash deploy/linux/install.sh

# Then run guided setup (interactive):
menutempmail
# Choose option 8 — SETUP GUIDED
```

Guided setup flow:
1. Install dependencies (node, cloudflared, wrangler)
2. Login Wrangler (device auth — buka URL + masukkan kode)
3. Login Cloudflare Tunnel (pilih zona → cert.pem otomatis)
4. Buat tunnel + DNS route (tanya nama tunnel, default: `tempmailbyjhopanstore`)
5. Deploy worker `tempmailbyjhopanstore` + set secrets
6. Aktifkan Email Routing + catch-all
7. Simpan state + verify

### Build locally, deploy binary

Build on Windows (cross-compile), do not build on VPS:

```bash
cd receiver
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tempmail-linux-amd64 .
```

### Host layout

```text
/opt/tempmail/tempmail              Go binary
/opt/tempmail/worker/               Wrangler project (wrangler.toml + src/)
/etc/tempmail/tempmail.env          App config (PORT, DOMAINS, secrets)
/etc/tempmail/cloudflare-state.json CF state (tunnel, worker, domains)
/opt/tempmail/data/tempmail.db      SQLite database
/usr/local/sbin/tempmailctl         Control panel binary
/usr/local/bin/menutempmail         Menu launcher
```

## Automation API

All inbox/message endpoints accept `X-Email-API-Key` header for automation — no JWT login required for scripts. Web UI requires JWT login.

### Auth model

| Access | Auth method | Use case |
|---|---|---|
| Web UI | JWT (`Authorization: Bearer <token>`) | Human using browser, must login |
| Automation scripts | `X-Email-API-Key` header | Bots, CI/CD, scripts |
| Worker ingestion | `X-Email-API-Key` header | Cloudflare Worker → server |
| `/api/config` | None (public) | Domains + site name only (no secrets) |

### Endpoints

| Endpoint | Method | Auth | Description |
|---|---|---|---|
| `/api/config` | GET | none | Domains + API key (for frontend) |
| `/api/inbox` | POST | API key | Create inbox (auto-generate name from rules) |
| `/api/inbox/{email}` | GET | API key | List messages in inbox |
| `/api/inbox/{email}/wait` | GET | API key | Block until email arrives (returns message + codes + links) |
| `/api/message/{id}` | GET | API key | Message detail (subject, body, codes, links) |
| `/api/message/{id}` | DELETE | API key | Delete message |
| `/api/message/{id}/attachments` | GET | API key | List attachments |
| `/api/attachment/{id}` | GET | API key | Download attachment |
| `/api/inbox/{email}` | DELETE | API key | Delete all messages in inbox |
| `/api/email/receive` | POST | API key | Worker ingestion (raw RFC822 email) |
| `/sse/{email}` | GET | JWT or api_key | Server-Sent Events (live inbox updates) |
| `/health` | GET | none | Health check |
| `/api/auth/login` | POST | none | JWT login (for admin panel) |
| `/api/setup` | POST | none | Bootstrap first admin user |
| `/api/admin/naming-rules` | GET | JWT admin | View naming rules |
| `/api/admin/naming-rules` | PUT | JWT admin | Update naming rules |

### Example: Full automation flow

```bash
API_KEY="your-api-key"
BASE="https://your-domain.com"

# 1a. Create inbox with custom username (script controls name)
curl -s -X POST "$BASE/api/inbox" \
  -H "Content-Type: application/json" \
  -H "X-Email-API-Key: $API_KEY" \
  -d '{"username":"jhopan.cloudflare","domain":"your-domain.com"}'
# → {"email":"jhopan.cloudflare@your-domain.com","expires":"24h",...}

# 1b. OR create inbox with auto-generated name (naming rules)
curl -s -X POST "$BASE/api/inbox" \
  -H "Content-Type: application/json" \
  -H "X-Email-API-Key: $API_KEY" \
  -d '{"domain":"your-domain.com"}'
# → {"email":"langitbiru42@your-domain.com","expires":"24h",...}

# 2. Register on target site using this email
#    (site sends verification email)

# 3. Wait for email (blocking — auto-returns when email arrives)
curl -s "$BASE/api/inbox/jhopan.cloudflare@your-domain.com/wait" \
  -H "X-Email-API-Key: $API_KEY"
# → {"id":7,"codes":["284716"],"links":["https://verify.example.com/token=xyz"],...}

# 4. Extract OTP and verification link from response
#    codes[0] = "284716"
#    links[0] = "https://verify.example.com/token=xyz"
```

### Python example

```python
import requests

BASE = "https://your-domain.com"
HEADERS = {"X-Email-API-Key": "your-api-key"}

# 1. Create inbox with custom username (script controls name)
r = requests.post(f"{BASE}/api/inbox", headers=HEADERS,
                   json={"username": "jhopan.cloudflare", "domain": "your-domain.com"})
email = r.json()["email"]
# Or auto-generated: json={"domain": "your-domain.com"}

# 2. Register on target site with this email
# ... (your registration code here) ...

# 3. Wait for email
r = requests.get(f"{BASE}/api/inbox/{email}/wait",
                 headers=HEADERS, timeout=60)
msg = r.json()

# 4. Extract OTP and link
otp = msg["codes"][0] if msg.get("codes") else None
link = msg["links"][0] if msg.get("links") else None
print(f"OTP: {otp}, Link: {link}")
```

## Naming rules

Inbox addresses are generated from configurable rules stored in the database (settings table, key `naming_rules`).

Default template: `{first}{sep}{second}{num}`

Example output: `langitbiru42`, `senjaindah17`, `bimautama63`

### Tokens

| Token | Description |
|---|---|
| `{first}` | Random word from `first_words` list |
| `{second}` | Random word from `second_words` list |
| `{sep}` | Separator (default: empty) |
| `{num}` | Random number (80% probability, range 10-99 by default) |
| `{rand:N}` | N random alphanumeric characters |
| `{ts}` | Unix timestamp |

### Update rules at runtime (admin)

```bash
# Login first to get JWT
TOKEN=$(curl -s -X POST https://your-domain.com/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"yourpass"}' \
  | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")

# Update naming rules
curl -X PUT https://your-domain.com/api/admin/naming-rules \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "template": "{first}{sep}{second}{num}",
    "separator": "",
    "first_words": ["jhopan", "sari", "bima", "arya"],
    "second_words": ["utama", "sari", "jaya", "mulia"],
    "num_min": 10,
    "num_max": 99,
    "num_probability": 80
  }'
```

### Web UI behavior

- **Login required:** all pages require JWT login. No guest mode. Opening `/app/` without login redirects to `/app/login.html`.
- **Admin login:** use email + password set during `/api/setup` bootstrap.
- After login: auto-generate email from naming rules, or type custom username + Enter.
- **Admin panel:** login via `/app/login.html` for admin features (users, naming rules).

### Security model

- Web UI: JWT login required (no public access).
- Automation: `X-Email-API-Key` header (for scripts only, not exposed in any API response).
- `/api/config`: public but returns only domains + site_name (no secrets).
- Worker: `workers_dev = false`, not publicly accessible.
- Email copies/forwarding: opt-in, OFF by default.

## Cloudflare setup

### Worker

Worker name: `tempmailbyjhopanstore` (fixed).

Worker config (`cloudflare-worker/wrangler.toml`):
- `workers_dev = false` (no public workers.dev URL)
- `preview_urls = false`
- Secrets set via `wrangler secret put`: `TEMPMAIL_SERVER_URL`, `TEMPMAIL_API_KEY`, `FORWARD_LIST`

### Email Routing

- Catch-all rule: `*@domain → worker:tempmailbyjhopanstore`
- Enabled via `wrangler email routing enable` or Cloudflare API
- MX records must be Cloudflare-managed (delete any existing non-CF MX records first)

### Tunnel

- Tunnel name: `tempmailbyjhopanstore` (default, configurable)
- Config: `/etc/cloudflared/config.yml` with `disableChunkedEncoding: true` for SSE support

## menutempmail

Dashboard + control panel:

```text
=== TempMail By JhopanStore ===
        Credit JhopanStore
Platform        : Debian GNU/Linux 13 (trixie) / x86_64
Service         : active | 20.5 MiB
Web LAN         : http://192.168.1.57:8081/app/
Inbox           : 4 inbox | 4 message
SQLite backup   : active
--- Cloudflare profiles ---
* wrangler-oauth: wrangler OAuth | Worker: deployed
Domains:
  renunganbot.qzz.io | profile=wrangler-oauth | routing=ON
Tunnel: 0ee13ee3-... | host: tempmail.renunganbot.qzz.io
CF Tunnel      : active
```

Menu options:
1-2: Manage CF profiles + domains
3-4: Login Wrangler + CF Tunnel (device auth)
5-6: Deploy worker + email routing
7: Email copies config
8: SETUP GUIDED (one-shot all steps)
9: AUTO SETUP (step-by-step)
L1-L4: Logout/reset/delete worker
10: Refresh status

## Security

- `JWT_SECRET` and `EMAIL_API_KEY` are mandatory; no hardcoded fallback.
- Worker ingress uses `X-Email-API-Key` header.
- `workers_dev = false` — Worker not publicly accessible.
- Keep Cloudflare API tokens, Worker secrets, DB, and admin credentials outside Git.
- Keep app port private/LAN-only unless accessed through Cloudflare Tunnel.
- Email forwarding/copies are opt-in (OFF by default).

## Development

```bash
cd receiver
go test ./...
go vet ./...
```

## Docs

- [`AGENTS.md`](AGENTS.md) — repository rules and agent/developer instructions.
- [`docs/PRD.md`](docs/PRD.md) — product requirements.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — system design.
- [`docs/SDD.md`](docs/SDD.md) — spec-driven change process.
- [`docs/RUNBOOK.md`](docs/RUNBOOK.md) — deploy, operation, test, recovery.

## License

MIT. Footer/credit: **TempMail By JhopanStore**.
