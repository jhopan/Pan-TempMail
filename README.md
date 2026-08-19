# 📧 TempMail Ultra-Light

<div align="center">

**Single-binary disposable email platform — ~24MB binary, ~35MB RAM**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

</div>

---

## 🌟 Overview

TempMail Ultra-Light is a production-ready temporary email platform in a **single Go binary**. No Node.js, no PostgreSQL, no Maddy, no Nginx — just one file.

**Key Highlights:**
- 📦 **Single binary** (~24MB) — everything embedded
- 💾 **SQLite embedded** — zero database config
- 📨 **Built-in SMTP** — no external mail server
- 🎨 **Embedded frontend** — no Node.js runtime
- ⚡ **~35MB RAM** — runs on the cheapest VPS
- 🤖 **Automation API** — create inboxes, wait for emails, extract OTPs
- 🔒 **Secure by default** — no hardcoded secrets, fail-closed auth

---

## 🚀 Quick Start

### Option 1: Docker (Recommended)

```bash
# 1. Generate JWT secret
echo "JWT_SECRET=$(openssl rand -hex 32)" > .env
echo "DOMAINS=mail.example.com" >> .env

# 2. Build and run
docker compose up -d --build

# 3. Check health
curl http://localhost:8080/health
```

### Option 2: Binary directly

```bash
# 1. Build
cd receiver
go build -ldflags="-s -w" -o tempmail .

# 2. Configure
cp ../.env.example .env
# Edit .env — set JWT_SECRET and DOMAINS

# 3. Run
./tempmail
```

### Option 3: From source

```bash
git clone https://github.com/jhopan/TempMailByJhopanstore.git
cd TempMailByJhopanstore
cp .env.example .env
# Edit .env — set JWT_SECRET and DOMAINS
cd receiver && go run .
```

---

## 📊 Resource Usage

| Component | Old Version | Ultra-Light |
|-----------|-------------|-------------|
| Binary size | N/A (multi-process) | **24MB** |
| RAM (idle) | ~400-500MB | **~35MB** |
| RAM (active) | ~600MB+ | **~50MB** |
| Processes | 4 (Maddy, Go, Next.js, Nginx) | **1** |
| External deps | PostgreSQL, Node.js, Maddy, Nginx | **None** |
| Docker image | N/A | **~25MB** (Alpine) |

---

## 🏗️ Architecture

```
┌──────────────────────────────────────────┐
│         Single Go Binary (~24MB)          │
│                                          │
│  ┌──────────┐  ┌────────┐  ┌─────────┐ │
│  │ SMTP     │  │ HTTP   │  │ Static  │ │
│  │ Port 25  │  │ API    │  │ Frontend│ │
│  │          │  │ Port   │  │ (embed) │ │
│  │ built-in │  │ 8080   │  │         │ │
│  └────┬─────┘  └───┬────┘  └────┬────┘ │
│       │            │            │       │
│       ▼            ▼            ▼       │
│  ┌────────────────────────────────────┐ │
│  │     SQLite (embedded file)         │ │
│  │     data/tempmail.db               │ │
│  └────────────────────────────────────┘ │
└──────────────────────────────────────────┘
```

---

## 🤖 Automation API

### Create an inbox

```bash
curl -X POST https://your-instance/api/inbox \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username": "test123", "domain": "mail.example.com"}'
```

### Wait for an email (blocking — no polling!)

```bash
# Blocks until email arrives, up to 60 seconds
curl "https://your-instance/api/inbox/test123@mail.example.com/wait?timeout=60" \
  -H "Authorization: Bearer $TOKEN"
```

Response includes **auto-extracted OTP codes** and **verification links**:

```json
{
  "id": 42,
  "from": "noreply@example.com",
  "subject": "Your verification code",
  "codes": ["483920"],
  "links": ["https://example.com/verify?token=abc123"]
}
```

### Full automation example

```bash
# 1. Login
TOKEN=$(curl -s -X POST https://your-instance/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"yourpass"}' | jq -r .token)

# 2. Create inbox
ADDR=$(curl -s -X POST https://your-instance/api/inbox \
  -H "Authorization: Bearer $TOKEN" \
  -d '{}' | jq -r .email)

# 3. ... trigger signup that sends email to $ADDR ...

# 4. Wait for email and extract OTP
OTP=$(curl -s "https://your-instance/api/inbox/$ADDR/wait?timeout=60" \
  -H "Authorization: Bearer $TOKEN" | jq -r .codes[0])

echo "OTP: $OTP"
```

---

## 📖 API Endpoints

### Authentication
```
POST   /api/auth/login              # Login (returns JWT)
GET    /api/auth/me                 # Current user info
```

