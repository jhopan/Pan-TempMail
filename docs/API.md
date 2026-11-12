# TempMail By JhopanStore — API Documentation

Complete guide for using TempMail automation API.

## Base Info

```text
Production URL:  https://your-domain.com
Local URL:        http://127.0.0.1:8080 (or your PORT)
```

## Authentication

All automation endpoints use `X-Email-API-Key` header. No JWT login required.

```text
X-Email-API-Key: your-api-key
```

Get your API key from:
- **Production:** `/etc/tempmail/tempmail.env` on the server (field `EMAIL_API_KEY`)
- **Local:** `receiver/.env` (field `EMAIL_API_KEY`)
- **Generate new:** `openssl rand -hex 32`

Web UI requires separate JWT login (email + password). API key is for automation only.

## Endpoint Reference

### 1. Create Inbox

Create a temporary email inbox.

**Request:**
```http
POST /api/inbox
Content-Type: application/json
X-Email-API-Key: your-api-key

{
  "domain": "your-domain.com"
}
```

**With custom username (you control the name):**
```json
{
  "username": "your-name",
  "domain": "your-domain.com"
}
```

**Response:**
```json
{
  "email": "your-name@your-domain.com",
  "expires": "24h",
  "inbox_id": 12,
  "sse_url": "/sse/your-name@your-domain.com?token=<JWT>",
  "wait_url": "/inbox/your-name@your-domain.com/wait"
}
```

**Parameters:**

| Field | Required | Description |
|---|---|---|
| `domain` | Yes | Domain from configured DOMAINS |
| `username` | No | Custom username. If empty, auto-generated from naming rules |

**Example (curl):**
```bash
# Auto-generated name
curl -s -X POST "https://your-domain.com/api/inbox" \
  -H "Content-Type: application/json" \
  -H "X-Email-API-Key: your-api-key" \
  -d '{"domain":"your-domain.com"}'

# Custom username
curl -s -X POST "https://your-domain.com/api/inbox" \
  -H "Content-Type: application/json" \
  -H "X-Email-API-Key: your-api-key" \
  -d '{"username":"your-name","domain":"your-domain.com"}'
```

---

### 2. Wait for Email (Blocking)

Block until an email arrives. Auto-returns when email is received. No polling needed.

**Request:**
```http
GET /api/inbox/{email}/wait
X-Email-API-Key: your-api-key
```

**Response (when email arrives):**
```json
{
  "id": 8,
  "from": "verify@cloudflare.com",
  "subject": "Verify your email",
  "codes": ["192834"],
  "links": ["https://verify.cloudflare.com/confirm?token=abc123"],
  "text_body": "Your verification code is 192834. Click: https://verify.cloudflare.com/confirm?token=abc123",
  "html_body": "",
  "received_at": "2026-08-28T14:29:50Z"
}
```

**Parameters:**

| Field | In | Required | Description |
|---|---|---|---|
| `email` | URL path | Yes | Full email address (URL-encoded) |
| `timeout` | Query (optional) | No | Max seconds to wait (default: 30) |

**Example:**
```bash
curl -s "https://your-domain.com/api/inbox/your-name@your-domain.com/wait" \
  -H "X-Email-API-Key: your-api-key"
```

**With custom timeout (60 seconds):**
```bash
curl -s "https://your-domain.com/api/inbox/your-name@your-domain.com/wait?timeout=60" \
  -H "X-Email-API-Key: your-api-key"
```

**Response fields:**

| Field | Type | Description |
|---|---|---|
| `id` | int | Message ID (use for GET /message/{id}) |
| `from` | string | Sender email address |
| `subject` | string | Email subject |
| `codes` | array | Extracted OTP/verification codes |
| `links` | array | Extracted verification URLs |
| `text_body` | string | Plain text email body |
| `html_body` | string | HTML email body |
| `received_at` | string | Timestamp (ISO 8601 UTC) |

---

### 3. List Messages in Inbox

Get all messages in an inbox.

**Request:**
```http
GET /api/inbox/{email}
X-Email-API-Key: your-api-key
```

