# ⚡ TempMail By JhopanStore

> **Disposable email platform — low resource, high impact.**
> Single Go binary. Cloudflare edge. SQLite storage. Zero external dependencies.

```text
╔══════════════════════════════════════════════╗
║      TempMail By JhopanStore                 ║
║        Credit JhopanStore                    ║
╚══════════════════════════════════════════════╝
```

---

## ✨ Features

| Feature | Description |
|---|---|
| 🚀 **Single binary** | One Go binary — HTTP API, web UI, SMTP, SQLite. No Node.js, Nginx, or Maddy needed. |
| 💾 **Low resource** | ~20MB RAM. Runs on Debian, Ubuntu, Armbian, Raspberry Pi. |
| ☁️ **Cloudflare edge** | Email Routing → Worker → Tunnel → your server. No port 25, no public IP. |
| 🔐 **Secure by default** | Web UI requires login. No register button. No guest mode. API key for automation only. |
| 🤖 **Automation API** | Create inbox, wait for email, extract OTP/links. Blocking `/wait` endpoint — no polling. |
| 📛 **Naming rules** | Human-like Indonesian names (`langitbiru42`, `senjaindah17`). Editable at runtime. Or send custom `username`. |
| 🎨 **No CDN** | Pure inline CSS. Zero external requests. Works offline. |
| 🖥️ **Supermenu** | `menutempmail` — dashboard + control panel. Everything from one menu. |
| 📦 **GitHub Actions** | Tag → auto build → release. Download binary, run setup. No manual build. |

---

## 🏗️ Architecture

```text
Email to anything@your-domain.com
        │
        ▼
Cloudflare Email Routing (catch-all: *@your-domain.com)
        │
        ▼
Worker: tempmailbyjhopanstore (72 lines JS, edge)
  ├─ optional copies (OFF by default)
  └─ POST /api/email/receive (with X-Email-API-Key)
        │
        ▼
Cloudflare Tunnel: tempmailbyjhopanstore
        │
        ▼
Your Server (Go binary on :8081)
  ├─ Parse email (RFC822)
  ├─ Extract OTP codes + verification links
  ├─ Store in SQLite
  └─ Push SSE to web UI (real-time)
```

### Why Worker (not Pages)?

Cloudflare Email Routing can only deliver emails to:
1. **Worker** (run code on email event) ← we use this
2. Forward to another email
3. Drop

Pages cannot receive email events. Our Worker is a 72-line courier — reads email, POSTs to our server, stores nothing. `workers_dev = false` — not publicly accessible.

---

## 🚀 Quick Setup (one command)

On a fresh Debian/Ubuntu machine:

```bash
curl -fsSL https://raw.githubusercontent.com/jhopan/TempMailByJhopanstore/main/deploy/linux/quick-setup.sh | sudo bash
```

This will:
1. ⬇️ Download binary from GitHub Releases
2. 📦 Clone repo + install scripts
3. ⚙️ Generate crypto-safe secrets
4. ❓ Ask: domain, port, admin email, admin password
5. ▶️ Start service + health check
6. 👤 Create admin user

Then run Cloudflare setup:
```bash
menutempmail → 8 (SETUP GUIDED)
```

---

## 📥 Manual Setup

### Step 1: Download binary

```bash
wget https://github.com/jhopan/TempMailByJhopanstore/releases/latest/download/tempmail-linux-amd64 -O /tmp/tempmail
chmod +x /tmp/tempmail
```

### Step 2: Clone repo

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

### Step 4: Configure

```bash
sudo nano /etc/tempmail/tempmail.env
```

```ini
PORT=8081
DOMAINS=your-domain.com
JWT_SECRET=          # openssl rand -hex 32
EMAIL_API_KEY=       # openssl rand -hex 32
SITE_NAME=TempMail By JhopanStore
GIN_MODE=release
```

### Step 5: Start + create admin

```bash
sudo systemctl start tempmail

curl -X POST http://127.0.0.1:8081/api/setup \
  -H "Content-Type: application/json" \
  -d '{"email":"your@email.com","password":"your-password"}'
```

### Step 6: Cloudflare setup

```bash
menutempmail → 8
```

### Step 7: Verify

```bash
menutempmail    # check dashboard — all green
```

Open `https://tempmail.your-domain.com/app/` → login.

---

## 🔐 Auth Model

| Access | Auth | Use case |
|---|---|---|
| 🌐 Web UI | JWT login (email + password) | Human — **must login**, no register, no guest |
| 🤖 Automation | `X-Email-API-Key` header | Scripts, bots, CI/CD — no login needed |
| 📧 Worker | `X-Email-API-Key` header | Cloudflare Worker → server |
| ⚙️ `/api/config` | Public | Domains + site name only — **no secrets** |

### Security

- ✅ Web UI: login required. No register button. No guest mode.
- ✅ `/api/setup`: locked after first admin created.
- ✅ `/api/admin/users` (register): requires JWT admin.
- ✅ `/api/config`: returns only `domains` + `site_name`. Never exposes API key.
- ✅ `workers_dev = false`: Worker not publicly accessible.
- ✅ Email copies: OFF by default.
- ✅ No secrets in repo. No passwords, tokens, or domains committed.

