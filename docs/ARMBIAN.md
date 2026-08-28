# TempMail By JhopanStore on Armbian / Linux

Product name: **TempMail By JhopanStore**. The Go core builds for Linux, macOS, and Windows. Current verified service installer targets Debian/Ubuntu/Armbian Linux through systemd.

Runs TempMail as one low-RAM service:

```text
TempMail:     /opt/tempmail/tempmail
Database:     /opt/tempmail/data/tempmail.db
Web UI:       http://<armbian-lan-ip>:8080/app/
API:          http://<armbian-lan-ip>:8080/api/
SMTP test:    127.0.0.1:2525 only
```

No VPS required. Incoming public email later uses:

```text
Cloudflare Email Routing → Email Worker → Cloudflare Tunnel → 127.0.0.1:8080
```

## Build on Windows

```bash
cd receiver
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o tempmail-linux-arm64 .
```

Never build on the Armbian device.

## Install

Copy these files to one temporary directory on Armbian:

```text
tempmail-linux-arm64
install.sh
tempmail.service
tempmail-backup.service
tempmail-backup.timer
```

Then run:

```bash
chmod +x install.sh
sudo ./install.sh ./tempmail-linux-arm64
```

The installer creates a `tempmail` system user, SQLite storage, services, and a daily backup. It enables SMTP only on loopback port `2525`; no public port 25 is required.

## First Admin

Create exactly once after installation:

```bash
curl -X POST http://127.0.0.1:8080/api/setup \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.invalid","password":"use-a-long-unique-password"}'
```

Then open `http://<armbian-lan-ip>:8080/app/login.html`.

## Service Operations

```bash
systemctl status tempmail
journalctl -u tempmail -f
systemctl restart tempmail
systemctl status tempmail-backup.timer
ls -lh /opt/tempmail/backups/
```

## Local Automation

Use API from Armbian, LAN, Tailscale, or a future Cloudflare Tunnel:

```bash
POST /api/inbox
GET  /api/inbox/<email>/wait?timeout=60
GET  /api/message/<id>
```

The wait endpoint returns automatically extracted OTP codes and links.

## Cloudflare Tunnel Later

Do not expose `8080` with router port forwarding. Use a named Cloudflare Tunnel:

```text
mail.example.com → Cloudflare Tunnel → http://127.0.0.1:8080
```

Put the tunnel URL in the Email Worker as `TEMPMAIL_SERVER_URL`. Set the same random value for Worker secret and `/etc/tempmail/tempmail.env` variable `EMAIL_API_KEY`.

Cloudflare Email Routing catch-all sends every `*@email-domain.example` to the Worker. The Worker preserves `message.to`, so `abc@email-domain.example` is stored separately from `otp@email-domain.example`.

Use a distinct web hostname (`mail.example.com`) and email domain (`email-domain.example` or the root domain). Do not use the web hostname as the temp-mail address domain.

## Backup

Daily SQLite backup is retained for seven days:

```text
/opt/tempmail/backups/tempmail-YYYY-MM-DD.db
```

Restore only while service is stopped:

```bash
sudo systemctl stop tempmail
sudo cp /opt/tempmail/backups/tempmail-YYYY-MM-DD.db /opt/tempmail/data/tempmail.db
sudo chown tempmail:tempmail /opt/tempmail/data/tempmail.db
sudo systemctl start tempmail
```

## Security

- `/etc/tempmail/tempmail.env` holds JWT and Worker API secrets; mode `640`.
- Keep `8080` LAN-only until Cloudflare Tunnel/Tailscale is configured.
- Cloudflare Worker endpoint requires `X-Email-API-Key`.
- Replace temporary `local.invalid` in `DOMAINS` before real email routing.
- Do not publish admin credentials in shell history, Git, or chat.

## Current Armbian target

```text
Architecture: ARM64 / aarch64
LAN UI:       http://192.168.x.x:8080/app/
```

This target is separate from VPS deployments.
```