**Response:**
```json
{
  "data": [
    {
      "id": 8,
      "sender": "verify@cloudflare.com",
      "subject": "Verify your email",
      "received_at": "2026-08-28T14:29:50Z",
      "has_attachment": false
    }
  ],
  "total": 1,
  "page": 1,
  "per_page": 20,
  "total_pages": 1
}
```

**Example:**
```bash
curl -s "https://your-domain.com/api/inbox/your-name@your-domain.com" \
  -H "X-Email-API-Key: your-api-key"
```

---

### 4. Get Message Detail

Get full message content including extracted codes and links.

**Request:**
```http
GET /api/message/{id}
X-Email-API-Key: your-api-key
```

**Response:**
```json
{
  "id": 8,
  "sender": "verify@cloudflare.com",
  "subject": "Verify your email",
  "codes": ["192834"],
  "links": ["https://verify.cloudflare.com/confirm?token=abc123"],
  "text_body": "Your verification code is 192834...",
  "html_body": "",
  "received_at": "2026-08-28T14:29:50Z"
}
```

**Example:**
```bash
curl -s "https://your-domain.com/api/message/8" \
  -H "X-Email-API-Key: your-api-key"
```

---

### 5. Delete Message

Delete a single message.

**Request:**
```http
DELETE /api/message/{id}
X-Email-API-Key: your-api-key
```

**Response:** `204 No Content`

**Example:**
```bash
curl -s -X DELETE "https://your-domain.com/api/message/8" \
  -H "X-Email-API-Key: your-api-key"
```

---

### 6. Delete All Messages in Inbox

Clear all messages for an email address.

**Request:**
```http
DELETE /api/inbox/{email}
X-Email-API-Key: your-api-key
```

**Response:** `204 No Content`

**Example:**
```bash
curl -s -X DELETE "https://your-domain.com/api/inbox/your-name@your-domain.com" \
  -H "X-Email-API-Key: your-api-key"
```

---

### 7. List Attachments

Get attachments for a message.

**Request:**
```http
GET /api/message/{id}/attachments
X-Email-API-Key: your-api-key
```

**Response:**
```json
[
  {
    "id": 1,
    "filename": "document.pdf",
    "size": 102400,
    "content_type": "application/pdf"
  }
]
```

**Example:**
```bash
curl -s "https://your-domain.com/api/message/8/attachments" \
  -H "X-Email-API-Key: your-api-key"
```

---

### 8. Download Attachment

Download a specific attachment.

**Request:**
```http
GET /api/attachment/{id}
X-Email-API-Key: your-api-key
```

**Response:** Binary file (original content type)

**Example:**
```bash
curl -s -o document.pdf "https://your-domain.com/api/attachment/1" \
  -H "X-Email-API-Key: your-api-key"
```

---

### 9. Get Config

Public endpoint — no auth required. Returns domains and site name only (no secrets).

**Request:**
```http
GET /api/config
```

**Response:**
```json
{
  "domains": ["your-domain.com"],
  "site_name": "TempMail By JhopanStore"
}
```

---

### 10. Health Check

Public endpoint — no auth required.

**Request:**
```http
GET /health
```

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2026-08-28T14:29:50Z"
}
```

---

### 11. Worker Ingestion (Internal)

Used by Cloudflare Worker to forward emails. Not for automation scripts.

**Request:**
```http
POST /api/email/receive
Content-Type: application/json
X-Email-API-Key: your-api-key
X-Email-Signature: sha256=<hmac-sha256-hex-of-the-raw-body>   (wajib bila WORKER_SIGNING_SECRET diset)

