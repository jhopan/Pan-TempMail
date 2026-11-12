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

  ─── Cloudflare ───
  Profil Aktif     cf-main (token aktif)
  Profil          1 profil
  Domain          your-domain.com (routing ON)
  Worker          ✓ deployed (per profil)
  Tunnel          tempmailbyjhopanstore / 7e95f66b… / credentials-file
  Konektor        cloudflared active
  Email Routing   ON
  Salinan Email   OFF
```

Menu:
- **1**: Kelola akun Cloudflare (profil + token per akun)
- **2**: Kelola domain TempMail
- **3**: Tempel API token profil (wajib untuk panel)
- **4**: Setup tunnel Cloudflare (lewat API token)
- **5**: Deploy/update Worker
- **6**: Aktifkan Email Routing catch-all
- **7**: Atur tujuan salinan email
- **8**: SETUP GUIDED (satu jalan semua langkah)
- **9**: AUTO SETUP (bertahap)
- **A**: Kelola admin (buat/cek/reset password)
- **D**: Doctor (cek semua status)
- **L2-L4**: Logout tunnel / reset credential Cloudflare / hapus Worker
- **10**: Refresh

Tidak ada `wrangler login` dan tidak ada `cloudflared tunnel login` di menu mana pun: akses Cloudflare
memakai satu API token per akun, dan konektor tunnel dipasang dari token konektor yang dikembalikan
API. Login yang tersisa hanya login ke aplikasi ini (email + password admin).

---

## 🧭 Admin panel (web)

Open `https://<web-host>/admin` (or `/app/admin.html`) and log in as an admin. The panel is not linked
from the inbox UI on purpose: an admin types `/admin`, and every request still needs an admin JWT, so a
regular user who finds the URL only sees a "not an admin account" notice.

The panel uses a sidebar with one section at a time (deep links work, e.g. `/admin#server`):
**Status**, **Server**, **Akun Cloudflare**, **Domain**, **Tunnel**, **Salinan email**, **Impor state**.

Everything that used to need SSH plus `menutempmail` is available there, and it writes to the same
records the menu uses (SQLite is the single source of truth):

| Section | What it does |
|---|---|
| Cloudflare profiles | Add an account (ID, label, Account ID, API token), activate, delete, deploy/delete the Worker |
| Domains | Add a domain (zone resolved with the profile token), enable/disable routing, unlock Cloudflare DNS records, delete from panel only |
| Tunnel | Show tunnel name, ID, status, live connector count, web host; save web host/name; create a tunnel and get the connector token plus the root commands to install it |
| Copies | Comma separated copy destinations (applied on the next Worker deploy) |
| Import | Paste an old `cloudflare-state.json` to load existing profiles and domains once |
| Server | Service status, restart service, backup now, journald logs, doctor checks, and tunnel install/restart/logout through the root helper |

Notes:

- API tokens are stored encrypted (AES-256-GCM, key derived from `JWT_SECRET`) and are never returned by the API.
- Adding a domain takes effect immediately: the TempMail domain list comes from the database, so no service restart.
- Cloudflare errors are shown verbatim (rejected token, zone not active, non-Cloudflare MX), instead of failing silently.
- The panel never installs or restarts `cloudflared` itself. The app runs as the `tempmail` user, so those actions go through the root helper (`/usr/local/sbin/tempmail-root`, systemd `tempmail-root.path` + `.timer`): the app writes a validated request to a spool directory, the root helper executes an allowlisted action and writes the result back. No `sudoers`, no root for the app. Without the helper, the panel still works and says `helper belum dipasang`.
- `menutempmail → 1/2/5/6/7` call the same API, so menu and panel never disagree.

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
== [1/7] Periksa prasyarat      == → cloudflared, sqlite3, python3, health app
== [2/7] Kredensial Cloudflare  == → API token (prompt / env / --api-token); tidak ada login browser
== [3/7] Login ke aplikasi      == → email + password admin aplikasi ini (bukan Cloudflare)
== [4/7] Profil Cloudflare      == → profil + token disimpan terenkripsi, langsung diaktifkan
== [5/7] Tunnel + konektor      == → tunnel dibuat lewat API, konektor dipasang dari token konektor
== [6/7] Domain + Email Routing == → domain + catch-all *@domain-b.com
== [7/7] Deploy Worker + verifikasi == → Worker diunggah lewat API, lalu /health dicek
```

#### Case 2: Domain B on a DIFFERENT Cloudflare account

Each account gets its own profile and its own Worker; the **Tunnel stays shared**, so no new tunnel is needed for email.

Panel flow (`/app/admin.html`):

```text
1. Create an API token in the Cloudflare account that owns domain B.
   Required: Zone:Read, Zone Settings:Edit, DNS:Edit, Email Routing Rules:Edit, Workers Scripts:Edit
   Optional: Cloudflare Tunnel:Edit (manage/read tunnels), Account Settings:Read
   Zone resources: All zones in that account.
2. Admin panel -> Cloudflare profiles -> add profile (id cf-b, Account ID B, the token), tick Activate.
3. Admin panel -> Domains -> add domain B. The zone is looked up with profile B's token.
4. Admin panel -> Cloudflare profiles -> Deploy Worker on profile cf-b.
5. Admin panel -> Domains -> Enable routing for domain B.
```

Menu equivalent: `menutempmail → 1` (add + select profile B) `→ 2` (add domain B) `→ 5` (deploy Worker for profile B) `→ 6` (enable routing for domain B).

No `wrangler` login, no `DOMAINS=` editing, and no service restart: profile B's token drives the deploy and the routing call, and the panel updates the domain list.

### What you must NOT redo for domain B

| Component | Reuse? | Why |
|---|---|---|
| Cloudflare Tunnel | ✅ **Reuse** | One tunnel serves all domains' email to the server |
| `cloudflared tunnel login` | ❌ No | `cert.pem` already exists, and re-login would replace account A's credential |
| Tunnel created | ❌ No | Existing tunnel is enough |
| Worker name | ✅ Same | `tempmailbyjhopanstore` (deployed per account) |
| Web UI hostname | ⚠️ Optional | Only if you want a separate web URL for domain B |

### Email-only vs Web for domain B

**Email-only (recommended):** domain B just receives email, web stays on domain A.
```text
Email → x@domain-b.com → Worker B → Tunnel A → Server → inbox
Web   → tempmail.domain-a.com/app/   (unchanged)
```

**Web on domain B:** requires a new tunnel in account B + its own DNS route. Uncommon, only if you want a separate admin URL per domain.

### The KEY difference: Profile vs Login vs Worker

| Concept | What | Notes |
|---|---|---|
| **Profile (CF A / CF B)** | Saved account data: label, Account ID, encrypted API token | Multiple allowed, never expire |
| **Active profile** | Which profile the panel uses by default | Switch anytime from the panel or menu |
| **Worker** | Per-account script `tempmailbyjhopanstore` | Deployed through the Cloudflare API, no `wrangler` needed |
| **Wrangler login** | OAuth device login from the old pre-API-token flow | Not used at all now: no script or menu option runs it. If `/root/.wrangler/config/default.toml` still exists on a host, it is dead weight from the old setup. |
| **Tunnel login (cert.pem)** | Browser login from the old flow that writes `/root/.cloudflared/cert.pem` | Not used by the current scripts. The running connector may still rely on it (`mode: credentials-file` in the panel); `tunnel-logout` deletes it, so use `tunnel-install` with a connector token if you ever need it back. |

Each profile carries its own API token, so switching accounts is a profile switch, never a re-login.

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
