# TempMail By JhopanStore

Low-resource disposable email platform for manual use and legitimate test/automation workflows.

```text
Cloudflare Email Routing → Worker → Cloudflare Tunnel → TempMail → SQLite
```

No public port 25, Maddy, PostgreSQL, Node.js runtime, or Nginx is required in Cloudflare mode.

## Highlights

- Single Go binary: HTTP API, embedded vanilla web UI, direct/local SMTP mode, SQLite.
- Verified low-resource Debian deployment: ~20MB RSS.
- **Web UI requires login** (JWT). No guest mode, no register button — safe from public access.
- **Automation API** with `X-Email-API-Key` header: create inbox, wait for email, extract OTP/link.
- **Naming rules**: human-like Indonesian names for web UI auto-generate. Automation scripts can send custom `username`.
- Multiple domains and Cloudflare profiles in one instance.
- Optional email copies: **OFF by default**.
- Linux menu: `menutempmail`.

## Architecture

```text
Email to any@domain.example
        ↓
Cloudflare Email Routing catch-all
        ↓
Worker tempmailbyjhopanstore (trigger: email event)
        ├─ optional copies (only if FORWARD_LIST configured)
        └─ POST /api/email/receive → Cloudflare Tunnel → Go server
                                        ↓
                                  Parse email
                                  Extract OTP + links
                                  Save to SQLite
                                  Push SSE to web UI
```

### Why Worker (not Pages)?

Cloudflare Email Routing can only deliver emails to:
1. **Worker** (run code on email event) ← we use this
2. Forward to another email address
3. Drop

There is no "POST to URL" option without a Worker. Pages cannot receive email events.

Our Worker is 72 lines of JS. It acts as a courier: reads raw email, POSTs to our Go server via tunnel, done. It stores nothing.

Worker security:
- `workers_dev = false` — no public `.workers.dev` URL
- Only triggers on `email` event, not public HTTP
- POST includes `X-Email-API-Key` for auth
- No database, no storage

## Fresh setup on a new machine

### Prerequisites

- Debian/Ubuntu/Armbian x86_64 or arm64
- Root access
- Domain on Cloudflare (nameservers pointing to CF)

### Step 1: Download binary from GitHub Releases

```bash
wget https://github.com/jhopan/TempMailByJhopanstore/releases/download/v1.0.0/tempmail-linux-amd64 -O /tmp/tempmail
chmod +x /tmp/tempmail
```

### Step 2: Clone repo (for scripts + worker)

```bash
git clone https://github.com/jhopan/TempMailByJhopanstore.git
cd TempMailByJhopanstore
```

### Step 3: Install

```bash
sudo bash deploy/linux/install.sh
sudo cp /tmp/tempmail /opt/tempmail/tempmail
sudo chown tempmail:tempmail /opt/tempmail/tempmail
```

This installs:
- Go binary to `/opt/tempmail/tempmail`
- Worker project to `/opt/tempmail/worker/`
- Config template to `/etc/tempmail/tempmail.env`
- Menu launcher `menutempmail` to `/usr/local/bin/`
- Control panel `tempmailctl` to `/usr/local/sbin/`
- systemd service

### Step 4: Configure

Edit `/etc/tempmail/tempmail.env`:

```bash
sudo nano /etc/tempmail/tempmail.env
```

Set these (all required):
```ini
PORT=8081
DOMAINS=your-domain.com
JWT_SECRET=    # generate: openssl rand -hex 32
EMAIL_API_KEY= # generate: openssl rand -hex 32
SITE_NAME=TempMail By JhopanStore
GIN_MODE=release
```

**Generate secrets:**
```bash
openssl rand -hex 32  # for JWT_SECRET
openssl rand -hex 32  # for EMAIL_API_KEY
```

### Step 5: Create admin user

```bash
curl -X POST http://127.0.0.1:8081/api/setup \
  -H "Content-Type: application/json" \
  -d '{"email":"your@email.com","password":"your-password"}'
```

This only works once (when no users exist). After that, `/api/setup` is permanently locked.

### Step 6: Cloudflare guided setup

```bash
menutempmail
# Choose option 8 — SETUP GUIDED
```

Flow:
1. Install dependencies (node, cloudflared, wrangler)
2. Login Wrangler — device auth (buka URL + masukkan kode)
3. Login Cloudflare Tunnel — pilih zona → cert.pem otomatis
4. Buat tunnel + DNS (nama tunnel default: `tempmailbyjhopanstore`)
5. Deploy worker `tempmailbyjhopanstore` + set secrets
6. Aktifkan Email Routing + catch-all
7. Simpan state + verify

**Note:** If Email Routing enable fails with "Non-Cloudflare MX records exist", go to Cloudflare Dashboard → DNS → delete all MX records, then re-run step 6.

### Step 7: Verify

```bash
menutempmail    # check dashboard — all green
```

Open `https://your-subdomain.your-domain.com/app/` → login with admin credentials.

## Build and release

Binaries are built via **GitHub Actions** — no manual build needed.

### Trigger a release

```bash
git tag v1.0.0
git push origin v1.0.0
```

GitHub Actions automatically builds:
- `tempmail-linux-amd64` — Linux x86_64 (Debian/Ubuntu/Armbian)
- `tempmail-linux-arm64` — Linux ARM64 (Raspberry Pi 4/5)
- `tempmail-windows-amd64.exe` — Windows x86_64

Binaries are attached to the GitHub Release page: https://github.com/jhopan/TempMailByJhopanstore/releases

