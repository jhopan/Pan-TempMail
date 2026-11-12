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

Two equivalent paths; both write the same records.

Web admin panel (`/admin`, not linked from the inbox UI, admin account required):

1. Create an API token per account (Zone:Read, Zone Settings:Edit, DNS:Edit, Email Routing Rules:Edit, Workers Scripts:Edit; add Cloudflare Tunnel:Edit to manage or read tunnels from the panel, and Account Settings:Read for the setup scripts' account check).
2. Profiles, add the account with its Account ID and token, then activate it.
3. Domains, add the domain; the zone must be `active` in that account.
4. Profiles, deploy the Worker for that profile.
5. Domains, enable routing for the domain (tick `unlock DNS` when MX records are not Cloudflare's).

Command line (`menutempmail`):

1. `1` account menu: add CF profile with API token/account ID.
2. `2` domain menu: add active Cloudflare zone to that profile.
3. `4` buat tunnel lewat API token profil aktif, lalu jalankan perintah `cloudflared service install` yang dikembalikan (langkah root, tanpa login browser).
4. `5` select profile, deploy Worker for it.
5. `6` activate routing for one domain.
6. Send test email to `random@that-domain`.
7. Verify Worker delivery to TempMail.
8. Repeat profile/domain flow for CF B, CF C as needed.

Existing installs: `menutempmail → 1 → 4` (or the panel's Import card) loads the old
`/etc/tempmail/cloudflare-state.json` into the database once. Re-running the import is safe.

Important:

- Adding a domain needs no service restart: the domain list comes from the database, with `DOMAINS` in the env file as fallback only.
- One tunnel hostname can serve Workers from domains/profile accounts.
- Every profile has its own Worker deploy and routing action; the tunnel never needs a second login.
- Copies default OFF. Enabling a copy destination requires Cloudflare destination-address verification in that account.
- Do not enable catch-all before Worker and tunnel health are working.
- Only root can install or restart `cloudflared`; the panel shows tunnel status and prints the install commands instead.

## Panel server actions (root helper)

The panel section `Server` can restart the app service, run the SQLite backup, list the SQLite
backups, show journald logs, run doctor checks, read the tunnel config, and install/restart/logout the
tunnel. All of it goes through the root helper; the app itself stays unprivileged.

Check the helper:

```bash
systemctl status tempmail-root.path tempmail-root.timer --no-pager
ls -l /opt/tempmail/data/helper/in /opt/tempmail/data/helper/out
journalctl -u tempmail-root.service -n 50 --no-pager
```

Process a request by hand (root shell), useful when the panel reports a timeout:

```bash
/usr/local/sbin/tempmail-root --once
journalctl -u tempmail-root.service -n 20 --no-pager
```

Disable the panel's server actions (the panel then shows `helper belum dipasang`; nothing else breaks):

```bash
systemctl disable --now tempmail-root.path tempmail-root.timer
```

Notes:

- `restart-service` waits 2 seconds by default before restarting, so the HTTP answer reaches the panel; the panel then polls `/health` until the service is back.
- `tunnel-install` needs a connector token from the panel's Tunnel section (`POST /api/admin/cloudflare/tunnel/create`).
- `tunnel-config` reports how the named tunnel authenticates (mode, tunnel id, credentials path, ingress hostnames, token presence) from `/etc/cloudflared/config.yml`; it is read-only and never prints the token.
- `backup-list` reports `count`/`latest`/`latest_bytes`/`latest_age_seconds`/`total_bytes` for `/opt/tempmail/backups`; an empty or missing directory answers `latest: -` with zeros, not an error.
- `tunnel-logout` deletes `/etc/cloudflared` and `/root/.cloudflared`; it requires the operator to type `RESET` in the panel.
- A rejected request still produces a result file, and the request file is always removed afterwards; if requests pile up in `helper/in`, check `journalctl -u tempmail-root.service`.
- Helper environment overrides (`HELPER_SYSTEMCTL`, `HELPER_JOURNALCTL`, `HELPER_CLOUDFLARED`, `HELPER_TUNNEL_CONFIG`, `HELPER_BACKUP_DIR`, `HELPER_TEST_MODE`) exist for offline testing of the helper itself; the installed systemd unit never sets them, so do not add them to the unit.

## Admin password

Password admin tidak lagi disimpan plaintext di `/etc/tempmail/tempmail.env`. Hanya `ADMIN_EMAIL` yang
disimpan sebagai petunjuk akun; password ditanyakan saat login API.

Reset dari server:

```bash
menutempmail → A → 3      # reset password admin (bcrypt ke database, tidak menyimpan ke file)
```

Reset dari panel web: `/admin` → **Pengguna** → `Reset password` pada baris akun (butuh akun admin yang
sudah login).

Pemanggil non-interaktif (mis. `setup-auto.sh`, skrip otomatis) memberi password lewat variabel:

```bash
TM_ADMIN_PASS='...' menutempmail          # dipakai menu saat tidak ada input terminal
ADMIN_PASSWORD='...' sudo bash /opt/tempmail/setup-auto.sh --domain ... --api-token ...
```

Kalau password hilang seluruhnya, akun admin di database bisa direset langsung:

```bash
sudo sqlite3 /opt/tempmail/data/tempmail.db "select id,email,role from users;"
python3 -c "import bcrypt,sqlite3; db=sqlite3.connect('/opt/tempmail/data/tempmail.db'); db.execute('UPDATE users SET password_hash=? WHERE email=?', (bcrypt.hashpw(b'password-baru', bcrypt.gensalt()).decode(), 'admin@example.invalid')); db.commit()"
```

## Multi-domain operations

`menutempmail` domain removal removes local TempMail acceptance only. It intentionally does not delete Cloudflare zones/DNS/routing.

To fully retire a domain:

1. Disable/remove its Cloudflare catch-all after confirming no mail is needed.
2. Remove it from TempMail menu.
3. Keep DNS/zone unless explicitly retiring it.

The panel offers the same in two labelled actions per row: `Hapus domain` (local only) and, for a
domain that is still routing, `Matikan & hapus` (disables the catch-all first, then removes it).

## Adding a domain from a second Cloudflare account

One TempMail instance accepts domains from several Cloudflare accounts. Each profile keeps its own
account ID and API token, and every Cloudflare call for a domain uses that domain's profile, so a
token can never act on the other account's zone. The check below is the proof to run after adding a
profile: adding a domain that lives in account B while selecting account A must be rejected.

Verified flow, all from `/admin` (no SSH):

1. `Akun Cloudflare` → create the second profile (`id`, label, account ID, API token of that account,
   leave "activate" off when it is not the main account).
2. `Domain` → add the domain, pick the second profile in the profile dropdown. A zone that does not
   exist in that account is refused with the Cloudflare reason in `detail`.
3. `Domain` → `Periksa` on that row, then `Rapikan`: this deploys the Worker for that profile, enables
   Email Routing, creates the missing Email Routing records, and creates the web-host CNAME.
4. `Tunnel` → `Buat CNAME web host` once per web-host zone; the tunnel itself is shared, and every
   profile's Worker gets the same `TEMPMAIL_SERVER_URL` and `TEMPMAIL_API_KEY` during deploy.

The same is available in `menutempmail`: option `1` adds the profile, `2` adds the domain, `5`
deploys the Worker for the selected profile, `6` enables routing.

Pitfalls:

- A profile without an API token cannot be used for these steps; `menutempmail` option `3` stores a
  token for an existing profile (an install that came from the old wrangler-login flow has none).
- Worker names are per account, so the same name exists once in each account; that is expected.
- Domain names are unique across the whole instance, so the same domain cannot be registered twice.
- The web host CNAME belongs to the account that owns the web-host zone, not to every profile.

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

## Release and deployment status

`.github/workflows/release.yml` builds Linux amd64, Linux arm64, and Windows amd64 on every `v*` tag
and attaches them to the GitHub Release. Until a release is published for the current code, deploy with
a local cross-build and SCP as shown above, keeping the previous binary for rollback.