{
  "to": "your-name@your-domain.com",
  "from": "verify@cloudflare.com",
  "raw": "From: verify@cloudflare.com\nTo: your-name@...\nSubject: Verify\n\nBody text"
}
```

Batas dan tanda tangan:

| Hal | Aturan |
|---|---|
| Ukuran | `raw` maksimum `MAX_RAW_EMAIL_BYTES` (default 5 MiB). Melebihi batas, atau body request lebih besar dari batas + 4 KiB, dibalas `413 {"error":"Email terlalu besar","limit_bytes":...}` sebelum parsing. Batas yang sama dipakai jalur SMTP. |
| Tanda tangan | Bila `WORKER_SIGNING_SECRET` diisi di `tempmail.env`, header `X-Email-Signature` wajib dan harus `sha256=` + HMAC-SHA256 dari body mentah memakai secret itu. Salah atau kosong dibalas `401 {"error":"Signature Worker tidak valid"}`. Bila secret kosong, tanda tangan tidak diminta (perilaku lama) dan server mencatat satu kali di log bahwa signing OFF. |
| Deploy | Saat secret terisi, panel menambahkan binding `TEMPMAIL_SIGNING_SECRET` ke Worker sehingga Worker ikut menandatangani. Worker versi baru juga melaporkan `signed: true` di endpoint `fetch` miliknya. |

**Response:**
```json
{
  "message_id": 8,
  "status": "received",
  "from": "verify@cloudflare.com",
  "to": "your-name@your-domain.com",
  "subject": "Verify",
  "codes": ["192834"],
  "links": ["https://verify.cloudflare.com/confirm?token=abc123"],
  "attachments": 0
}
```

---

### 12. Cloudflare Control API (admin panel)

Used by the web admin panel (`/admin`, which serves `/app/admin.html`) and by `menutempmail`. The panel
is not linked from the inbox UI. Requires an admin JWT, not the automation key. Panel writes are the
single source of truth: SQLite, not `/etc/tempmail/cloudflare-state.json`.

**Auth:** `POST /api/auth/login` with an admin account, then send `Authorization: Bearer <token>`.
Every route below lives under `/api/admin/cloudflare` and requires role `admin`.

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/status` | - | aggregate state (see below) |
| GET | `/profiles` | - | profile list |
| POST | `/profiles` | `{"id","label","account_id","token","activate"}` | created profile |
| PUT | `/profiles/{id}` | `{"label"?,"account_id"?,"token"?}` | updated profile |
| DELETE | `/profiles/{id}` | - | `{"deleted":true}` (409 while domains use it) |
| POST | `/profiles/{id}/activate` | - | `{"active_profile":"id"}` |
| POST | `/profiles/{id}/deploy-worker` | `{}` | deploy result |
| DELETE | `/profiles/{id}/worker` | - | `{"deleted":true}` |
| GET | `/domains` | - | domain list |
| POST | `/domains` | `{"domain","profile"?}` | created domain |
| DELETE | `/domains/{domain}` | - | `{"deleted":true,"cloudflare_untouched":true}` |
| POST | `/domains/{domain}/routing` | `{"routing":true,"unlock_dns":false}` | updated domain (1) |
| GET | `/domains/{domain}/check` | - | readiness checklist (zone, Worker, routing, Email DNS, web-host CNAME) |
| POST | `/domains/{domain}/fix` | `{"cname"?,"routing"?,"worker"?,"email_dns"?,"overwrite_cname"?}` | `{"done":[...],"skipped":[...],"failed":[...]}` |
| GET | `/tunnel` | - | tunnel status |
| PUT | `/tunnel` | `{"web_host","name"}` | tunnel status |
| POST | `/tunnel/create` | `{"name","web_host"}` | tunnel id, connector token, install commands |
| POST | `/tunnel/cname` | `{}` | ensures the web-host CNAME: `{"web_host","expected","created","existing","zone"}` |
| GET | `/copies` | - | `{"destinations":[...]}` |
| PUT | `/copies` | `{"destinations":[...]}` | destinations plus profiles that need a redeploy |
| POST | `/import` | `{"state":{...}}` | counts imported from an old state file |

**Profile shape:** `{"id","label","account_id","active","has_token","worker_deployed","domain_count"}`.
**Domain shape:** `{"domain","profile","zone_id","enabled","routing"}`.
**Tunnel shape:** `{"name","id","web_host","status","connections","account_id","source","error"}`.
**`/status`** adds `active_profile`, `copies`, `worker_name`, `worker_compat_date`, `env_domains`, `api_base`.