### Build locally (testing only)

```bash
cd receiver
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tempmail-linux-amd64 .
```

## Host layout

```text
/opt/tempmail/tempmail               Go binary
/opt/tempmail/worker/                Wrangler project (wrangler.toml + src/)
/opt/tempmail/data/tempmail.db       SQLite database
/etc/tempmail/tempmail.env            App config (PORT, DOMAINS, secrets)
/etc/tempmail/cloudflare-state.json   CF state (tunnel, worker, domains)
/etc/cloudflared/config.yml           Tunnel config (SSE: disableChunkedEncoding)
/usr/local/sbin/tempmailctl           Control panel
/usr/local/bin/menutempmail           Menu launcher
/root/.wrangler/config/default.toml  Wrangler OAuth token (device auth)
/root/.cloudflared/cert.pem           Tunnel certificate
```

## Auth model

| Access | Auth method | Use case |
|---|---|---|
| Web UI | JWT login (email + password) | Human using browser — **must login** |
| Automation scripts | `X-Email-API-Key` header | Bots, CI/CD, scripts — no login needed |
| Worker ingestion | `X-Email-API-Key` header | Cloudflare Worker → Go server |
| `/api/config` | None (public) | Returns domains + site_name only (no secrets) |

### Security

- Web UI: **login required**. No guest mode. No register button.
- `/api/setup`: only works when 0 users exist. After first admin, permanently locked.
- `/api/admin/users` (register): requires JWT admin.
- `/api/config`: returns only `domains` + `site_name`. Never exposes `api_key`.
- `workers_dev = false`: Worker not publicly accessible.
- Email copies/forwarding: opt-in, OFF by default.

## Automation API

All inbox/message endpoints accept `X-Email-API-Key` header. No JWT login required for scripts.

### What you need

1. **API Key** — from `/etc/tempmail/tempmail.env` on the server (field `EMAIL_API_KEY`)
2. **Base URL** — your tunnel URL (e.g. `https://tempmail.your-domain.com`)

### Endpoints

| Endpoint | Method | Description |
|---|---|---|
| `/api/inbox` | POST | Create inbox. `{"username":"custom","domain":"..."}` or `{"domain":"..."}` (auto name) |
| `/api/inbox/{email}` | GET | List messages in inbox |
| `/api/inbox/{email}/wait` | GET | Block until email arrives (returns message + codes + links) |
| `/api/message/{id}` | GET | Message detail (subject, body, codes, links) |
| `/api/message/{id}` | DELETE | Delete message |
| `/api/message/{id}/attachments` | GET | List attachments |
| `/api/attachment/{id}` | GET | Download attachment |
| `/api/inbox/{email}` | DELETE | Delete all messages in inbox |
| `/api/email/receive` | POST | Worker ingestion (internal, not for scripts) |
| `/api/config` | GET | Domains + site_name (public, no secrets) |
| `/health` | GET | Health check |

### Example: Full automation flow

```bash
API_KEY="your-api-key"
BASE="https://tempmail.your-domain.com"

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

BASE = "https://tempmail.your-domain.com"
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
  -d '{"email":"your@email.com","password":"your-password"}' \
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

## Cloudflare setup

### Worker

Worker name: `tempmailbyjhopanstore` (fixed in `cloudflare-worker/wrangler.toml`).

Config:
- `workers_dev = false` — no public URL
- `preview_urls = false`
- Secrets set via `wrangler secret put`: `TEMPMAIL_SERVER_URL`, `TEMPMAIL_API_KEY`, `FORWARD_LIST`

### Email Routing

- Catch-all rule: `*@domain → worker:tempmailbyjhopanstore`
- MX records must be Cloudflare-managed (delete existing non-CF MX records first)

### Tunnel

- Tunnel name: `tempmailbyjhopanstore` (default, configurable during setup)
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
3-4: Login Wrangler (device auth) + CF Tunnel
5-6: Deploy worker + email routing
7: Email copies config
8: SETUP GUIDED (one-shot all steps)
9: AUTO SETUP (step-by-step)
L1-L4: Logout/reset/delete worker
10: Refresh status

## Quick local run (development)

```bash
cd receiver
cp ../.env.example .env
# Set JWT_SECRET, EMAIL_API_KEY, and DOMAINS in .env
go run .
# UI at http://127.0.0.1:8080/app/
```

## Development

```bash
cd receiver
go test ./...
go vet ./...
```

## Security

- `JWT_SECRET` and `EMAIL_API_KEY` are mandatory; no hardcoded fallback.
- Worker ingress uses `X-Email-API-Key` header.
- `workers_dev = false` — Worker not publicly accessible.
- Web UI requires login — no guest mode, no register button.
- `/api/setup` locked after first admin created.
- `/api/config` returns no secrets.
- Keep Cloudflare tokens, Worker secrets, DB, and admin credentials outside Git.
- Keep app port private/LAN-only unless accessed through Cloudflare Tunnel.
- Email forwarding/copies are opt-in (OFF by default).

## Docs

- [`AGENTS.md`](AGENTS.md) — repository rules and agent/developer instructions.
- [`docs/PRD.md`](docs/PRD.md) — product requirements.
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — system design.
- [`docs/SDD.md`](docs/SDD.md) — spec-driven change process.
- [`docs/RUNBOOK.md`](docs/RUNBOOK.md) — deploy, operation, test, recovery.

## License

MIT. Footer/credit: **TempMail By JhopanStore**.
