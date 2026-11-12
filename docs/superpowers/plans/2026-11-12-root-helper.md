# Root helper (tanpa memberi root ke app) — interface contract

**Goal:** panel web bisa restart service, backup, baca log, pasang/restart tunnel, logout credential, dan doctor, tanpa app menjadi root.

**Architecture:** app (user `tempmail`) menulis request JSON ke `$HELPER_DIR/in/<id>.json`; systemd `tempmail-root.path` (`DirectoryNotEmpty=$HELPER_DIR/in`) memicu `tempmail-root.service` (root, oneshot) yang menjalankan `/usr/local/sbin/tempmail-root --once`; helper memvalidasi lalu menjalankan aksi allowlist dan menulis `$HELPER_DIR/out/<id>.json`. App membaca hasil dan menghapusnya. Tidak ada sudoers, `NoNewPrivileges=true` di unit app tetap aktif.

**Contract version:** 1

## Direktori

```text
/opt/tempmail/data/helper/          tempmail:tempmail 0700
/opt/tempmail/data/helper/in/       tempmail:tempmail 0750   request dari app
/opt/tempmail/data/helper/out/      tempmail:tempmail 0750   hasil dari helper
/opt/tempmail/data/helper/state/    root:root 0700           cooldown + audit
```

App env: `HELPER_DIR` (default `/opt/tempmail/data/helper`), `HELPER_WAIT_SECONDS` (default 25).

## Request (app -> helper)

Nama file `<id>.json`, `id` = 16 hex acak. Field ketat, field tak dikenal = ditolak.

```json
{"id":"a1b2c3d4e5f60718","action":"logs","lines":100,"token":"","confirm":"","delay":0,"requested_by":"admin@example.invalid"}
```

| Action | Param | Efek helper | Mode |
|---|---|---|---|
| `service-status` | - | `systemctl is-active` untuk `tempmail`, `cloudflared`, `tempmail-backup.timer` + detail | tunggu |
| `restart-service` | `delay` (0-10, default 2) | tunggu `delay` detik, lalu `systemctl restart tempmail` | fire-and-forget |
| `backup-now` | - | `systemctl start tempmail-backup.service` | tunggu |
| `logs` | `lines` (1-500, default 100) | `journalctl -u tempmail -n <lines> --no-pager` | tunggu |
| `doctor` | - | cek binary, env 0640 root:tempmail, data dir owner, `/etc/cloudflared/config.yml`, `cert.pem`, cloudflared active, backup timer | tunggu |
| `tunnel-status` | - | `cloudflared tunnel list --output json` + status service | tunggu |
| `tunnel-install` | `token` (regex `^[A-Za-z0-9._~-]{20,4096}$`) | `cloudflared service install <token>`, `systemctl enable --now cloudflared` | tunggu |
| `tunnel-restart` | - | `systemctl restart cloudflared` | tunggu |
| `tunnel-logout` | `confirm` harus `RESET` | stop + disable + uninstall service, hapus `/etc/cloudflared` dan `/root/.cloudflared` | fire-and-forget |

Fire-and-forget = handler HTTP membalas `status: "accepted"` setelah request ditulis (aksi akan memutus app/tunnel, jadi hasil tidak bisa ditunggu).

## Result (helper -> app)

```json
{"id":"...","action":"...","ok":true,"status":"completed","output":"text","error":"","finished_at":"RFC3339","seconds":0.42}
```

`status`: `completed` | `rejected` (validasi gagal) | `failed` (aksi gagal). `ok` = `status == "completed"`.

## Aturan keamanan helper

- Tolak: request berupa symlink, ukuran > 4096 byte, owner file bukan `HELPER_USER`, JSON tidak valid, field tak dikenal, action di luar allowlist, param di luar aturan.
- Tidak pernah memakai shell: seluruh eksekusi lewat `subprocess` dengan daftar argumen.
- Tulis hasil atomik (file `.tmp` + `os.replace`), mode 0640 `root:tempmail`.
- Cooldown per action untuk yang memutasi: `restart-service` 10s, `tunnel-install` 30s, `tunnel-logout` 60s, `backup-now` 10s.
- Output dibatasi 20000 byte.
- Setiap request dicatat ke journald (`print` ke stdout/stderr helper).
- Override biner hanya lewat env (`HELPER_SYSTEMCTL`, `HELPER_JOURNALCTL`, `HELPER_CLOUDFLARED`) untuk pengujian; unit systemd tidak menyetelnya.

## HTTP (JWT + admin, grup `/api/admin/system`)

| Method | Path | Body | Hasil |
|---|---|---|---|
| GET | `/api/admin/system/status` | - | fakta lokal app (tanpa helper, cepat) |
| POST | `/api/admin/system/action` | `{"action":"logs","lines":100}` | `200` hasil helper, `400` aksi/param salah, `501` helper belum dipasang, `504` helper tidak menjawab |

`GET /api/admin/system/status`:

```json
{"helper":{"installed":true,"dir":"/opt/tempmail/data/helper","pending":0},
 "db":{"path":"...db","bytes":40960,"inboxes":2,"messages":5,"attachments":0},
 "app":{"uptime_seconds":320,"rss_bytes":15728640,"go_version":"go1.25.0","site_name":"TempMail","port":"8081"},
 "cloudflare":{"profiles":2,"domains":3,"active_profile":"cf-a","worker_name":"tempmailbyjhopanstore"}}
```
