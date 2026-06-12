# 📧 TempMailByJhopanStore

<div align="center">

**Self-hosted Temporary Email Platform with Authentication & Multi-Domain Support**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Next.js](https://img.shields.io/badge/Next.js-16-black?style=flat&logo=next.js)](https://nextjs.org/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

[Features](#-features) • [Installation](#-quick-start) • [Documentation](#-documentation) • [Contributing](#-contributing)

</div>

---

## 🌟 Overview

TempMailByJhopanStore is a production-ready temporary email platform designed for self-hosting. It provides secure, disposable email addresses with real-time inbox updates, user authentication, and multi-domain support.

**Key Highlights:**
- 🔐 **JWT Authentication** with role-based access control (admin/user)
- ⚡ **Real-time Updates** via Server-Sent Events (SSE)
- 🌍 **Multi-Domain Support** with automatic Cloudflare DNS configuration
- 🎨 **Modern UI** built with Next.js 16 and Tailwind CSS
- 🔒 **Secure Email Rendering** using sandboxed iframes
- 📱 **Responsive Design** works on desktop and mobile

---

## ✨ Features

### For Users
- ✅ Instant email generation with custom domains
- ✅ Real-time inbox updates (no refresh needed)
- ✅ View HTML emails in secure sandbox
- ✅ Download attachments
- ✅ 60-second cooldown protection
- ✅ Copy email address with one click

### For Admins
- ✅ Complete user management (CRUD operations)
- ✅ Role-based access control (admin/user)
- ✅ Password reset functionality
- ✅ Multi-domain configuration
- ✅ Database backup tools
- ✅ Service monitoring dashboard

### For Developers
- ✅ RESTful API with JWT authentication
- ✅ Rate limiting (60 req/min API, 20 req/min DELETE)
- ✅ UTF-8 fallback for non-standard email encodings
- ✅ Automatic cleanup of old emails (24 hours)
- ✅ Comprehensive logging
- ✅ Easy deployment scripts

---

## 🚀 Quick Start

### Prerequisites

- Ubuntu 22.04/24.04 LTS (fresh install recommended)
- Root/sudo access
- Domain name (with Cloudflare DNS recommended)
- Minimum: 1GB RAM, 1 CPU, 10GB disk

### Installation

**1. Clone the repository**

```bash
cd /opt
sudo git clone https://github.com/jhopan/TempMailByJhopanstore.git
cd TempMailByJhopanstore
sudo chown -R $USER:$USER .
```

**2. Run setup script**

```bash
sudo bash scripts/setup.sh
```

The setup will:
- ✅ Install all dependencies (Go, Node.js, PostgreSQL, Maddy, Nginx)
- ✅ Configure database and create admin account
- ✅ Setup mail server (Maddy)
- ✅ Configure reverse proxy (Nginx)
- ✅ Create systemd services
- ✅ Configure firewall (UFW)

**You'll be prompted for:**
- Database password
- Server IP address
- Primary domain (e.g., `mail.example.com`)
- Cloudflare API credentials (optional)

**3. Build and start services**

```bash
sudo bash scripts/start.sh
# Select: 1) Build & Start (full)
```

**4. Access the application**

Open your browser: `http://your-server-ip`

**Default Admin Credentials:**
```
Email:    jhopanstore@gmail.com
Password: Jh0pan@St0re
```

⚠️ **Important:** Change the default password immediately after first login!

---

## 📖 Documentation

### Management Menu

For day-to-day operations, use the interactive menu:

```bash
sudo bash scripts/menu.sh
```

**Available Options:**

**Services Management**
1. Build & Start All
2. Restart All Services
3. Stop All Services
4. View Logs

**User Management**
5. List Users
6. Add User
7. Delete User
8. Reset Password

**Domain Management**
9. Add Domain (with Cloudflare DNS auto-setup)

**Database**
10. Backup Database

### Adding New Domains

**Via Menu (Recommended):**
```bash
sudo bash scripts/menu.sh
# Select: 9) Add Domain
```

**Direct Command:**
```bash
sudo bash scripts/add-domain.sh newdomain.com
```

The script automatically:
- Creates Cloudflare zone (if needed)
- Sets up MX, A, SPF, and DMARC records
- Updates Maddy configuration
- Updates frontend domain list
- Restarts services

**Requirements:**
- Cloudflare API Token (with Zone:Edit permission)
- Cloudflare Account ID

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         Browser                              │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                    Nginx (Port 80/443)                       │
│              Reverse Proxy + SSL Termination                 │
└───────────────────────────┬─────────────────────────────────┘
                            │
            ┌───────────────┴───────────────┐
            │                               │
            ▼                               ▼
┌───────────────────────┐      ┌─────────────────────────┐
│  Next.js Frontend     │      │   Go Backend (Gin)      │
│  Port 3000            │      │   Port 8080             │
│                       │      │                         │
│  • Login Page         │      │  • REST API             │
│  • Inbox UI           │      │  • JWT Auth             │
│  • Admin Dashboard    │      │  • Rate Limiting        │
│  • API Routes         │      │  • SSE Endpoint         │
└───────────────────────┘      └────────────┬────────────┘
                                            │
                                ┌───────────┴───────────┐
                                │                       │
                                ▼                       ▼
                     ┌──────────────────┐    ┌─────────────────┐
                     │  Maddy SMTP      │    │  PostgreSQL     │
                     │  Port 25         │    │  Database       │
                     │                  │    │                 │
                     │  Receives emails │    │  • inboxes      │
                     │  Forwards to     │    │  • messages     │
                     │  Go backend      │    │  • attachments  │
                     │  via LMTP:8024   │    │  • users        │
                     └──────────────────┘    └─────────────────┘
```

---

## 🛠️ Tech Stack

| Component | Technology | Version |
|-----------|-----------|---------|
| **Backend** | Go (Gin Framework) | 1.25+ |
| **Frontend** | Next.js (App Router) | 16.x |
| **UI** | Tailwind CSS | 4.x |
| **Mail Server** | Maddy | 0.7+ |
| **Database** | PostgreSQL | 14+ |
| **Web Server** | Nginx | 1.24+ |
| **Authentication** | JWT (golang-jwt) | 5.x |
| **Password Hash** | bcrypt | Latest |
| **Rate Limiting** | ulule/limiter | 3.x |
| **DNS Management** | Cloudflare API | v4 |

---

## 📁 Project Structure

```
TempMailByJhopanStore/
├── receiver/              # Go Backend
│   ├── main.go           # Entry point
│   ├── api/              # HTTP routes & middleware
│   ├── handler/          # Auth handlers
│   ├── handlers/         # API handlers
│   ├── middleware/       # JWT & admin middleware
│   ├── models/           # Data models
│   ├── repository/       # Database operations
│   ├── database/         # DB connection
│   ├── parser/           # Email parsing
│   └── lmtp/             # LMTP server
│
├── frontend/             # Next.js Frontend
│   ├── src/app/          # Pages
│   │   ├── page.tsx      # Main inbox
│   │   ├── login/        # Login page
│   │   ├── admin/        # Admin dashboard
│   │   └── api/          # API proxy routes
│   └── src/context/      # Auth context
│
├── scripts/              # Management Scripts
│   ├── setup.sh          # Fresh installation
│   ├── start.sh          # Build & start
│   ├── menu.sh           # Interactive menu
│   └── add-domain.sh     # Domain management
│
├── config/               # Configuration Files
│   ├── nginx/            # Nginx config
│   ├── maddy/            # Maddy config
│   └── systemd/          # Systemd services
│
├── docs/                 # Documentation
│   ├── plan.md           # Full documentation
│   └── DEPLOYMENT.md     # Deployment guide
│
├── README.md             # This file
└── .env.example          # Environment template
```

---

## 🔐 Security Features

- ✅ **JWT Authentication** with 24-hour token expiry
- ✅ **Bcrypt Password Hashing** (cost factor 12)
- ✅ **Rate Limiting** to prevent abuse
- ✅ **Role-Based Access Control** (admin/user)
- ✅ **No Public Registration** (admin creates accounts)
- ✅ **Sandboxed Email Rendering** (iframe isolation)
- ✅ **DOMPurify XSS Protection**
- ✅ **UTF-8 Encoding Validation**
- ✅ **SQL Injection Prevention** (parameterized queries)
- ✅ **Firewall Configuration** (UFW)

---

## 📊 API Endpoints

### Authentication
```
POST   /api/auth/login              # Login (returns JWT)
GET    /api/auth/me                 # Current user info
```

### Admin (Admin only)
```
GET    /api/admin/users             # List all users
POST   /api/admin/users             # Create user
PUT    /api/admin/users/:id         # Update user
DELETE /api/admin/users/:id         # Delete user
PUT    /api/admin/users/:id/password # Reset password
```

### Inbox
```
GET    /inbox/:email                # Get inbox messages
DELETE /inbox/:email                # Delete all messages
```

### Messages
```
GET    /message/:id                 # Get message detail
DELETE /message/:id                 # Delete message
GET    /message/:id/attachments     # Get attachments
```

### Real-time
```
GET    /sse/:email                  # Server-Sent Events stream
```

---

## 🔧 Configuration

### Environment Variables

The setup script automatically creates `receiver/.env` with your configuration:

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=tempmail
DB_PASSWORD=***
# JWT
JWT_SECRET=your-s...n
# Server
PORT=8080
```

### Nginx Configuration

Located at: `/etc/nginx/sites-available/tempmail`

Key features:
- Reverse proxy to Next.js (port 3000)
- SSE endpoint proxy (port 8080)
- WebSocket support
- Gzip compression

### Maddy Configuration

Located at: `/etc/maddy/maddy.conf`

Key features:
- SMTP server (port 25)
- LMTP forwarding to Go backend (port 8024)
- Multi-domain support
- Relaxed spam checks (appropriate for temp mail)

---

## 🐛 Troubleshooting

### Email Not Appearing

```bash
# Check Maddy logs
sudo journalctl -u maddy -f

# Check backend logs
sudo journalctl -u tempmail-receiver -f

# Check database
sudo -u postgres psql -d tempmail -c "SELECT * FROM messages ORDER BY id DESC LIMIT 5;"
```

### Frontend Errors

```bash
# Rebuild frontend
cd /opt/TempMailByJhopanStore/frontend
npm run build
sudo systemctl restart tempmail-frontend

# Check logs
sudo journalctl -u tempmail-frontend -n 50
```

### Login Issues

```bash
# Check if user exists
sudo -u postgres psql -d tempmail -c "SELECT id, email, role FROM users;"

# Reset admin password via menu
sudo bash scripts/menu.sh
# Select: 8) Reset Password
```

### Port Conflicts

```bash
# Check what's using the port
sudo netstat -tlnp | grep :80
sudo netstat -tlnp | grep :25

# Kill process if needed
sudo kill -9 <PID>
```

---

## 🔄 Updating

```bash
cd /opt/TempMailByJhopanStore
git pull
sudo bash scripts/start.sh
# Select: 1) Build & Start (full)
```

---

## 📦 Backup & Restore

### Backup Database

```bash
# Via menu
sudo bash scripts/menu.sh
# Select: 10) Backup Database

# Manual backup
sudo -u postgres pg_dump tempmail > /backup/tempmail-$(date +%Y%m%d).sql
```

### Restore Database

```bash
sudo -u postgres psql -d tempmail < /backup/tempmail-20260612.sql
```

---

## 🚀 Deployment Options

### Option 1: Fresh Install (Recommended)

Follow the [Quick Start](#-quick-start) guide above.

### Option 2: Docker Support (Coming Soon)

```bash
# docker-compose up -d
```

### Option 3: Manual Installation

See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for detailed manual installation steps.

---

## 📝 Roadmap

- [x] Core functionality (inbox, messages, SSE)
- [x] Authentication system
- [x] Admin dashboard
- [x] Multi-domain support
- [x] Cloudflare DNS integration
- [x] Cooldown protection
- [ ] Docker support
- [ ] Email forwarding
- [ ] Custom domain per user
- [ ] Email search & filter
- [ ] Export emails (EML/PDF)
- [ ] Webhook notifications
- [ ] Multi-language support

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

---

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 👤 Author

**JhopanStore**
- Email: jhopanstore@gmail.com
- GitHub: [@jhopan](https://github.com/jhopan)

---

## 🙏 Acknowledgments

- [Maddy Mail Server](https://github.com/foxcpp/maddy) - Composable mail server
- [Next.js](https://nextjs.org/) - The React framework for production
- [Gin](https://gin-gonic.com/) - High-performance HTTP web framework
- [Tailwind CSS](https://tailwindcss.com/) - Utility-first CSS framework
- [Cloudflare](https://cloudflare.com/) - DNS and CDN services

---

## 📞 Support

For support and questions:
- 📧 Email: jhopanstore@gmail.com
- 🐛 Issues: [GitHub Issues](https://github.com/jhopan/TempMailByJhopanstore/issues)

---

<div align="center">

**Made with ❤️ by JhopanStore**

⭐ Star this repo if you find it useful!

</div>
