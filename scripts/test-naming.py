import json, urllib.request, urllib.error

B = "http://127.0.0.1:8080"

def req(method, path, body=None, token=""):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(B + path, data=data, method=method)
    r.add_header("Content-Type", "application/json")
    if token:
        r.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(r, timeout=10) as resp:
            return resp.status, json.loads(resp.read())
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read())

s, login = req("POST", "/api/auth/login", {"email": "admin@tempmail.local", "password": "admin12345"})
token = login["token"]
print("login:", s)

# 1. default rules
s, rules = req("GET", "/api/admin/naming-rules", token=token)
print("default rules:", s, "| template:", rules.get("template"), "| words:", len(rules.get("first_words", [])), "x", len(rules.get("second_words", [])))

# 2. bot flow: empty body -> generated address
for i in range(3):
    s, inbox = req("POST", "/api/inbox", {}, token)
    print("auto inbox:", s, inbox.get("email"))

# 3. change rules: prefix + dot separator + always 4-digit number
new_rules = {
    "template": "bot.{first}.{second}{num}",
    "separator": "",
    "first_words": ["alpha", "bravo", "charlie", "delta"],
    "second_words": ["one", "two", "three"],
    "num_min": 1000,
    "num_max": 9999,
    "num_probability": 100,
}
s, res = req("PUT", "/api/admin/naming-rules", new_rules, token)
print("update rules:", s, res.get("message"))

# 4. bot flow again with new rules
for i in range(3):
    s, inbox = req("POST", "/api/inbox", {}, token)
    print("new-rule inbox:", s, inbox.get("email"))

# 5. invalid rules rejected
s, res = req("PUT", "/api/admin/naming-rules", {"template": "{first}", "first_words": [], "second_words": []}, token)
print("invalid rules:", s, res.get("error"))