**`/domains/{domain}/check` shape:**

```json
{"domain":"x.tld","profile":"cf-a",
 "zone":{"id":"...","name":"x.tld","status":"active"},
 "worker":{"name":"tempmailbyjhopanstore","deployed":true},
 "routing":{"enabled":true,"target":"tempmailbyjhopanstore","ok":true},
 "email_dns":{"ready":true,
              "missing":[{"type":"MX","name":"x.tld","content":"route1.mx.cloudflare.net","priority":1}],
              "conflicts":[{"type":"MX","name":"x.tld","content":"mail.other.tld","priority":5}]},
 "web_host":{"configured":true,"host":"mail.example.tld","expected":"<tunnel-id>.cfargotunnel.com",
             "zone":{"id":"...","name":"example.tld","status":"active"},
             "records":[{"type":"CNAME","name":"mail.example.tld","content":"<tunnel-id>.cfargotunnel.com","proxied":true}],
             "ok":true},
 "issues":[{"key":"cname","level":"warn","message":"...","fixable":true}]}
```

`issues[].level` is `ok`, `warn`, or `bad`; `fixable:true` only appears for `worker`, `routing`,
`email_dns`, and `cname`. A failing Cloudflare call is reported as an issue with the raw message
rather than a `5xx`, so one broken call never hides the rest of the checklist.

**`/domains/{domain}/fix`** only touches what is selected: `worker` redeploys the Worker for that
domain's profile, `routing` enables Email Routing and points the catch-all at the Worker, `email_dns`
creates missing Email Routing records, and `cname` creates the web-host CNAME. It never deletes a
record, and an existing CNAME with a different target is skipped unless `overwrite_cname` is true.
`mx_conflict` is reported and never fixed: a non-Cloudflare MX must be removed by the owner.

**Errors:** `{"error":"short message","detail":"raw Cloudflare message"}`. The Cloudflare
detail is always forwarded, so a rejected token, a pending zone, or a non-Cloudflare MX setup
shows the real reason instead of a silent failure.

**Behaviour worth knowing:**

- Adding a domain resolves its zone with the profile token and refuses zones that are missing or not `active`.
- Adding a domain from a **different Cloudflare account** is the same flow with a second profile: create the profile, then add the domain and select that profile. The Worker is uploaded per profile, so every profile needs its own `deploy-worker` before its domains can route.
- DNS writes are additive only: `POST /tunnel/cname` and the `cname`/`email_dns` parts of `POST /domains/{domain}/fix` create what is missing. No endpoint deletes DNS records.
- `routing:true` enables Email Routing, then sets the catch-all to the Worker. `routing:false` disables the catch-all only.
- (1) `unlock_dns:true` is **destructive and off by default**: Cloudflare documents `DELETE /zones/{id}/email/routing/dns` as *"Disable your Email Routing zone. Also removes additional MX records previously required for Email Routing to work."* Mail for that domain stops until the Email Routing records exist again, which `POST /domains/{domain}/fix` with `email_dns:true` can restore. The panel labels it "matikan CF" and asks for confirmation; the setup scripts never send it.
- Deleting a domain removes it from TempMail only; Cloudflare DNS, zone, and routing stay untouched.
- `deploy-worker` uploads the embedded Worker script and sets `TEMPMAIL_SERVER_URL`, `TEMPMAIL_API_KEY`, and `FORWARD_LIST` as Worker secrets. No `wrangler` or Node.js is needed on the host.
- API tokens are stored encrypted (AES-256-GCM, key derived from `JWT_SECRET`) and are never returned by any endpoint. Rotating `JWT_SECRET` invalidates stored tokens, so re-enter them after a rotation.
- `tunnel/create` returns the connector token and the root commands to install `cloudflared`; the app never writes `/etc/cloudflared` or restarts the tunnel service itself.

**Example:**