---

## 🤖 Automation API

### What you need

1. **API Key** — from `/etc/tempmail/tempmail.env` on the server
2. **Base URL** — your tunnel URL (e.g. `https://tempmail.your-domain.com`)

### Endpoints

| Endpoint | Method | Description |
|---|---|---|
| `/api/inbox` | POST | Create inbox (`{"username":"custom","domain":"..."}` or `{"domain":"..."}` for auto name) |
| `/api/inbox/{email}` | GET | List messages |
| `/api/inbox/{email}/wait` | GET | Block until email arrives → returns message + codes + links |
| `/api/message/{id}` | GET | Message detail (codes, links, body) |
| `/api/message/{id}` | DELETE | Delete message |
| `/api/message/{id}/attachments` | GET | List attachments |
| `/api/attachment/{id}` | GET | Download attachment |
| `/api/inbox/{email}` | DELETE | Delete all messages in inbox |
| `/api/config` | GET | Domains + site_name (public, no secrets) |
| `/health` | GET | Health check |

📖 **Full API docs:** [`docs/API.md`](docs/API.md) — all endpoints with examples in Bash, Python, Node.js.

### Quick example

```bash
API_KEY="your-api-key"
BASE="https://tempmail.your-domain.com"

# 1. Create inbox
curl -s -X POST "$BASE/api/inbox" \
  -H "X-Email-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"username":"my-test","domain":"your-domain.com"}'
# → {"email":"my-test@your-domain.com",...}

# 2. Register on site with this email...

# 3. Wait for email (blocking — auto-returns)
curl -s "$BASE/api/inbox/my-test@your-domain.com/wait" \
  -H "X-Email-API-Key: $API_KEY"
# → {"codes":["123456"],"links":["https://verify.example.com/..."],...}
```

---

## 📛 Naming Rules

Auto-generated inbox addresses use configurable rules:

```
Template: {first}{sep}{second}{num}
Output:   langitbiru42, senjaindah17, bimautama63
```

| Token | Description |
|---|---|
| `{first}` | Random word from first_words list |
| `{second}` | Random word from second_words list |
| `{sep}` | Separator (default: empty) |
| `{num}` | Random number (80% probability, 10-99) |
| `{rand:N}` | N random alphanumeric chars |
| `{ts}` | Unix timestamp |

**Automation scripts can skip naming rules** — just send `"username":"custom-name"` in `POST /api/inbox`.

Update rules at runtime via admin API → see [`docs/API.md`](docs/API.md).

---

## 🖥️ menutempmail — Supermenu

```text
╔══════════════════════════════════════════════╗
║      TempMail By JhopanStore                 ║
║        Credit JhopanStore                    ║
╚══════════════════════════════════════════════╝

  Platform         Debian GNU/Linux 13 / x86_64
  Service          active | 20.5 MiB
  Admin            your@email.com
  Password         your-password
  Domains          your-domain.com
  Web LAN          http://192.168.x.x:8081/app/
  Web Tunnel       https://tempmail.your-domain.com/app/
  Inbox            2 inbox | 1 message
  SQLite Backup    active
  API Key          your-api-key-here

  ─── Cloudflare (Tunnel) ───
  Login            ✓ login (cert.pem)
  Tunnel Name      tempmailbyjhopanstore
  Tunnel ID        your-tunnel-id
  Tunnel Service   active
  Email Routing    ON
  Salinan Email    OFF

  ─── Wrangler (Worker) ───
  Login            ✓ login
  Active Profile   wrangler-oauth
  Worker           ✓ deployed
  Profiles         1 profile
```

Menu:
- **1-7**: Manage CF profiles, domains, login, worker, routing, copies
- **8**: SETUP GUIDED (one-shot all steps)
- **9**: AUTO SETUP (step-by-step)
- **A**: Kelola admin (create/view/reset password)
- **L1-L4**: Logout/reset/delete worker
- **10**: Refresh

---

## 🌐 Multi-Domain Support

TempMail supports **multiple domains** in one instance. Each domain can be on the **same Cloudflare account** or **different accounts**.

### How domains are grouped

```text
Domain A (account CF-A):
  Email → x@domain-a.com → CF Routing (A) → Worker A → Tunnel → Server
  Web   → https://tempmail.domain-a.com/app/

Domain B (account CF-B):
  Email → y@domain-b.com → CF Routing (B) → Worker B → Tunnel → Server
  Web   → https://tempmail.domain-a.com/app/  ← web stays on A (recommended)
```

### Adding a second domain — SOLVED

When adding a **second domain**, you do **NOT** need to go through the full `menutempmail → 8` setup again. Most steps auto-skip. Here's the exact flow:

#### Case 1: Domain B on the SAME Cloudflare account

```bash
menutempmail → 2  (add domain B)
            → 6  (enable Email Routing for B)
            → 10 (refresh)
```
Only **3 steps**. No login, no tunnel, no worker re-deploy needed. Email Routing for B is enabled automatically; everything else already exists.

