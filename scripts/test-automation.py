import json, urllib.request, urllib.error

B = "http://127.0.0.1:8080"
API_KEY = "cea0864e46a56db016f028be587359020a7ee27754f6d961"

def req(method, path, body=None, token=""):
    data = json.dumps(body).encode() if body else None
    r = urllib.request.Request(B + path, data=data, method=method)
    r.add_header("Content-Type", "application/json")
    if token:
        r.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(r, timeout=10) as resp:
            return resp.status, json.loads(resp.read())
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read())

# login
s, login = req("POST", "/api/auth/login", {"email": "admin@tempmail.local", "password": "admin12345"})
print("login:", s)
token = login["token"]

# create inbox
s, inbox = req("POST", "/api/inbox", {"username": "otptest", "domain": "tempmail.local"}, token)
print("inbox:", s, inbox)

# simulate worker: POST /api/email/receive with API key
raw = """From: service@example.com
To: otptest@tempmail.local
Subject: Your verification code
Content-Type: text/plain

Your OTP code is 482913.
Verify here: https://example.com/verify?code=482913
"""
data = json.dumps({"from": "service@example.com", "to": "otptest@tempmail.local", "subject": "Your verification code", "raw": raw}).encode()
r = urllib.request.Request(B + "/api/email/receive", data=data, method="POST")
r.add_header("Content-Type", "application/json")
r.add_header("X-Email-API-Key", API_KEY)
with urllib.request.urlopen(r, timeout=10) as resp:
    print("receive:", resp.status, json.loads(resp.read()))

# wait endpoint (should return immediately since message exists)
s, msg = req("GET", "/api/inbox/otptest@tempmail.local/wait?timeout=5", token=token)
print("wait:", s)
print("codes:", msg.get("codes"))
print("links:", msg.get("links"))
print("subject:", msg.get("subject"))