```bash
BASE=http://127.0.0.1:8081
JWT=$(curl -s -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.invalid","password":"your-password"}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')

curl -s "$BASE/api/admin/cloudflare/status" -H "Authorization: Bearer $JWT"

curl -s -X POST "$BASE/api/admin/cloudflare/profiles" -H "Authorization: Bearer $JWT" \
  -H 'Content-Type: application/json' \
  -d '{"id":"cf-adelian","label":"Adelian","account_id":"<account-id>","token":"<api-token>","activate":true}'

curl -s -X POST "$BASE/api/admin/cloudflare/domains" -H "Authorization: Bearer $JWT" \
  -H 'Content-Type: application/json' -d '{"domain":"domain-b.example"}'

curl -s -X POST "$BASE/api/admin/cloudflare/domains/domain-b.example/routing" \
  -H "Authorization: Bearer $JWT" -H 'Content-Type: application/json' \
  -d '{"routing":true,"unlock_dns":true}'
```

### 13. Admin Users and Naming Rules (panel)

Dipakai panel `/admin` pada seksi `Pengguna` dan `Aturan nama`. Butuh JWT dengan role admin.

| Method | Path | Body | Hasil |
|---|---|---|---|
| GET | `/api/admin/users` | - | array user `{id,email,role,created_at,updated_at}` |
| POST | `/api/admin/users` | `{"email","password","role"}` | `201` user baru; `409` bila email sudah ada |
| PUT | `/api/admin/users/:id` | `{"email","role"}` (keduanya wajib) | user terbaru |
| PUT | `/api/admin/users/:id/password` | `{"password"}` (min 6) | `204` |
| DELETE | `/api/admin/users/:id` | - | `204`; menolak menghapus akun sendiri |
| GET | `/api/admin/naming-rules` | - | objek aturan nama |
| PUT | `/api/admin/naming-rules` | objek aturan penuh | aturan tersimpan |

Bentuk aturan nama (`naming.Rules`): `template`, `separator`, `first_words[]`, `second_words[]`,
`num_min`, `num_max`, `num_probability`. Token template yang dikenali: `{first}`, `{second}`, `{sep}`,
`{num}`, `{rand:N}`, `{ts}`.

Aturan kepemilikan inbox (berlaku untuk semua endpoint inbox/pesan): user non-admin hanya bisa membuka
inbox miliknya sendiri (`inboxes.owner_id` sama dengan user yang login); inbox tanpa owner (dibuat
otomasi) hanya bisa dibuka admin atau pemanggil `X-Email-API-Key`; admin bebas. Pelanggaran dibalas
`403 {"error":"Inbox ini bukan milik akun kamu"}`.

### 14. Server Actions API (root helper)

The admin panel's `Server` section uses these. Requires an admin JWT. The app itself is unprivileged:
requests go to the root helper through `/opt/tempmail/data/helper` (see `docs/RUNBOOK.md`).

| Method | Path | Body | Response |
|---|---|---|---|
| GET | `/api/admin/system/status` | - | local facts, never blocks on the helper |
| POST | `/api/admin/system/action` | `{"action":"logs","lines":100}` | helper result |

Actions: `service-status`, `restart-service`, `backup-now`, `backup-list`, `logs`, `doctor`,
`tunnel-status`, `tunnel-config`, `tunnel-install` (needs `token`), `tunnel-restart`,
`tunnel-logout` (needs `confirm: "RESET"`).

`tunnel-config` and `backup-list` take no fields and always answer `ok: true`; they read
`/etc/cloudflared/config.yml` (`HELPER_TUNNEL_CONFIG`) and the backup directory
(`HELPER_BACKUP_DIR`, default `/opt/tempmail/backups`). `tunnel-config` reports
`mode` (`token`/`credentials`/`unknown`), `tunnel_id`, `credentials_file`, `ingress_hosts`,
`token_present`, `config_file` (`present`/`missing`) as `key: value` lines, and never prints the
token itself. `backup-list` reports `count`, `latest`, `latest_bytes`, `latest_age_seconds`,
`total_bytes`, all zero/`-` when the directory is empty or missing.