### Inbox
```
POST   /api/inbox                   # Create inbox (automation)
GET    /api/inbox/:email            # Get messages (paginated)
DELETE /api/inbox/:email            # Delete inbox + messages
GET    /api/inbox/:email/wait       # Wait for message (blocking)
```

### Messages
```
GET    /api/message/:id             # Get message detail + extracted codes/links
DELETE /api/message/:id             # Delete message
GET    /api/message/:id/attachments # List attachments
GET    /api/attachment/:id          # Download attachment
```

### Real-time
```
GET    /sse/:email?token=JWT        # Server-Sent Events stream
```

### Admin
```
GET    /api/admin/users             # List users
POST   /api/admin/users             # Create user
PUT    /api/admin/users/:id         # Update user
DELETE /api/admin/users/:id         # Delete user
PUT    /api/admin/users/:id/password # Reset password
```

### Public
```
GET    /health                      # Health check
GET    /api/config                  # Public config (domains, site name)
```

---

## 🔧 Configuration

All config via environment variables (`.env` file supported):

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_DRIVER` | `sqlite` | `sqlite` or `postgres` |
| `DB_PATH` | `data/tempmail.db` | SQLite file path |
| `JWT_SECRET` | **required** | Generate with `openssl rand -hex 32` |
| `DOMAINS` | `example.com` | Comma-separated mail domains |
| `SMTP_PORT` | `25` | SMTP listen port |
| `PORT` | `8080` | HTTP API port |
| `CORS_ORIGINS` | empty | Comma-separated allowed origins |
| `CLEANUP_INTERVAL` | `1h` | How often to purge old data |
| `MESSAGE_RETENTION` | `24h` | How long to keep messages |
| `WEBHOOK_URL` | empty | URL to POST to on email receipt |
| `ATTACHMENTS_DIR` | `data/attachments` | Attachment storage path |

---

## 🚀 Deployment

### VPS (minimum 512MB RAM)

```bash
# Build on server
git clone https://github.com/jhopan/TempMailByJhopanstore.git
cd TempMailByJhopanstore
docker compose up -d --build
```

### Oracle Cloud Free Tier

```bash
# ARM instance (4 core, 24GB RAM) — way more than enough
docker compose up -d --build
```

### Requirements

- **Port 25** must be unblocked (for SMTP reception)
- **Domain** with MX record pointing to your server
- **Minimum**: 512MB RAM, 1 CPU, 10GB disk

---

## 🔒 Security

- ✅ JWT auth with no fallback secret (fail-closed)
- ✅ Bcrypt password hashing
- ✅ Rate limiting (login 5/min, API 60/min, delete 20/min)
- ✅ Role-based access (admin/user)
- ✅ Sandboxed email HTML rendering
- ✅ CORS from environment variable
- ✅ No hardcoded credentials
- ✅ Graceful shutdown
- ✅ SQL injection prevention (parameterized queries)

---

## 🛠️ Tech Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go 1.25+ (Gin framework) |
| Database | SQLite (embedded) or PostgreSQL (optional) |
| SMTP | Built-in (emersion/go-smtp) |
| Frontend | Vanilla HTML + JS + Tailwind CSS (CDN) |
| Auth | JWT (golang-jwt/v5) |
| Container | Docker (Alpine, multi-stage) |

---

## 📁 Project Structure

```
TempMailByJhopanStore/
├── receiver/              # Go Backend (single binary)
│   ├── main.go           # Entry point + graceful shutdown
│   ├── api/              # HTTP routes & SSE
│   ├── handler/          # Auth handlers
│   ├── handlers/         # API handlers (inbox, message, etc.)
│   ├── middleware/       # JWT & admin middleware
│   ├── models/           # Data models
│   ├── repository/       # Database operations
│   ├── database/         # DB connection (SQLite/PostgreSQL)
│   ├── cleanup/          # Auto-purge expired emails
│   ├── smtp/             # Built-in SMTP server
│   ├── parser/           # Email parsing
│   └── web/              # Embedded static frontend
│       └── static/       # HTML, JS, CSS files
├── Dockerfile            # Multi-stage build (~25MB image)
├── docker-compose.yml    # Single-container deployment
└── .env.example          # Environment template
```

---

## 🐛 Troubleshooting

### Email not arriving

```bash
# Check if SMTP is listening
netstat -tlnp | grep :25

# Check logs
docker compose logs -f tempmail
```

### Can't login

```bash
# Check if JWT_SECRET is set
grep JWT_SECRET .env

# Generate one if missing
echo "JWT_SECRET=$(openssl rand -hex 32)" >> .env
```

### Port 25 blocked

Many VPS providers block port 25. Check:
```bash
timeout 5 bash -c 'cat < /dev/null > /dev/tcp/gmail-smtp-in.l.google.com/25' \
  && echo "port 25 open" || echo "port 25 blocked"
```

---

**Made with ❤️ by JhopanStore**
