# 🚀 Quick Deployment Guide

## Fresh Install di VPS Baru

### 1. Clone Repository

```bash
cd /opt
sudo git clone <your-repo-url> TempMailByJhopanStore
cd TempMailByJhopanStore
sudo chown -R $USER:$USER .
```

### 2. Jalankan Setup

```bash
sudo bash scripts/setup.sh
```

Setup akan otomatis:
- ✅ Install semua dependencies (Go, Node.js, PostgreSQL, Maddy, Nginx)
- ✅ Setup database PostgreSQL
- ✅ Buat admin user default
- ✅ Konfigurasi Maddy mail server
- ✅ Konfigurasi Nginx reverse proxy
- ✅ Setup systemd services
- ✅ Konfigurasi firewall

**Input yang diminta:**
- Database password
- Server IP address
- Primary domain

### 3. Build & Start

```bash
sudo bash scripts/start.sh
```

Pilih opsi **1) Build & Start (full)**

### 4. Access

Buka browser: `http://your-server-ip`

**Login default:**
- Email: `jhopanstore@gmail.com`
- Password: `Jh0pan@St0re`

⚠️ **Ganti password setelah login pertama!**

---

## 📋 Management Menu

Untuk manajemen sehari-hari:

```bash
sudo bash scripts/menu.sh
```

**Menu yang tersedia:**

### Services
1. Build & Start All
2. Restart All Services
3. Stop All Services
4. View Logs

### User Management
5. List Users
6. Add User
7. Delete User
8. Reset Password

### Domain
9. Add Domain (dengan Cloudflare DNS auto-setup)

### Database
10. Backup Database

---

## 🌍 Tambah Domain Baru

### Cara 1: Via Menu (Recommended)

```bash
sudo bash scripts/menu.sh
# Pilih: 9) Add Domain
# Masukkan domain baru
# Masukkan Cloudflare API Token & Account ID
```

### Cara 2: Langsung

```bash
sudo bash scripts/add-domain.sh newdomain.com
```

Script akan otomatis:
- ✅ Buat zone di Cloudflare (jika belum ada)
- ✅ Setup MX record → mail server
- ✅ Setup A record → server IP
- ✅ Setup SPF & DMARC records
- ✅ Update Maddy config
- ✅ Update frontend domain list
- ✅ Restart services

**Requirements:**
- Cloudflare API Token (dengan Zone:Edit permission)
- Cloudflare Account ID

---

## 🔧 Manual Commands

### Restart Services

```bash
sudo systemctl restart tempmail-receiver
sudo systemctl restart tempmail-frontend
sudo systemctl restart maddy
sudo systemctl restart nginx
```

### View Logs

```bash
# Backend
sudo journalctl -u tempmail-receiver -f

# Frontend
sudo journalctl -u tempmail-frontend -f

# Mail server
sudo journalctl -u maddy -f

# Nginx
sudo journalctl -u nginx -f
```

### Database Operations

```bash
# Login ke database
sudo -u postgres psql -d tempmail

# List users
SELECT id, email, role, created_at FROM users;

# Add user manually
INSERT INTO users (email, password_hash, role) 
VALUES ('user@example.com', '$2b$12$...', 'user');

# Backup
sudo -u postgres pg_dump tempmail > backup.sql

# Restore
sudo -u postgres psql -d tempmail < backup.sql
```

### Rebuild After Changes

```bash
# Backend only
cd /opt/TempMailByJhopanStore/receiver
go build -o receiver-final .
sudo systemctl restart tempmail-receiver

# Frontend only
cd /opt/TempMailByJhopanStore/frontend
npm run build
sudo systemctl restart tempmail-frontend
```

---

## 🔐 Security Checklist

- [ ] Ganti password admin default
- [ ] Setup SSL/TLS dengan Let's Encrypt
- [ ] Konfigurasi firewall (sudah otomatis di setup.sh)
- [ ] Backup database secara berkala
- [ ] Update system packages regularly
- [ ] Monitor logs untuk suspicious activity

### Setup SSL (Optional)

```bash
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d yourdomain.com
```

---

## 📊 Monitoring

### Check Service Status

```bash
sudo systemctl status tempmail-receiver
sudo systemctl status tempmail-frontend
sudo systemctl status maddy
sudo systemctl status nginx
```

### Check Ports

```bash
sudo netstat -tlnp | grep -E ':(25|80|443|3000|8080)'
```

### Check Database

```bash
# Total inboxes
sudo -u postgres psql -d tempmail -c "SELECT COUNT(*) FROM inboxes;"

# Total messages
sudo -u postgres psql -d tempmail -c "SELECT COUNT(*) FROM messages;"

# Recent messages
sudo -u postgres psql -d tempmail -c "SELECT id, sender, subject, received_at FROM messages ORDER BY id DESC LIMIT 10;"
```

---

## 🐛 Troubleshooting

### Email tidak masuk

```bash
# Check Maddy logs
sudo journalctl -u maddy -f

# Check if domain is in Maddy config
grep "local_domains" /etc/maddy/maddy.conf

# Check DNS (MX record)
dig MX yourdomain.com
```

### Frontend error

```bash
# Rebuild frontend
cd /opt/TempMailByJhopanStore/frontend
npm run build
sudo systemctl restart tempmail-frontend

# Check logs
sudo journalctl -u tempmail-frontend -n 50
```

### Backend error

```bash
# Rebuild backend
cd /opt/TempMailByJhopanStore/receiver
go build -o receiver-final .
sudo systemctl restart tempmail-receiver

# Check logs
sudo journalctl -u tempmail-receiver -n 50
```

### Login failed

```bash
# Check if user exists
sudo -u postgres psql -d tempmail -c "SELECT id, email, role FROM users;"

# Reset admin password
sudo bash scripts/menu.sh
# Pilih: 8) Reset Password
```

### Port conflict

```bash
# Check what's using the port
sudo netstat -tlnp | grep :80
sudo netstat -tlnp | grep :25

# Kill process if needed
sudo kill -9 <PID>
```

---

## 📦 Backup & Restore

### Backup

```bash
# Via menu
sudo bash scripts/menu.sh
# Pilih: 10) Backup Database

# Manual
sudo -u postgres pg_dump tempmail > /backup/tempmail-$(date +%Y%m%d).sql
```

### Restore

```bash
sudo -u postgres psql -d tempmail < /backup/tempmail-20260612.sql
```

---

## 🔄 Update from Git

```bash
cd /opt/TempMailByJhopanStore
git pull
sudo bash scripts/start.sh
# Pilih: 1) Build & Start (full)
```

---

## 📞 Support

Untuk bantuan:
- Check logs: `sudo bash scripts/menu.sh` → 4) View Logs
- Check plan.md untuk dokumentasi lengkap
- Check README.md untuk overview

---

**Made with ❤️ by JhopanStore**
