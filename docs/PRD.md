# PRD — TempMail By JhopanStore

## 1. Purpose

Self-hosted, ultra-light temporary email platform for manual use and legitimate test/automation flows. Must run with low RAM/CPU, receive mail without public port 25 through Cloudflare Email Routing, and preserve separate inboxes per recipient address.

## 2. Goals

- Single Go binary with embedded web UI and SQLite.
- Low resource baseline: target <100MB RAM including tunnel process on Linux.
- One instance supports many temp-mail domains.
- Multiple Cloudflare accounts/profiles may contribute domains to one TempMail instance.
- Per-domain catch-all routing: `anything@domain.example` reaches its own inbox.
- Automation API can create an inbox and wait for a message without client polling.
- Extract common OTP codes and links from received mail.
- Optional email copies are OFF by default.
- One interactive menu (`menutempmail`) handles Linux/Armbian setup and ongoing Cloudflare/domain operations.

## 3. Non-goals

- Sending outbound email.
- Full IMAP/SMTP mailbox service.
- Public anonymous inbox browsing.
- Bypassing site anti-abuse controls or using temp email for fraud.
- Claiming production adapters for Windows/macOS until implemented and tested.

## 4. Users

| User | Need |
|---|---|
| Owner/admin | Configure Cloudflare accounts, domains, Worker, Tunnel, and users. |
| Automation client | Create unique address, wait for verification mail, read OTP/link. |
| Manual user | Open web UI, inspect inbox, download attachment, clear data. |

## 5. Core flows

### 5.1 HTTP email flow — default for cloud hosts

```text
Sender
→ Cloudflare Email Routing catch-all
→ Worker for that Cloudflare profile
→ HTTPS Cloudflare Tunnel
→ POST /api/email/receive
→ parse + SQLite + SSE
→ web/API inbox
```

`message.to` is carried unchanged. This separates `abc@domain.example` from `otp@domain.example`.

### 5.2 Direct SMTP — optional

```text
Sender → port 25 → built-in Go SMTP → SQLite
```

Use only when public inbound port 25 is known to work. On Armbian default install SMTP binds to loopback `127.0.0.1:2525` for local tests only.

### 5.3 Optional email copy

```text
Worker → TempMail always
Worker → configured destination(s) only when owner explicitly enables copies
```

Empty destination list means OFF. A copy destination must be verified by Cloudflare before forwarding works.

## 6. Functional requirements

### TempMail app

- `GET /api/setup` reports `needs_setup` (whether the users table is empty) so the login page can show the create-admin form.
- `POST /api/setup` creates first admin only when no users exist.
- `POST /api/auth/login` issues JWT.
- `POST /api/inbox` creates/extends inbox.
- `GET /api/inbox/:email/wait` waits up to bounded timeout.
- Message response includes parsed body, OTP codes, verification links, and attachment metadata.
- Inbox TTL default 24 hours; cleanup job purges stale data.
- Static UI is available under `/app/`.

### Menu

- Display dashboard before choices.
- Show OS/CPU, app health, memory, domain list, inbox/message count, backup status, Cloudflare profiles, tunnel, Worker state, routing state, optional copies.
- Add/select/remove Cloudflare profiles safely.
- Add/remove TempMail domains safely.
- Domain removal from menu never deletes Cloudflare zone/DNS/routing by default.
- Support worker deploy and routing enable per active profile/domain.

### Cloudflare

- Each profile stores account ID and token root-only on host.
- Each domain belongs to exactly one profile.
- Adding a domain must be doable from the web panel for a domain in the same Cloudflare account and for a domain in a different account (second profile), with no SSH step.
- Removing a domain from the panel leaves Cloudflare untouched by default; a domain that is still routing gets a second, clearly labelled action that disables the catch-all first, because a leftover catch-all would point at a Worker that no longer serves that domain.
- The Tunnel panel shows the host connector's real authentication mode, tunnel id, and ingress hostnames read by the root helper (`tunnel-config`), so the setup is understandable without a Cloudflare API token; when the connector uses `cert.pem`/credentials-file the panel warns that the logout action would delete that credential and stop the connector.
- Every domain has a readiness checklist that reads the real Cloudflare state: zone active, Worker deployed for the owning profile, catch-all routing pointed at that Worker, Email Routing DNS records present, and the web-host CNAME present and proxied.
- The checklist reports problems and can repair the safe subset (deploy Worker, enable routing, create missing Email Routing records, create the web-host CNAME). It never deletes DNS records, and a foreign MX is reported as a conflict for the owner to resolve.
- Tunnel can serve one web hostname and receive Worker traffic from domains across profiles.
- Catch-all changes require explicit confirmation.

### Server actions (admin panel)

- Panel can show service status, restart the app service, run the SQLite backup, read bounded journald logs, run doctor checks, and install/restart/logout the tunnel connector.
- Every privileged action goes through the root helper spool with a fixed action allowlist, validated parameters, and a result file; the app process itself never gains root.
- Helper absence degrades gracefully: the panel reports the helper as not installed and refuses privileged actions with `501`.

### Admin panel sections