```text
== [2/7] Login Wrangler       == → Skip (already logged in)
== [3/7] Login CF Tunnel      == → Skip (cert.pem exists)
== [4/7] Buat tunnel + DNS    == → Skip (tunnel exists)
== [5/7] Deploy worker        == → Skip (worker deployed)
== [6/7] Email Routing (B)    == → ✅ NEW — enable catch-all *@domain-b.com
```

#### Case 2: Domain B on a DIFFERENT Cloudflare account

This requires a **separate Worker** (each CF account has its own worker). But the **Tunnel stays shared** — no new tunnel needed for email.

```bash
# Step 1 — add domain B to config
sudo nano /etc/tempmail/tempmail.env
#   DOMAINS=domain-a.com,domain-b.com
sudo systemctl restart tempmail

# Step 2 — login wrangler with account B (replaces token with acct B)
menutempmail → 3 (Login Wrangler)
#   open link → authorize with account B

# Step 3 — save account B as a CF profile
menutempmail → 1 (Kelola akun Cloudflare)
#   add profile → label "CF B" + account ID of B

# Step 4 — register domain B under profile B
menutempmail → 2 (Kelola domain)
#   add domain B → attach to profile "CF B"

# Step 5 — deploy worker to account B
menutempmail → 5 (Deploy worker)
#   select profile B → wrangler deploy worker in account B

# Step 6 — enable Email Routing for domain B
menutempmail → 6 (Aktifkan Email Routing)
#   select domain B → enable catch-all → worker in account B

# Step 7 — verify
menutempmail → 10 (Refresh)
```

### What you must NOT redo for domain B

| Component | Reuse? | Why |
|---|---|---|
| Cloudflare Tunnel | ✅ **Reuse** | One tunnel serves all domains' email to the server |
| `cloudflared tunnel login` | ❌ No | `cert.pem` already exists |
| Tunnel created | ❌ No | Existing tunnel is enough |
| Worker name | ✅ Same | `tempmailbyjhopanstore` (per account) |
| Web UI hostname | ⚠️ Optional | Only if you want a separate web URL for domain B |

### Email-only vs Web for domain B

**Email-only (recommended):** domain B just receives email, web stays on domain A.
```text
Email → x@domain-b.com → Worker B → Tunnel A → Server → inbox
Web   → tempmail.domain-a.com/app/   (unchanged)
```

**Web on domain B:** requires a new tunnel in account B + DNS route. Uncommon — only if you want a separate admin URL per domain.

### The KEY difference: Profile vs Login vs Worker

| Concept | What | Notes |
|---|---|---|
| **Login (Wrangler)** | Active OAuth token | 1 token = 1 account. Last login is active. |
| **Profile (CF A / CF B)** | Saved account data (label, account ID) | Multiple allowed, never expire |
| **Active profile** | Which profile is used for deploy/routing | Switch anytime |
| **Worker** | Per-account script `tempmailbyjhopanstore` | Deployed in host account |

Because Wrangler stores only **one** OAuth token at a time, switching accounts requires `menutempmail → 3` again. For permanent multi-account without re-login, **API Token per profile** is the way (see roadmap).

---

## 📦 Build & Release

Binaries built via **GitHub Actions** — no manual build needed.

```bash
# Trigger release
git tag v1.0.0
git push origin v1.0.0
```

GitHub Actions builds:
- `tempmail-linux-amd64` — Linux x86_64
- `tempmail-linux-arm64` — Linux ARM64
- `tempmail-windows-amd64.exe` — Windows x86_64

Download from: https://github.com/jhopan/TempMailByJhopanstore/releases

---

## 📁 Host Layout

```
/opt/tempmail/tempmail               Go binary
/opt/tempmail/worker/                Wrangler project
/opt/tempmail/data/tempmail.db       SQLite database
/etc/tempmail/tempmail.env            Config (PORT, DOMAINS, secrets)
/etc/tempmail/cloudflare-state.json   CF state
/etc/cloudflared/config.yml           Tunnel config
/usr/local/sbin/tempmailctl           Control panel
/usr/local/bin/menutempmail           Menu launcher
```

---

## ☁️ Cloudflare Setup

| Component | Value |
|---|---|
| Worker name | `tempmailbyjhopanstore` (fixed) |
| Tunnel name | `tempmailbyjhopanstore` (fixed) |
| wrangler.toml | `workers_dev=false`, `preview_urls=false` |
| Secrets | `TEMPMAIL_SERVER_URL`, `TEMPMAIL_API_KEY`, `FORWARD_LIST` |
| Tunnel config | `disableChunkedEncoding: true` (SSE support) |

---

## 🔧 Development

```bash
cd receiver
cp ../.env.example .env    # set JWT_SECRET, EMAIL_API_KEY, DOMAINS
go run .                   # http://127.0.0.1:8080/app/

go test ./...
go vet ./...
```

---

## 📚 Docs

- [`docs/API.md`](docs/API.md) — Full API documentation with examples
- [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) — Deployment guide

---

## 📄 License

MIT. Credit: **TempMail By JhopanStore**.
