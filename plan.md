# TempMailByJhopanStore

> Platform Temp Mail self-hosted dengan autentikasi, multi-domain, dan manajemen user.

---

## 📋 Daftar Isi

1. [Arsitektur Sistem](#arsitektur-sistem)
2. [Tech Stack](#tech-stack)
3. [Struktur Folder](#struktur-folder)
4. [Dependencies](#dependencies)
5. [Setup VPS](#setup-vps)
6. [Database Schema](#database-schema)
7. [Backend (Go Receiver)](#backend-go-receiver)
8. [Frontend (Next.js)](#frontend-nextjs)
9. [Mail Server (Maddy)](#mail-server-maddy)
10. [Web Server (Nginx)](#web-server-nginx)
11. [Authentication System](#authentication-system)
12. [Domain Management](#domain-management)
13. [Systemd Services](#systemd-services)
14. [Deployment Checklist](#deployment-checklist)
15. [Konfigurasi Environment](#konfigurasi-environment)

---

## 🏗️ Arsitektur Sistem

```
Internet
    │
    ▼
┌─────────────────────────────────────────────────┐
│                    Nginx (Port 80/443)           │
│  Reverse Proxy + SSE Support                     │
│                                                  │
│  /            → Next.js (Port 3000)             │
│  /api/inbox/* → Next.js API Route → Go Backend  │
│  /api/auth/*  → Next.js API Route → Go Backend  │
│  /api/admin/* → Next.js API Route → Go Backend  │
│  /api/sse/*   → Go Backend (SSE endpoint)       │
└─────────────────────────────────────────────────┘
         │                          │
         ▼                          ▼
┌─────────────────┐    ┌─────────────────────────┐
│  Next.js 16     │    │  Go Receiver (Port 8080) │
│  (Port 3000)    │    │  API + LMTP (Port 8024)  │
│                 │    │                           │
│  - Login Page   │    │  - Inbox API             │
│  - Inbox UI     │    │  - Message API           │
│  - Admin Panel  │    │  - Auth (JWT)            │
│  - API Routes   │    │  - SSE Real-time         │
│  - Cooldown     │    │  - Rate Limiting         │
│  - iframe email │    │  - UTF-8 Fallback Parser │
└─────────────────┘    └─────────────────────────┘
         │                          │
         └──────────┬───────────────┘
                    ▼
         ┌─────────────────────┐
         │  PostgreSQL          │
         │  Database: tempmail  │
         │                      │
         │  - inboxes           │
         │  - messages          │
         │  - attachments       │
         │  - users             │
         └─────────────────────┘
                    ▲
                    │
         ┌─────────────────────┐
         │  Maddy Mail Server   │
         │  SMTP (Port 25)      │
         │  IMAP (Port 143)     │
         │  LMTP → Go :8024     │
         └─────────────────────┘
```

---

## 🛠️ Tech Stack

| Komponen       | Teknologi                    | Versi    |
|----------------|------------------------------|----------|
| Backend        | Go (Gin Framework)           | 1.25+    |
| Frontend       | Next.js (App Router)         | 16.2.9   |
| UI             | Tailwind CSS                 | 4.x      |
| Mail Server    | Maddy                        | Latest   |
| Database       | PostgreSQL                   | 14+      |
| Web Server     | Nginx                        | 1.24+    |
| Auth           | JWT (golang-jwt/v5)          | 5.x      |
| Password Hash  | bcrypt (x/crypto)            | Latest   |
| Rate Limiting  | ulule/limiter                | 3.x      |
| MIME Parser    | go-message + charset         | Latest   |
| Sanitizer      | DOMPurify (frontend)         | 3.x      |
| DNS Management | Cloudflare API               | v4       |
| OS             | Ubuntu 22.04/24.04 LTS      | -        |

---

## 📁 Struktur Folder

```
/opt/TempMailByJhopanStore/
├── plan.md                          # File ini
├── README.md                        # Dokumentasi project
├── .env.example                     # Template environment variables
│
├── receiver/                        # Go Backend
│   ├── go.mod
│   ├── go.sum
│   ├── main.go                      # Entry point
│   ├── .env                         # Environment variables
│   ├── api/
│   │   └── router.go                # Routing, middleware, SSE, rate limiting
│   ├── lmtp/
│   │   └── server.go                # LMTP server (terima email dari Maddy)
│   ├── handlers/
│   │   ├── inbox_handler.go         # GET/DELETE inbox
│   │   ├── message_handler.go       # GET/DELETE message + attachments
│   │   └── delete.go                # Delete handlers
│   ├── handler/
│   │   └── auth_handler.go          # Login, register, user CRUD (admin)
│   ├── middleware/
│   │   └── auth.go                  # JWT middleware + admin check
│   ├── models/
│   │   └── user.go                  # User model
│   ├── repository/
│   │   ├── delete.go                # DB delete operations
│   │   └── user_repository.go       # DB user operations
│   ├── database/
│   │   └── database.go              # DB connection
│   ├── parser/
│   │   └── mime_parser.go           # MIME parser + UTF-8 fallback
│   └── receiver-final               # Compiled binary
│
├── frontend/                        # Next.js Frontend
│   ├── package.json
│   ├── next.config.js
│   ├── tailwind.config.ts
│   ├── tsconfig.json
│   ├── src/
│   │   ├── app/
│   │   │   ├── layout.tsx           # Root layout + AuthProvider
│   │   │   ├── page.tsx             # Main inbox page
│   │   │   ├── login/
│   │   │   │   └── page.tsx         # Login page
│   │   │   ├── admin/
│   │   │   │   └── page.tsx         # Admin dashboard
│   │   │   └── api/
│   │   │       ├── health/
│   │   │       │   └── route.ts     # Health check
│   │   │       ├── inbox/
│   │   │       │   └── [email]/
│   │   │       │       └── route.ts # Inbox proxy
│   │   │       └── message/
│   │   │           └── [id]/
│   │   │               ├── route.ts           # Message proxy
│   │   │               └── attachments/
│   │   │                   └── route.ts       # Attachments proxy
│   │   └── context/
│   │       └── AuthContext.tsx      # Auth state management
│   └── .next/                       # Build output
│
├── scripts/
│   ├── add-domain.sh               # Add domain + Cloudflare DNS
│   ├── maddy-add-domain.py         # Helper: update Maddy config
│   ├── setup.sh                    # Initial VPS setup script
│   ├── backup.sh                   # Database backup script
│   └── deploy.sh                   # Build & deploy script
│
├── config/
│   ├── nginx/
│   │   └── tempmail.conf           # Nginx config
│   ├── maddy/
│   │   └── maddy.conf              # Maddy config
│   └── systemd/
│       ├── tempmail-receiver.service
│       ├── tempmail-frontend.service
│       └── maddy.service
│
└── docs/
    ├── API.md                       # API documentation
    ├── DEPLOYMENT.md                # Deployment guide
    └── TROUBLESHOOTING.md           # Common issues & fixes
```

---

## 📦 Dependencies

### System Dependencies (Ubuntu)

```bash
# Basic
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl wget git build-essential software-properties-common

# PostgreSQL
sudo apt install -y postgresql postgresql-contrib

# Nginx
sudo apt install -y nginx

# Go 1.25+
wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc

# Node.js 20+
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs

# Maddy Mail Server
# (Download dari GitHub releases)
wget https://github.com/foxcpp/maddy/releases/latest/download/maddy-linux-x86_64.tar.gz
tar xzf maddy-linux-x86_64.tar.gz
sudo cp maddy /usr/local/bin/

# Python 3 + bcrypt (untuk password hashing & helper scripts)
sudo apt install -y python3 python3-pip
pip3 install bcrypt
```

### Go Dependencies

```go
// go.mod
require (
    github.com/gin-gonic/gin v1.x
    github.com/golang-jwt/jwt/v5 v5.x
    golang.org/x/crypto v0.x
    golang.org/x/text v0.x
    github.com/ulule/limiter/v3 v3.x
    github.com/lib/pq v1.x
    github.com/emersion/go-message v0.x
    github.com/joho/godotenv v1.x
)
```

### Node.js Dependencies

```json
{
  "dependencies": {
    "next": "16.2.9",
    "react": "19.x",
    "react-dom": "19.x",
    "dompurify": "3.x",
    "isomorphic-dompurify": "2.x"
  },
  "devDependencies": {
    "typescript": "5.x",
    "tailwindcss": "4.x",
    "@tailwindcss/postcss": "4.x",
    "@types/dompurify": "3.x",
    "@types/react": "19.x",
    "@types/node": "22.x"
  }
}
```

---

## 🖥️ Setup VPS

### 1. Initial Setup

```bash
# Update system
sudo apt update && sudo apt upgrade -y

# Install all dependencies (lihat bagian Dependencies)

# Create project directory
sudo mkdir -p /opt/TempMailByJhopanStore
sudo chown $USER:$USER /opt/TempMailByJhopanStore

# Clone/copy project files
cd /opt/TempMailByJhopanStore
```

### 2. Database Setup

```bash
# Create PostgreSQL database
sudo -u postgres psql -c "CREATE DATABASE tempmail;"
sudo -u postgres psql -c "CREATE USER tempmail WITH PASSWORD '***';"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE tempmail TO tempmail;"

# Create tables
sudo -u postgres psql -d tempmail -c "
CREATE TABLE IF NOT EXISTS inboxes (
    id SERIAL PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    owner_id INTEGER,
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS messages (
    id SERIAL PRIMARY KEY,
    inbox_id INTEGER REFERENCES inboxes(id),
    sender TEXT,
    subject TEXT,
    html_body TEXT,
    text_body TEXT,
    received_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS attachments (
    id SERIAL PRIMARY KEY,
    message_id INTEGER REFERENCES messages(id),
    filename TEXT,
    content_type TEXT,
    size INTEGER,
    data BYTEA
);

CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
"

# Create admin user
python3 -c "
import bcrypt
password = 'Jh0pan@St0re'
hashed = bcrypt.hashpw(password.encode(), bcrypt.gensalt()).decode()
import subprocess
subprocess.run(['sudo', '-u', 'postgres', 'psql', '-d', 'tempmail', '-c',
    f\"INSERT INTO users (email, password_hash, role) VALUES ('jhopanstore@gmail.com', '{hashed}', 'admin') ON CONFLICT DO NOTHING;\"])
"
```

### 3. SSL/TLS (Optional - Let's Encrypt)

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d yourdomain.com
```

---

## 🗄️ Database Schema

### Table: inboxes
| Column     | Type      | Description                    |
|------------|-----------|--------------------------------|
| id         | SERIAL PK | Auto-increment ID              |
| email      | TEXT      | Full email address (unique)    |
| type       | TEXT      | Inbox type                     |
| owner_id   | INTEGER   | Owner user ID (nullable)       |
| expires_at | TIMESTAMP | Expiration time (nullable)     |
| created_at | TIMESTAMP | Creation timestamp             |

### Table: messages
| Column      | Type      | Description                    |
|-------------|-----------|--------------------------------|
| id          | SERIAL PK | Auto-increment ID              |
| inbox_id    | INTEGER   | FK → inboxes.id               |
| sender      | TEXT      | Sender email address           |
| subject     | TEXT      | Email subject                  |
| html_body   | TEXT      | HTML content                   |
| text_body   | TEXT      | Plain text content             |
| received_at | TIMESTAMP | Receive timestamp              |

### Table: attachments
| Column       | Type      | Description                    |
|--------------|-----------|--------------------------------|
| id           | SERIAL PK | Auto-increment ID              |
| message_id   | INTEGER   | FK → messages.id              |
| filename     | TEXT      | Original filename              |
| content_type | TEXT      | MIME type                      |
| size         | INTEGER   | File size in bytes             |
| data         | BYTEA     | Binary file data               |

### Table: users
| Column        | Type         | Description                    |
|---------------|--------------|--------------------------------|
| id            | SERIAL PK    | Auto-increment ID              |
| email         | VARCHAR(255) | User email (unique)            |
| password_hash | VARCHAR(255) | Bcrypt hashed password         |
| role          | VARCHAR(50)  | Role: 'admin' or 'user'        |
| created_at    | TIMESTAMP    | Account creation               |
| updated_at    | TIMESTAMP    | Last update                    |

---

## ⚙️ Backend (Go Receiver)

### Features
- **LMTP Server** (Port 8024) — Terima email dari Maddy
- **REST API** (Port 8080) — Inbox, message, auth endpoints
- **SSE** — Real-time inbox updates
- **JWT Auth** — Token-based authentication (24h expiry)
- **Rate Limiting** — 60 req/min (API), 20 req/min (DELETE)
- **UTF-8 Fallback** — Auto-convert non-UTF-8 emails
- **Cleanup Worker** — Auto-delete inbox > 24 jam

### API Endpoints

```
# Public
POST   /api/auth/login              # Login (returns JWT)

# Protected (JWT required)
GET    /api/auth/me                 # Current user info

# Admin only
GET    /api/admin/users             # List all users
POST   /api/admin/users             # Create user
PUT    /api/admin/users/:id         # Update user
DELETE /api/admin/users/:id         # Delete user
PUT    /api/admin/users/:id/password # Reset password

# Inbox (via Next.js proxy)
GET    /inbox/:email                # Get inbox messages
DELETE /inbox/:email                # Delete all messages

# Message (via Next.js proxy)
GET    /message/:id                 # Get message detail
DELETE /message/:id                 # Delete message
GET    /message/:id/attachments     # Get attachments

# SSE
GET    /sse/:email                  # Server-Sent Events stream
```

### Environment Variables (receiver/.env)

```env
DB_HOST=localhost
DB_PORT=5432
DB_USER=tempmail
DB_PASSWORD=***
DB_NAME=tempmail
JWT_SECRET=your-super-secret-jwt-key-change-this
```

---

## 🎨 Frontend (Next.js)

### Features
- **Login Page** — Email + password authentication
- **Main Inbox** — Real-time email inbox with SSE
- **Admin Dashboard** — User management (CRUD)
- **Email Rendering** — iframe sandbox for safe HTML display
- **Multi-Domain** — Dropdown domain selector
- **Cooldown Timer** — 60s cooldown between email changes
- **Auth Guard** — Redirect to login if not authenticated
- **Copy to Clipboard** — One-click email copy
- **Delete** — Individual message and bulk inbox delete

### Pages

| Route      | Description                    | Auth Required | Role     |
|------------|--------------------------------|---------------|----------|
| `/`        | Main inbox page                | ✅ Yes        | Any      |
| `/login`   | Login form                     | ❌ No         | -        |
| `/admin`   | User management dashboard      | ✅ Yes        | Admin    |

### API Routes (Next.js Proxy)

| Route                          | Proxied To                      |
|--------------------------------|---------------------------------|
| `/api/inbox/[email]`          | `http://127.0.0.1:8080/inbox/*` |
| `/api/message/[id]`           | `http://127.0.0.1:8080/message/*` |
| `/api/message/[id]/attachments` | `http://127.0.0.1:8080/message/*/attachments` |
| `/api/health`                  | Health check                    |

---

## 📬 Mail Server (Maddy)

### Configuration

```
# /etc/maddy/maddy.conf (key parts)

# Accept mail for these domains
$(local_domains) = renunganbot.qzz.io

# Disable strict checks for temp mail
msgmodifier.local_checks {
    check_spf off
    check_dmarc off
    require_mx_record off
}

# SMTP listener
smtp tcp://0.0.0.0:25 {
    ...
    deliver_to lmtp tcp://127.0.0.1:8024
}
```

### Adding New Domain

```bash
sudo bash /opt/TempMailByJhopanStore/scripts/add-domain.sh newdomain.com
```

---

## 🌐 Web Server (Nginx)

### Configuration

```nginx
server {
    listen 80;
    server_name yourdomain.com;

    # SSE endpoint (direct to backend)
    location /api/sse/ {
        proxy_pass http://localhost:8080/sse/;
        proxy_http_version 1.1;
        proxy_set_header Connection '';
        proxy_buffering off;
        proxy_cache off;
        chunked_transfer_encoding on;
        proxy_read_timeout 3600s;
    }

    # Frontend (all other requests)
    location / {
        proxy_pass http://localhost:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
}
```

---

## 🔐 Authentication System

### Flow

```
1. User POST /api/auth/login {email, password}
2. Backend: verify bcrypt hash → generate JWT (24h)
3. Frontend: store token in localStorage
4. All protected requests: Authorization: Bearer <token>
5. Backend middleware: verify JWT, check role
6. Admin routes: require role = 'admin'
```

### Admin Default Account

| Field    | Value                    |
|----------|--------------------------|
| Email    | jhopanstore@gmail.com    |
| Password | Jh0pan@St0re             |
| Role     | admin                    |

### Features
- ✅ JWT token (24h expiry)
- ✅ Bcrypt password hashing
- ✅ Role-based access (admin/user)
- ✅ No public registration
- ✅ Admin can create/edit/delete users
- ✅ Admin can reset passwords
- ✅ Can't delete own account

---

## 🌍 Domain Management

### Script: add-domain.sh

**Features:**
- Cloudflare API integration
- Auto-create DNS records (MX, A, SPF, DMARC)
- Update Maddy config
- Update frontend domain list
- Restart services

**Usage:**
```bash
sudo bash /opt/TempMailByJhopanStore/scripts/add-domain.sh newdomain.com
```

**Requirements:**
- Cloudflare API Token (with Zone:Edit permission)
- Cloudflare Account ID

---

## 🔧 Systemd Services

### tempmail-receiver.service

```ini
[Unit]
Description=Temp Mail Go Backend
After=network.target postgresql.service

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/opt/TempMailByJhopanStore/receiver
EnvironmentFile=/opt/TempMailByJhopanStore/receiver/.env
ExecStart=/opt/TempMailByJhopanStore/receiver/receiver-final
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### tempmail-frontend.service

```ini
[Unit]
Description=Temp Mail Frontend (Next.js)
After=network.target

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/opt/TempMailByJhopanStore/frontend
ExecStart=/usr/bin/npx next start -p 3000
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

---

## ✅ Deployment Checklist

- [ ] VPS provisioned (min 1GB RAM, 1 CPU)
- [ ] Domain pointed to VPS IP
- [ ] DNS MX record configured
- [ ] System dependencies installed
- [ ] Go 1.25+ installed
- [ ] Node.js 20+ installed
- [ ] PostgreSQL running + database created
- [ ] Maddy installed + configured
- [ ] Nginx installed + configured
- [ ] Backend compiled + systemd service active
- [ ] Frontend built + systemd service active
- [ ] Admin account created
- [ ] SSL certificate (optional)
- [ ] Firewall configured (ports 25, 80, 443)
- [ ] Backup script configured (cron)
- [ ] Tested: send email → appears in inbox
- [ ] Tested: login → access inbox
- [ ] Tested: admin → create/delete users

---

## 🔧 Konfigurasi Environment

### Firewall

```bash
sudo ufw allow 22/tcp    # SSH
sudo ufw allow 25/tcp    # SMTP
sudo ufw allow 80/tcp    # HTTP
sudo ufw allow 443/tcp   # HTTPS
sudo ufw enable
```

### Cron Jobs

```bash
# Daily backup at 2 AM
0 2 * * * /opt/TempMailByJhopanStore/scripts/backup.sh

# Cleanup old inboxes (handled by Go cleanup worker, but fallback)
0 3 * * * sudo -u postgres psql -d tempmail -c "DELETE FROM inboxes WHERE created_at < NOW() - INTERVAL '24 hours' CASCADE;"
```

---

## 📝 Notes & Lessons Learned

1. **Nginx rewrites vs Next.js API routes**: Jangan gunakan `rewrites()` di `next.config.js` jika sudah ada API route handler — rewrites akan override API routes dan menyebabkan 404.

2. **URL encoding**: `encodeURIComponent(email)` mengubah `@` menjadi `%40`. Backend (Echo/Gin) biasanya auto-decode, tapi perlu hati-hati dengan proxy chain.

3. **PostgreSQL UTF-8**: PostgreSQL menolak byte non-UTF-8. Email dari luar sering pakai encoding lain (ISO-8859-1, Windows-1252). Solusi: fallback parser di Go (`golang.org/x/text/encoding`).

4. **Maddy strict checks**: Untuk temp mail, matikan semua strict checks (SPF, DMARC, DKIM, require_mx_record) karena harus menerima email dari sumber manapun.

5. **iframe sandbox**: Gunakan `<iframe sandbox>` untuk render HTML email agar CSS email tidak bocor ke UI utama.

6. **SSH secret redaction**: Platform agent auto-redact variabel sensitif. Gunakan placeholder string saat transfer file, lalu replace di server.

---

*Last updated: 2026-06-12*
*Author: TempMailByJhopanStore*