- `Dashboard` (landing, visual: cincin kesiapan aspek, alur email Pengirim → CF Routing → Worker → Tunnel → Server → Inbox dengan warna per simpul, grafik batang aktivitas 24 jam, tiga ubin angka inbox/database/RAM dengan bar target RAM, kotak status delapan aspek, daftar hal yang butuh tindakan dengan judul singkat, dan pintasan ke panel lain), `Setup` (rel lima langkah berwarna dengan hitungan selesai di atas checklist, tiap langkah punya tombol aksi sendiri, dan tiap domain ditampilkan sebagai deretan titik aspek Worker/routing/Email DNS/CNAME), `Server` (aksi helper root sebagai kartu aksi berglyph, status unit systemd sebagai grid titik, ubin backup terakhir berisi nama/ukuran/umur/jumlah berkas, panjang log sebagai chip 50/100/200/500, keluaran mentah di blok monospace), `Diagnostik` (angka uptime/RAM/inbox/database lebih dulu, grid titik helper/aplikasi/database/Cloudflare/tunnel/salinan, baris domain dengan status routing, dan fakta mentah kedua endpoint status terlipat di bagian "Fakta mentah"), `Akun Cloudflare`, `Domain`, `Tunnel`, `Salinan email`, `Impor state`, `Pengguna` (create user, change role, reset password, delete), dan `Aturan nama` (edit the auto-generated inbox naming rules).
- The content column spans the available width (no fixed max width) so tables and riwayat log tidak menyisakan ruang kosong di layar lebar.
- Every panel lists its data with loading, empty, and error states, and every control calls a real endpoint.
- Panels stay scannable: status is shown as compact pills and numbers, alert and step text is a short headline with the long explanation kept in the element's `title`, and notes are one line. Panels must not explain the same fact twice.
- Status is drawn, not described: dots, rings, meters, and bars carry the state, tables carry the details, and raw values belong in a collapsed block (`Diagnostik` → "Fakta mentah", `Server` → the monospace output area). A panel whose whole content is a key-value list is a bug.
- Password fields are write-only: sent once, cleared, never rendered back.

## 7. Security requirements

- `JWT_SECRET` dan `EMAIL_API_KEY` tidak punya nilai bawaan yang bisa ditebak. Kalau environment tidak menyetelnya, aplikasi membuat nilai acak sekali dan menyimpannya di `<data dir>/secrets.env` dengan izin 0600; nilai dari environment selalu menang, dan berkas yang tidak bisa ditulis membuat aplikasi berhenti dengan pesan yang jelas (bukan diam-diam memakai kunci sementara).
- Alur pertama kali harus bisa diselesaikan dari browser: aplikasi dinyalakan, halaman login menawarkan pembuatan admin pertama, lalu seluruh setup Cloudflare dilakukan di panel. CLI hanya untuk menyalakan aplikasi, port, dan jaringan.
- `EMAIL_API_KEY` must protect Worker receive endpoint.
- Never commit credentials, database files, cloudflared credentials, token files, or built binaries.
- Copies are explicit opt-in.
- Input validation at all HTTP boundaries.
- Run public app behind Cloudflare Tunnel/TLS. No router port-forward requirement.
- The app runs unprivileged; privileged host actions go through an allowlisted root helper, never through a shell.
- Non-admin users only reach their own inboxes; admin and API-key callers are unrestricted by design.
- Ingestion is size capped and can require an HMAC signature from the Worker.

## 8. Reliability requirements

- systemd restarts app after crash on Linux.
- SQLite WAL enabled.
- Daily SQLite backups retained seven days.
- Cloudflare Tunnel reconnects via service.
- Email copy is not assumed as primary storage; TempMail must report delivery failures in Worker logs.

## 9. Success criteria

- Fresh Linux/Armbian deployment reaches `/health`.
- Admin can login to `/app/`.
- Local SMTP or HTTP Worker simulation stores a message.
- OTP/link extraction succeeds for test message.
- `menutempmail` shows accurate dashboard state.
- A domain from CF A and a domain from CF B can route to the same TempMail instance after their respective Worker/routing setup.
- A domain in a second Cloudflare account can be added, Worker-deployed for that profile, and routed through the panel alone, with the readiness checklist showing the real Cloudflare state at each step.
- The panel `Setup` checklist drives a first install end to end: helper, profile, tunnel plus web-host CNAME, domain plus Worker/routing/Email DNS, and a receiving test inbox.
- The admin panel (`/admin`) can add a Cloudflare profile, add a domain, deploy the Worker, and enable routing, with Cloudflare errors shown verbatim.
- Menu and panel report the same profiles, domains, routing state, tunnel state, and copy destinations from one source of truth.
- Panel `Server` section restarts the service, runs the backup, shows journald logs, runs doctor, and installs/restarts/logs out the tunnel through the root helper, while the app process itself stays unprivileged.

## 10. Roadmap

1. Verify the multi-profile Cloudflare control panel against real accounts (implementation done: profiles, domains, routing, Worker deploy, tunnel status, copies, state import through `/api/admin/cloudflare/*`).
2. Inbox ownership checks for non-admin users (done: `inboxes.owner_id` recorded on web creation and enforced on read/wait/delete/message/attachment/SSE, admin and API-key callers unrestricted).
3. Bounded raw-email size and signed Worker requests (done: `MAX_RAW_EMAIL_BYTES` on HTTP and SMTP, optional `WORKER_SIGNING_SECRET` with `X-Email-Signature` HMAC-SHA256).
4. Add release pipeline with Linux arm64/amd64 binaries.
5. Implement/test launchd (macOS) and Windows Service adapters.
6. Add automatic Cloudflare routing status reconciliation.
