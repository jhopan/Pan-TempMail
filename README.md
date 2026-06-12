# 📧 TempMailByJhopanStore

Platform Temp Mail self-hosted yang lengkap dengan autentikasi, multi-domain, dan real-time email updates.

![Version](https://img.shields.io/badge/version-1.0.0-blue)
![License](https://img.shields.io/badge/license-MIT-green)
![Status](https://img.shields.io/badge/status-Production-success)

---

## ✨ Fitur Utama

- 🔐 **Authentication System** — Login dengan JWT, role-based access (admin/user)
- 📬 **Real-time Inbox** — Server-Sent Events (SSE) untuk update instan
- 🌍 **Multi-Domain Support** — Dropdown domain selector dengan Cloudflare API integration
- 🎨 **Modern UI** — Next.js 16 + Tailwind CSS dengan dark theme
- 🔒 **Secure Email Rendering** — iframe sandbox untuk isolasi HTML email
- 🛡️ **Admin Dashboard** — User management (create, edit, delete, reset password)
- ⏱️ **Cooldown Protection** — 60s cooldown untuk mencegah abuse
- 📎 **Attachment Support** — View dan download email attachments
- 🧹 **Auto Cleanup** — Hapus inbox otomatis setelah 24 jam
- 🔧 **Easy Domain Setup** — Script otomatis dengan Cloudflare DNS integration

---

## 🚀 Quick Start

### Prerequisites

- Ubuntu 22.04/24.04 LTS
- 1GB RAM minimum
- Domain dengan akses DNS (Cloudflare recommended)
- SSH access ke VPS

### Installation

```bash
# 1. Clone/download project
cd /opt
sudo git clone <repository-url> TempMailByJhopanStore
cd TempMailByJhopanStore

# 2. Run setup script (akan install semua dependencies)
sudo bash scripts/setup.sh

# 3. Configure environment
cp .env.example receiver/.env
nano receiver/.env  # Edit database credentials

# 4. Setup database
sudo bash scripts/setup-db.sh

# 5. Build & start services
sudo bash scripts/deploy.sh

# 6. Access the app
# http://your-vps-ip
```

### Default Admin Account

```
Email:    jhopanstore@gmail.com
Password: Jh0pan@St0re
```

**⚠️ PENTING:** Ganti password setelah login pertama!

---

## 📖 Dokumentasi

- **[plan.md](plan.md)** — Dokumentasi lengkap (arsitektur, dependencies, setup)
- **[docs/API.md](docs/API.md)** — API endpoints documentation
- **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)** — Production deployment guide
- **[docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)** — Common issues & solutions

---

## 🏗️ Architecture

```
┌─────────────┐
│   Browser   │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│    Nginx    │ Port 80/443
│ (Proxy+SSE) │
└──────┬──────┘
       │
       ├──────────────────┐
       ▼                  ▼
┌─────────────┐    ┌─────────────┐
│   Next.js   │    │ Go Receiver │
│  (Frontend) │    │  (Backend)  │
│  Port 3000  │    │  Port 8080  │
└─────────────┘    └──────┬──────┘
                          │
                    ┌─────┴─────┐
                    ▼           ▼
              ┌──────────┐ ┌────────┐
              │ Maddy    │ │Postgres│
              │ (SMTP)   │ │  (DB)  │
              │ Port 25  │ │        │
              └──────────┘ └────────┘
```

---

## 🛠️ Tech Stack

| Component    | Technology              |
|--------------|-------------------------|
| Backend      | Go 1.25+ (Gin)          |
| Frontend     | Next.js 16 (App Router) |
| UI           | Tailwind CSS 4.x        |
| Mail Server  | Maddy                   |
| Database     | PostgreSQL 14+          |
| Web Server   | Nginx 1.24+             |
| Auth         | JWT + Bcrypt            |
| DNS          | Cloudflare API          |

---

## 📁 Project Structure

```
TempMailByJhopanStore/
├── receiver/           # Go backend
├── frontend/           # Next.js frontend
├── scripts/            # Setup & management scripts
├── config/             # Configuration files
├── docs/               # Documentation
├── plan.md             # Full documentation
└── README.md           # This file
```

---

## 🔧 Configuration

### Environment Variables (receiver/.env)

```env
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=tempmail
DB_PASSWORD=***
D# JWT
JWT_SECRET=your-secret-key-here

# Server
PORT=8080
```

### Add New Domain

```bash
# Script akan:
# 1. Setup Cloudflare DNS (MX, A, SPF, DMARC)
# 2. Update Maddy config
# 3. Update frontend domain list
# 4. Restart services

sudo bash scripts/add-domain.sh newdomain.com
```

---

## 🚀 Deployment

### Production Deployment

```bash
# Build backend
cd receiver
go build -o receiver-final

# Build frontend
cd ../frontend
npm run build

# Start services
sudo systemctl start tempmail-receiver
sudo systemctl start tempmail-frontend
sudo systemctl start maddy
sudo systemctl start nginx
```

### Using Docker (Coming Soon)

```bash
# docker-compose up -d
```

---

## 📊 API Endpoints

### Authentication
- `POST /api/auth/login` — Login
- `GET /api/auth/me` — Current user (protected)

### Admin (Admin only)
- `GET /api/admin/users` — List users
- `POST /api/admin/users` — Create user
- `PUT /api/admin/users/:id` — Update user
- `DELETE /api/admin/users/:id` — Delete user

### Inbox
- `GET /api/inbox/:email` — Get messages
- `DELETE /api/inbox/:email` — Delete inbox

### Messages
- `GET /api/message/:id` — Get message
- `DELETE /api/message/:id` — Delete message
- `GET /api/message/:id/attachments` — Get attachments

### Real-time
- `GET /api/sse/:email` — SSE stream

---

## 🔒 Security Features

- ✅ JWT token authentication (24h expiry)
- ✅ Bcrypt password hashing
- ✅ Rate limiting (60 req/min API, 20 req/min DELETE)
- ✅ Role-based access control (admin/user)
- ✅ No public registration (admin creates accounts)
- ✅ iframe sandbox for email HTML isolation
- ✅ DOMPurify for XSS protection
- ✅ UTF-8 encoding validation
- ✅ SQL injection prevention (parameterized queries)

---

## 🐛 Troubleshooting

### Email tidak muncul di inbox

```bash
# Check Maddy logs
sudo journalctl -u maddy -f

# Check backend logs
sudo journalctl -u tempmail-receiver -f

# Check database
sudo -u postgres psql -d tempmail -c "SELECT * FROM messages ORDER BY id DESC LIMIT 5;"
```

### Frontend error

```bash
# Check frontend logs
sudo journalctl -u tempmail-frontend -f

# Rebuild
cd /opt/TempMailByJhopanStore/frontend
npm run build
sudo systemctl restart tempmail-frontend
```

### Login failed

```bash
# Check if user exists
sudo -u postgres psql -d tempmail -c "SELECT id, email, role FROM users;"

# Reset admin password
python3 -c "
import bcrypt
hashed = bcrypt.hashpw(b'Jh0pan@St0re', bcrypt.gensalt()).decode()
print(hashed)
" | sudo -u postgres psql -d tempmail -c "UPDATE users SET password_hash = '$(cat)' WHERE email = 'jhopanstore@gmail.com';"
```

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
- [ ] API rate limiting per user
- [ ] Email search & filter
- [ ] Export emails (EML/PDF)
- [ ] Webhook notifications
- [ ] Multi-language support

---

## 🤝 Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details.

---

## 👤 Author

**JhopanStore**
- Email: jhopanstore@gmail.com
- Project: TempMailByJhopanStore

---

## 🙏 Acknowledgments

- [Maddy Mail Server](https://github.com/foxcpp/maddy) — Composable mail server
- [Next.js](https://nextjs.org/) — React framework
- [Gin](https://gin-gonic.com/) — Go web framework
- [Tailwind CSS](https://tailwindcss.com/) — Utility-first CSS
- [Cloudflare](https://cloudflare.com/) — DNS & CDN services

---

## 📞 Support

Untuk bantuan dan pertanyaan:
- Email: jhopanstore@gmail.com
- Issues: [GitHub Issues](link-to-issues)

---

**Made with ❤️ by JhopanStore**
