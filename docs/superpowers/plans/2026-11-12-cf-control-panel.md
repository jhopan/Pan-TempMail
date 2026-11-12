# Cloudflare Control Panel Implementation Plan

> **For agentic workers:** this file is the frozen interface contract. Do not invent names, fields, or endpoints that are not listed here. Steps use `- [ ]` for tracking.

**Goal:** Kelola akun Cloudflare, domain, Worker, routing, tunnel, dan copy email dari web admin panel, dengan SQLite sebagai sumber data tunggal; `menutempmail` menjadi klien API yang sama.

**Architecture:** Satu paket klien Cloudflare REST (`receiver/cloudflare`), satu tabel baru per entitas (`cf_profiles`, `cf_domains`) plus setting di tabel `settings` yang sudah ada, satu grup route JWT+admin `/api/admin/cloudflare/*`, halaman admin vanilla HTML/JS di `receiver/web/static`, dan `tempmailctl` yang memanggil API lokal. Operasi root (pasang/restart `cloudflared`) tetap di bash; panel web hanya lewat Cloudflare API.

**Tech Stack:** Go 1.25 (module `receiver`), gin, modernc sqlite, net/http + mime/multipart untuk upload Worker, crypto/aes untuk penyimpanan token, vanilla HTML/CSS/JS tanpa CDN.

**Decisions locked (user):** antislop DURING; tunnel = Cloudflare API + status read-only, tanpa sudoers baru; SQLite = sumber data tunggal, menu jadi klien API.

**Design Read (antislop):** Reading this as: ops console for a self-hosted temp-mail instance, following the existing product identity (dark surface `#0a0a0f`/`#12121a`, purple accent `#8b5cf6`, system font stack), dial ENERGY 1 / RHYTHM 1 / MOTION 1 (data-dense tool, no decoration).

---

## Frozen contract

### Env vars (new)

| Var | Default | Purpose |
|---|---|---|
| `CLOUDFLARE_API_BASE` | `https://api.cloudflare.com/client/v4` | Test seam for Cloudflare REST calls |
| `CF_WORKER_NAME` | `tempmailbyjhopanstore` | Worker script name |
| `CF_WORKER_COMPAT_DATE` | `2024-01-01` | Worker compatibility date |
| `CF_TUNNEL_NAME` | `tempmailbyjhopanstore` | Tunnel name used for status lookup |

### Settings keys (existing `settings` table)

`cf_active_profile`, `cf_tunnel_web_host`, `cf_tunnel_name`, `cf_tunnel_id`, `cf_copies`

### Database (append to the `queries` slice in `receiver/database/postgres.go` `migrate()`)

```sql
CREATE TABLE IF NOT EXISTS cf_profiles (
  id TEXT PRIMARY KEY,
  label TEXT NOT NULL DEFAULT '',
  account_id TEXT NOT NULL,
  token TEXT NOT NULL DEFAULT '',
  worker_deployed INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS cf_domains (
  domain TEXT PRIMARY KEY,
  profile_id TEXT NOT NULL,
  zone_id TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 0,
  routing INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_cf_domains_profile ON cf_domains(profile_id);
```

### Package `receiver/cloudflare`

```go
type APIError struct { Status int; Codes []int; Messages []string }
func (e *APIError) Error() string            // "cloudflare 403 (9109): Invalid access token"
var ErrZoneNotFound = errors.New("cloudflare: zone not found or not active")

type Account struct { ID, Name string }
type Tunnel struct { ID, Name, Status string; Connections int; CreatedOn string }
type Binding struct { Name, Type, Text string }   // Type: "secret_text" | "plain_text"
type Deployment struct { ID, ModifiedOn string }

func NewClient(token string) *Client                      // base from CLOUDFLARE_API_BASE, timeout 20s
func NewClientWithBase(token, baseURL string) *Client
func (c *Client) Account(accountID string) (Account, error)                     // GET /accounts/{id}
func (c *Client) ZoneID(domain string) (string, error)                          // GET /zones?name={domain}
func (c *Client) EnableEmailRouting(zoneID string) error                        // POST /zones/{id}/email/routing/enable
func (c *Client) UnlockEmailRoutingDNS(zoneID string) error                     // DELETE /zones/{id}/email/routing/dns
func (c *Client) SetCatchAll(zoneID, workerName string, enabled bool) error     // PUT /zones/{id}/email/routing/rules/catch_all
func (c *Client) DeployWorker(accountID, name, compatDate, script string, bindings []Binding) (Deployment, error)
func (c *Client) SetWorkerSubdomain(accountID, name string, enabled, previews bool) error
func (c *Client) DeleteWorker(accountID, name string) error
func (c *Client) ListTunnels(accountID string) ([]Tunnel, error)
func (c *Client) CreateTunnel(accountID, name string) (Tunnel, error)
func (c *Client) TunnelToken(accountID, tunnelID string) (string, error)

func SealSecret(plain string) (string, error)   // AES-256-GCM, key SHA-256("tempmail:cf:"+JWT_SECRET), output "v1:"+base64(nonce||ciphertext)
func OpenSecret(sealed string) (string, error)   // input tanpa prefix "v1:" dikembalikan apa adanya (impor state lama)
```