```bash
BASE=http://127.0.0.1:8081
JWT=$(curl -s -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.invalid","password":"your-password"}' | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')

curl -s "$BASE/api/admin/system/status" -H "Authorization: Bearer $JWT"

curl -s -X POST "$BASE/api/admin/system/action" -H "Authorization: Bearer $JWT" \
  -H 'Content-Type: application/json' -d '{"action":"logs","lines":100}'
```

Status shape:

```json
{"helper":{"installed":true,"dir":"/opt/tempmail/data/helper","pending":0},
 "db":{"path":"/opt/tempmail/data/tempmail.db","bytes":40960,"inboxes":2,"messages":5,"attachments":0,
       "series":[0,0,1,0,2,...],"series_peak":2,"series_window_hours":24,"series_capped":false},
 "app":{"uptime_seconds":320,"rss_bytes":15728640,"go_version":"go1.25.0","site_name":"TempMail","port":"8081"},
 "cloudflare":{"profiles":2,"domains":3,"active_profile":"cf-a","worker_name":"tempmailbyjhopanstore"}}
```

`db.series` is the panel's 24-hour activity chart: one number per hour, oldest first, last bucket is
the hour containing "now" (UTC), and `series_capped` marks a lower bound when the 5000-row read cap
was hit. A failing read degrades to `series: [0,…]` instead of an error, so the status endpoint always
answers.

Action result:

```json
{"id":"a1b2c3d4e5f60718","action":"logs","ok":true,"status":"completed","output":"...","error":"","finished_at":"2026-11-12T10:00:00Z","seconds":0.42}
```

Errors: `400` action/param invalid (no request written), `501` helper not installed on this host,
`504` helper did not answer in time (`HELPER_WAIT_SECONDS`, default 25; the request file stays queued
and the helper processes it, and the timer retries every 30s). `restart-service` and `tunnel-logout`
answer `{"status":"accepted"}` immediately because they cut the app or the tunnel connection.

## Complete Automation Flow

### Step-by-step

```
1. Create inbox          → POST /api/inbox
2. Use email to register → (your code, e.g. register on a website)
3. Wait for email        → GET /api/inbox/{email}/wait
4. Extract OTP/links     → from response: codes[0], links[0]
5. Cleanup              → DELETE /api/inbox/{email}
```

### Bash Example

```bash
API_KEY="your-api-key"
BASE="https://your-domain.com"
AUTH="X-Email-API-Key: $API_KEY"

# 1. Create inbox with custom name
EMAIL=$(curl -s -X POST "$BASE/api/inbox" \
  -H "Content-Type: application/json" \
  -H "$AUTH" \
  -d '{"username":"your-name","domain":"your-domain.com"}' \
  | grep -o '"email":"[^"]*"' | cut -d'"' -f4)

echo "Inbox: $EMAIL"

# 2. Register on target site using $EMAIL
# ... (your registration code here) ...

# 3. Wait for email (blocking, max 60s)
RESPONSE=$(curl -s "$BASE/api/inbox/$EMAIL/wait?timeout=60" -H "$AUTH")

# 4. Extract OTP and link
OTP=$(echo "$RESPONSE" | grep -o '"codes":\["[^"]*"' | cut -d'"' -f4)
LINK=$(echo "$RESPONSE" | grep -o '"links":\["[^"]*"' | cut -d'"' -f4)

echo "OTP:  $OTP"
echo "Link: $LINK"

# 5. Cleanup
curl -s -X DELETE "$BASE/api/inbox/$EMAIL" -H "$AUTH"
```

### Python Example

