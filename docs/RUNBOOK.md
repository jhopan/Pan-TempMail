# Runbook — TempMail By JhopanStore

## Verified Armbian host layout

```text
App binary:     /opt/tempmail/tempmail
Config:         /etc/tempmail/tempmail.env
CF state:       /etc/tempmail/cloudflare-state.json
Database:       /opt/tempmail/data/tempmail.db
Backups:        /opt/tempmail/backups/
UI LAN:         http://192.168.x.x:8080/app/
Menu:           menutempmail
```

## Normal operator commands

```bash
menutempmail
sudo systemctl status tempmail --no-pager
sudo journalctl -u tempmail -f
sudo systemctl restart tempmail
curl -fsS http://127.0.0.1:8080/health
```

## Service failure

```bash
sudo systemctl status tempmail --no-pager -l
sudo journalctl -u tempmail -n 100 --no-pager
sudo ls -ld /opt/tempmail/data
sudo ls -l /etc/tempmail/tempmail.env
```

Expected writable DB paths:

```text
/opt/tempmail/data
/opt/tempmail/backups
owner: tempmail:tempmail
```

If SQLite reports `unable to open database file`, repair directory ownership then restart:

```bash
sudo chown -R tempmail:tempmail /opt/tempmail/data /opt/tempmail/backups
sudo chmod 0750 /opt/tempmail/data /opt/tempmail/backups
sudo systemctl restart tempmail
```

## First admin

Only run when no users exist:

```bash
curl -X POST http://127.0.0.1:8080/api/setup \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.invalid","password":"replace-with-long-unique-password"}'
```

Do not put real password commands in shell history. Use interactive API client/UI where possible.

## Local end-to-end test

1. Login and obtain JWT.
2. Create inbox `armbian-check@local.invalid`.
3. Send an RFC822 test message to `127.0.0.1:2525`.
4. Query inbox and message.
5. Confirm OTP/link extraction.

Python smoke test:

```bash
python3 - <<'PY'
import smtplib
from email.mime.text import MIMEText
m = MIMEText('Code: 593817. Verify: https://example.invalid/verify?token=test')
m['From'] = 'sender@test.invalid'
m['To'] = 'armbian-check@local.invalid'
m['Subject'] = 'TempMail test'
s = smtplib.SMTP('127.0.0.1', 2525, timeout=10)
s.sendmail(m['From'], [m['To']], m.as_string())
s.quit()
PY
```

Then inspect with authenticated API/UI.

## Cloudflare setup order

1. `menutempmail` → account menu: add CF profile with API token/account ID.
2. Domain menu: add active Cloudflare zone to that profile.
3. Login Cloudflare Tunnel and create one web hostname.
4. Select profile, deploy Worker for it.
5. Activate routing for one domain.
6. Send test email to `random@that-domain`.
7. Verify Worker delivery to TempMail.
8. Repeat profile/domain flow for CF B, CF C as needed.

Important:

- One tunnel hostname can serve Workers from domains/profile accounts.
- Every profile has its own Worker deploy and routing action.
- Copies default OFF. Enabling a copy destination requires Cloudflare destination-address verification.
- Do not enable catch-all before Worker and tunnel health are working.

## Multi-domain operations

`menutempmail` domain removal removes local TempMail acceptance only. It intentionally does not delete Cloudflare zones/DNS/routing.

To fully retire a domain:

1. Disable/remove its Cloudflare catch-all after confirming no mail is needed.
2. Remove it from TempMail menu.
3. Keep DNS/zone unless explicitly retiring it.

## Backup and restore

```bash
sudo systemctl status tempmail-backup.timer
sudo ls -lh /opt/tempmail/backups/
```

Restore:

```bash
sudo systemctl stop tempmail
sudo cp /opt/tempmail/backups/tempmail-YYYY-MM-DD.db /opt/tempmail/data/tempmail.db
sudo chown tempmail:tempmail /opt/tempmail/data/tempmail.db
sudo systemctl start tempmail
curl -fsS http://127.0.0.1:8080/health
```

## Updating binary

Build on Windows, never on VPS/Armbian:

```bash
cd receiver
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o tempmail-linux-arm64 .
```

Upload then on Armbian:

```bash
sudo systemctl stop tempmail
sudo install -o root -g root -m 0755 /tmp/tempmail-linux-arm64 /opt/tempmail/tempmail
sudo systemctl start tempmail
curl -fsS http://127.0.0.1:8080/health
```

Use a prior locally built binary for rollback if health check fails.

## Release bootstrap status

`scripts/install-armbian.sh` expects GitHub Release assets. The repository is private and a release asset pipeline is not yet published. Until then, use local build + SCP deployment.