Cloudflare response envelope: `{"success":bool,"errors":[{"code":int,"message":string}],"result":...}`. Setiap non-2xx atau `success=false` menjadi `*APIError`.

### Package `receiver/worker`

```go
func Script() string      // //go:embed script.js  (salinan byte-identik cloudflare-worker/src/index.js)
func CompatDate() string  // env CF_WORKER_COMPAT_DATE
func Name() string        // env CF_WORKER_NAME
```

### `receiver/repository/cf_repository.go`

```go
type CFProfile struct { ID, Label, AccountID string; WorkerDeployed bool; CreatedAt, UpdatedAt time.Time }
type CFDomain  struct { Domain, ProfileID, ZoneID string; Enabled, Routing bool; CreatedAt, UpdatedAt time.Time }

var ErrCFNotFound = errors.New("cf record not found")
const SettingActiveProfile = "cf_active_profile"
const SettingTunnelWebHost = "cf_tunnel_web_host"
const SettingTunnelName    = "cf_tunnel_name"
const SettingCopies        = "cf_copies"

func ListCFProfiles(db *sql.DB) ([]CFProfile, error)
func GetCFProfile(db *sql.DB, id string) (CFProfile, error)
func CreateCFProfile(db *sql.DB, p CFProfile) error
func UpdateCFProfileMeta(db *sql.DB, id, label, accountID string) error
func SetCFProfileToken(db *sql.DB, id, sealed string) error
func GetCFProfileToken(db *sql.DB, id string) (string, error)
func SetCFProfileWorkerDeployed(db *sql.DB, id string, deployed bool) error
func DeleteCFProfile(db *sql.DB, id string) error
func CountCFDomainsForProfile(db *sql.DB, id string) (int, error)

func ListCFDomains(db *sql.DB) ([]CFDomain, error)
func GetCFDomain(db *sql.DB, domain string) (CFDomain, error)
func UpsertCFDomain(db *sql.DB, d CFDomain) error
func SetCFDomainRouting(db *sql.DB, domain string, routing bool) error
func DeleteCFDomain(db *sql.DB, domain string) error
func ListedDomains(db *sql.DB) ([]string, error)   // enabled=1 AND routing=1, urut alfabetis
```

Semua query lewat `placeholder()`; maksimum 9 parameter per query.

### HTTP endpoints (grup `admin` yang sudah ada: JWT + AdminMiddleware + rate limit)