```python
import requests
import json

BASE = "https://your-domain.com"
HEADERS = {"X-Email-API-Key": "your-api-key"}

def create_inbox(username=None, domain="your-domain.com"):
    """Create a temporary inbox. Returns email address."""
    payload = {"domain": domain}
    if username:
        payload["username"] = username
    r = requests.post(f"{BASE}/api/inbox", headers=HEADERS, json=payload)
    return r.json()["email"]

def wait_for_email(email, timeout=60):
    """Block until email arrives. Returns message dict."""
    r = requests.get(
        f"{BASE}/api/inbox/{email}/wait",
        headers=HEADERS,
        timeout=timeout + 5
    )
    return r.json()

def get_otp(message):
    """Extract first OTP code from message."""
    codes = message.get("codes", [])
    return codes[0] if codes else None

def get_link(message):
    """Extract first verification link from message."""
    links = message.get("links", [])
    return links[0] if links else None

def delete_inbox(email):
    """Delete all messages in inbox."""
    requests.delete(f"{BASE}/api/inbox/{email}", headers=HEADERS)

# === Full automation example ===

# 1. Create inbox
email = create_inbox(username="your-name")
print(f"Inbox ready: {email}")

# 2. Register on target site with this email
# ... your registration code here ...

# 3. Wait for verification email
msg = wait_for_email(email, timeout=60)
print(f"From: {msg['from']}")
print(f"Subject: {msg['subject']}")

# 4. Extract OTP and link
otp = get_otp(msg)
link = get_link(msg)
print(f"OTP: {otp}")
print(f"Link: {link}")

# 5. Use OTP or link to verify
# ... your verification code here ...

# 6. Cleanup
delete_inbox(email)
print("Done")
```

### Node.js Example

```javascript
const BASE = "https://your-domain.com";
const HEADERS = { "X-Email-API-Key": "your-api-key", "Content-Type": "application/json" };

async function createInbox(username, domain = "your-domain.com") {
  const body = username ? { username, domain } : { domain };
  const r = await fetch(`${BASE}/api/inbox`, {
    method: "POST", headers: HEADERS,
    body: JSON.stringify(body)
  });
  return (await r.json()).email;
}

async function waitForEmail(email, timeout = 60) {
  const r = await fetch(`${BASE}/api/inbox/${email}/wait?timeout=${timeout}`, {
    headers: HEADERS
  });
  return await r.json();
}

// Usage
(async () => {
  const email = await createInbox("your-name");
  console.log(`Inbox: ${email}`);

  // Register on target site here...

  const msg = await waitForEmail(email, 60);
  const otp = msg.codes?.[0];
  const link = msg.links?.[0];
  console.log(`OTP: ${otp}, Link: ${link}`);
})();
```

## Error Handling

All errors return JSON with `error` field:

```json
{
  "error": "API key required (X-Email-API-Key header)"
}
```

Common errors:

| Status | Error | Cause |
|---|---|---|
| 401 | `API key required` | Missing `X-Email-API-Key` header |
| 401 | `Invalid or missing API key` | Wrong API key value |
| 400 | `No domain configured` | DOMAINS env not set or empty |
| 400 | `Invalid request` | Malformed JSON body |
| 404 | `page not found` | Wrong endpoint path |
| 404 | (empty) | Message/inbox not found |

## Rate Limits

| Endpoint group | Limit |
|---|---|
| API (GET/POST) | 60 requests/minute |
| DELETE | 20 requests/minute |

## Email Data Extraction

TempMail automatically extracts from email body:

| Type | What it finds | Example |
|---|---|---|
| `codes` | Numeric OTP codes (4-8 digits) | `["192834", "123456"]` |
| `links` | Verification URLs | `["https://verify.cloudflare.com/confirm?token=..."]` |

These are available in:
- `GET /api/inbox/{email}/wait` response
- `GET /api/message/{id}` response
- `POST /api/email/receive` response

## Web UI vs API

| Feature | Web UI | API |
|---|---|---|
| Auth | JWT login (email + password) | `X-Email-API-Key` header |
| Create inbox | Button "Random" or type + Enter | `POST /api/inbox` |
| View messages | Click message in list | `GET /api/inbox/{email}` |
| Real-time | SSE (`/sse/{email}?token=JWT`) | Polling or `/wait` |
| Delete | Button "Delete" | `DELETE /api/message/{id}` |

Web UI is for humans. API is for automation scripts.