| Method | Path | Body | Sukses |
|---|---|---|---|
| GET | `/api/admin/cloudflare/status` | - | `200` status agregat |
| GET | `/api/admin/cloudflare/profiles` | - | `200 [Profile]` |
| POST | `/api/admin/cloudflare/profiles` | `{"id","label","account_id","token","activate":bool}` | `201 Profile` |
| PUT | `/api/admin/cloudflare/profiles/:id` | `{"label"?,"account_id"?,"token"?}` | `200 Profile` |
| DELETE | `/api/admin/cloudflare/profiles/:id` | - | `200 {"deleted":true}` |
| POST | `/api/admin/cloudflare/profiles/:id/activate` | - | `200 {"active_profile":"id"}` |
| POST | `/api/admin/cloudflare/profiles/:id/deploy-worker` | `{}` | `200 {"profile","worker","modified_on","secrets":["TEMPMAIL_SERVER_URL","TEMPMAIL_API_KEY","FORWARD_LIST"]}` |
| DELETE | `/api/admin/cloudflare/profiles/:id/worker` | - | `200 {"deleted":true}` |
| GET | `/api/admin/cloudflare/domains` | - | `200 [Domain]` |
| POST | `/api/admin/cloudflare/domains` | `{"domain","profile"?}` | `201 Domain` |
| DELETE | `/api/admin/cloudflare/domains/:domain` | - | `200 {"deleted":true,"cloudflare_untouched":true}` |
| POST | `/api/admin/cloudflare/domains/:domain/routing` | `{"routing":bool,"unlock_dns":bool}` | `200 Domain` |
| GET | `/api/admin/cloudflare/tunnel` | - | `200 Tunnel` |
| PUT | `/api/admin/cloudflare/tunnel` | `{"web_host"?,"name"?}` | `200 Tunnel` |
| POST | `/api/admin/cloudflare/tunnel/create` | `{"name"?,"web_host"?}` | `200 {"id","name","token","commands":["..."]}` |
| GET | `/api/admin/cloudflare/copies` | - | `200 {"destinations":["..."]}` |
| PUT | `/api/admin/cloudflare/copies` | `{"destinations":["..."]}` | `200 {"destinations":["..."],"applied_to":["profile"]}` |
| POST | `/api/admin/cloudflare/import` | `{"state":{...}}` | `200 {"imported":{"profiles":n,"domains":n,"tunnel":bool,"active_profile":"id"}}` |

Shapes:

```json
Profile   {"id","label","account_id","active":true,"has_token":true,"worker_deployed":false,"domain_count":2}
Domain    {"domain","profile","zone_id","enabled":false,"routing":false}
Tunnel    {"name","id","web_host","status","connections","account_id","source":"api|config|none","error":""}
status    {"profiles":[Profile],"active_profile":"id","domains":[Domain],"tunnel":Tunnel,
           "copies":["a@b.c"],"worker_name":"...","worker_compat_date":"...","env_domains":["..."],
           "api_base":"https://api.cloudflare.com/client/v4"}
Error     {"error":"pesan singkat","detail":"pesan mentah dari Cloudflare bila ada"}
```

Aturan: respons TIDAK PERNAH memuat token (`has_token` boolean saja). Pesan error Cloudflare diteruskan di `detail`, tidak ditelan.

### Perubahan perilaku lain

- `web.ConfigHandler` (`/api/config`): daftar domain diambil dari `repository.ListedDomains()`; kalau kosong, fallback `DOMAINS` env, kalau masih kosong `example.com`.
- `handlers.CreateInboxAPI`: domain default diambil dari `ListedDomains()` pertama, fallback `DOMAINS` env.
- Worker script yang di-deploy via API sama dengan `cloudflare-worker/src/index.js`; test menjaga byte-identik.

---

## Tasks

### Task 1: Cloudflare client (owner: subagent A)

**Files:** Create `receiver/cloudflare/client.go`, `receiver/cloudflare/errors.go`, `receiver/cloudflare/secret.go`, `receiver/cloudflare/client_test.go`, `receiver/cloudflare/secret_test.go`.

- [ ] Test dulu (`httptest` stub + `NewClientWithBase`): envelope sukses, `success=false` → `*APIError` dengan `Codes`/`Messages`, `ZoneID` → `ErrZoneNotFound` saat kosong atau status `pending`, body `catch_all` persis `{"enabled":true,"name":"Pan-TempMail catch-all","matchers":[{"type":"all"}],"actions":[{"type":"worker","value":["tempmailbyjhopanstore"]}]}`, multipart `DeployWorker` memuat metadata `main_module=index.js` + `compatibility_date` + bindings, `SealSecret`/`OpenSecret` round-trip dan passthrough tanpa prefix.
- [ ] Implementasi sampai test hijau.
- [ ] `cd receiver && gofmt -l . && go test ./cloudflare/... && go vet ./cloudflare/...`

**Acceptance:** test hijau, tidak ada panggilan jaringan nyata di test, tidak ada token yang tercetak di `Error()`.

### Task 2: Worker asset (owner: subagent A)

**Files:** Create `receiver/worker/script.js` (salinan `cloudflare-worker/src/index.js`), `receiver/worker/worker.go`, `receiver/worker/worker_test.go`.

- [ ] Test `TestScriptMatchesRepoWorker` membaca `../../cloudflare-worker/src/index.js` dan gagal kalau berbeda byte.
- [ ] `worker.go` dengan `//go:embed script.js`, `Script()`, `Name()`, `CompatDate()`.
- [ ] `gofmt`, `go test ./worker/...`.

### Task 3: Repository + migration (owner: subagent B)

**Files:** Create `receiver/repository/cf_repository.go`, `receiver/repository/cf_repository_test.go`; modify `receiver/database/postgres.go` (`migrate()` queries slice).

- [ ] Test dengan SQLite temporer (`t.TempDir()`): CRUD profil, `ErrCFNotFound`, token tersimpan/terbaca, `UpsertCFDomain` idempoten, `SetCFDomainRouting`, `CountCFDomainsForProfile`, `ListedDomains` hanya `enabled && routing` dan terurut.
- [ ] Implementasi; semua query pakai `placeholder()`.
- [ ] `gofmt`, `go test ./repository/... ./database/...`, `go vet ./...`

### Task 4: Admin handlers + router + domain source (owner: lead)

**Files:** Create `receiver/handlers/cf_handler.go`, `receiver/handlers/cf_handler_test.go`; modify `receiver/api/router.go`, `receiver/web/web.go`, `receiver/handlers/inbox_handler.go`.

- [ ] Test handler memakai temp SQLite + stub Cloudflare (`CLOUDFLARE_API_BASE` menunjuk `httptest`): create profil validasi token, tolak id invalid, hapus profil dengan domain → `409`, `routing` meneruskan error Cloudflare, respons tidak memuat token.
- [ ] Implementasi seluruh endpoint tabel di atas.

### Task 5: Web admin UI (owner: subagent D)

**Files:** Create `receiver/web/static/admin.html`, `receiver/web/static/admin.js`; modify `receiver/web/static/index.html` (link Admin di topbar untuk role admin).

- [ ] Halaman admin: profil CF (tambah/pilih/hapus/deploy worker), domain (tambah/hapus/toggle routing + unlock DNS), tunnel (nama/ID/status, set web host, buat tunnel + perintah root), copy destinations, impor state lama, tombol diagnostik ke `/health`.
- [ ] State UI wajib: loading, kosong, error (R-27); kontrol keyboard + focus visible (R-32); tap target ≥44px dan tanpa overflow di 360px (R-03).
- [ ] Tanpa CDN, tanpa framework, CSS inline mengikuti identitas app (dark + `#8b5cf6`).

### Task 6: Menu jadi klien API (owner: subagent E)

**Files:** Modify `deploy/linux/tempmailctl` (`api_login`, `api_get`, `api_post`, dan fungsi profil/domain/worker/routing/copy/dashboard), `deploy/linux/install.sh` (tulis `CLOUDFLARE_API_BASE`, `CF_WORKER_NAME`).

- [ ] `api_login` memakai `ADMIN_EMAIL`/`ADMIN_PASS` dari `/etc/tempmail/tempmail.env` → JWT; `TM_API_BASE` bisa dioverride untuk uji lokal.
- [ ] Fungsi root-only (`tunnel_login`, `create_tunnel`, `logout_*`, `reset_all`) tetap bash.
- [ ] Tambah submenu impor `cloudflare-state.json` ke DB.
- [ ] Verifikasi: `bash -n deploy/linux/tempmailctl`, plus harness fungsi API terhadap app lokal berjalan.

### Task 7: Docs + env example (owner: subagent F)

**Files:** Modify `docs/API.md`, `docs/PRD.md`, `docs/ARCHITECTURE.md`, `docs/RUNBOOK.md`, `README.md`, `deploy/linux/DEPLOYMENT.md`? (pakai `docs/DEPLOYMENT.md`), `.env.example`.

- [ ] Dokumentasikan endpoint baru, env baru, alur multi-akun via web, batas tunnel tanpa root, dan prosedur impor state.

### Task 8: Integrasi + verifikasi live (owner: lead)

- [ ] `cd receiver && gofmt -l . && go test ./... && go vet ./...`
- [ ] Jalankan binary lokal dengan stub Cloudflare, login admin, klik semua kontrol panel di browser, catat bukti per elemen (R-35).
- [ ] Uji: impor state lama → domain muncul di `/api/config` tanpa restart, toggle routing menampilkan error Cloudflare apa adanya, deploy worker multipart tervalidasi stub.
- [ ] Delivery Gate antislop sebagai laporan PASS/FAIL.
