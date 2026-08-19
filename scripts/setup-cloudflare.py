#!/usr/bin/env python3
"""
TempMail Cloudflare Auto-Setup
Automatically configures:
  1. Cloudflare DNS records (MX + A)
  2. Email Routing catch-all → Worker
  3. Deploys the Email Worker

Requirements:
  pip install httpx
  npx wrangler (for Worker deploy)

Usage:
  python3 setup-cloudflare.py --domain mail.example.com \
    --cf-token YOUR_CF_API_TOKEN \
    --cf-account YOUR_CF_ACCOUNT_ID \
    --server-url https://your-vps.com \
    --server-ip 1.2.3.4 \
    --email-api-key YOUR_EMAIL_API_KEY
"""

import argparse
import json
import subprocess
import sys
import time
from pathlib import Path

try:
    import httpx
except ImportError:
    print("Installing httpx...")
    subprocess.check_call([sys.executable, "-m", "pip", "install", "httpx"])
    import httpx

CF_API = "https://api.cloudflare.com/client/v4"


def cf_request(client, method, path, **kwargs):
    """Make a Cloudflare API request with error handling."""
    resp = client.request(method, f"{CF_API}{path}", **kwargs)
    data = resp.json()
    if not data.get("success", False):
        errors = data.get("errors", [{"message": "Unknown error"}])
        msg = errors[0].get("message", "Unknown error") if errors else "Unknown error"
        # Not an error if zone already exists
        if "already exists" in msg.lower():
            print(f"  [skip] {msg}")
            return None
        raise Exception(f"Cloudflare API error: {msg}")
    return data.get("result")


def get_zone_id(client, token, domain):
    """Get zone ID for a domain (or create one if it doesn't exist)."""
    headers = {"Authorization": f"Bearer {token}"}
    # Search by domain
    result = cf_request(client, "GET", f"/zones?name={domain}", headers=headers)
    if result:
        print(f"  [ok] Zone found: {domain} (ID: {result[0]['id']})")
        return result[0]["id"]

    # Create zone
    print(f"  [+] Creating zone for {domain}...")
    result = cf_request(client, "POST", "/zones",
                        headers=headers,
                        json={"name": domain, "account": {"id": ""}, "type": "full"})
    if result:
        print(f"  [ok] Zone created: {domain} (ID: {result['id']})")
        return result["id"]
    return None


def setup_dns(client, token, zone_id, domain, server_ip):
    """Setup A and MX records for the domain."""
    headers = {"Authorization": f"Bearer {token}"}

    # A record: mail.domain.com → server_ip
    a_name = f"mail.{domain}"
    print(f"  [+] Creating A record: {a_name} → {server_ip}")
    try:
        cf_request(client, "POST", f"/zones/{zone_id}/dns_records",
                   headers=headers,
                   json={"type": "A", "name": a_name, "content": server_ip, "ttl": 1, "proxied": False})
        print(f"  [ok] A record created")
    except Exception as e:
        if "already exists" in str(e).lower():
            print(f"  [skip] A record already exists")
        else:
            raise

    # MX record: domain → mail.domain.com
    print(f"  [+] Creating MX record: {domain} → mail.{domain}")
    try:
        cf_request(client, "POST", f"/zones/{zone_id}/dns_records",
                   headers=headers,
                   json={"type": "MX", "name": domain, "content": f"mail.{domain}", "priority": 10, "ttl": 1})
        print(f"  [ok] MX record created")
    except Exception as e:
        if "already exists" in str(e).lower():
            print(f"  [skip] MX record already exists")
        else:
            raise

    # SPF record
    print(f"  [+] Creating SPF (TXT) record")
    try:
        cf_request(client, "POST", f"/zones/{zone_id}/dns_records",
                   headers=headers,
                   json={"type": "TXT", "name": domain, "content": "v=spf1 include:_spf.mx.cloudflare.net ~all", "ttl": 1})
        print(f"  [ok] SPF record created")
    except Exception as e:
        if "already exists" in str(e).lower():
            print(f"  [skip] SPF record already exists")


def enable_email_routing(client, token, zone_id):
    """Enable Cloudflare Email Routing for the zone."""
    headers = {"Authorization": f"Bearer {token}"}
    print(f"  [+] Enabling Email Routing...")
    try:
        cf_request(client, "POST", f"/zones/{zone_id}/email/routing/enable",
                   headers=headers, json={})
        print(f"  [ok] Email Routing enabled")
    except Exception:
        print(f"  [skip] Email Routing may already be enabled")


def setup_catch_all(client, token, zone_id, worker_name):
    """Set up catch-all email routing to forward to the Worker."""
    headers = {"Authorization": f"Bearer {token}"}
    print(f"  [+] Setting up catch-all routing to Worker: {worker_name}")
    try:
        cf_request(client, "PUT", f"/zones/{zone_id}/email/routing/rules/catch_all",
                   headers=headers,
                   json={
                       "name": "TempMail catch-all",
                       "enabled": True,
                       "actions": [
                           {
                               "type": "worker",
                               "value": [worker_name]
                           }
                       ],
                       "matchers": [{"type": "all"}]
                   })
        print(f"  [ok] Catch-all routing configured")
    except Exception as e:
        print(f"  [warn] Catch-all setup: {e}")


def deploy_worker(worker_dir, server_url, email_api_key):
    """Deploy the Cloudflare Worker using wrangler."""
    print(f"\n🚀 Deploying Cloudflare Worker...")

    # Update wrangler.toml with values
    wrangler_path = Path(worker_dir) / "wrangler.toml"
    content = wrangler_path.read_text()
    content = content.replace("https://your-vps.com", server_url)
    content = content.replace('TEMPMAIL_API_KEY = ""', f'TEMPMAIL_API_KEY = "{email_api_key}"')
    wrangler_path.write_text(content)

    # Deploy via wrangler
    result = subprocess.run(
        ["npx", "wrangler", "deploy"],
        cwd=worker_dir,
        capture_output=True,
        text=True,
        shell=True
    )

    if result.returncode != 0:
        print(f"  [error] Wrangler deploy failed:\n{result.stderr}")
        print(f"  [hint] Make sure you're logged in: npx wrangler login")
        return False

    print(f"  [ok] Worker deployed!")
    # Extract worker URL from output
    for line in result.stdout.split("\n"):
        if "workers.dev" in line:
            print(f"  [info] {line.strip()}")
    return True


def main():
    parser = argparse.ArgumentParser(description="TempMail Cloudflare Auto-Setup")
    parser.add_argument("--domain", required=True, help="Domain for temp mail (e.g., mail.example.com)")
    parser.add_argument("--cf-token", required=True, help="Cloudflare API Token")
    parser.add_argument("--cf-account", required=True, help="Cloudflare Account ID")
    parser.add_argument("--server-url", required=True, help="VPS URL (e.g., https://your-vps.com)")
    parser.add_argument("--server-ip", required=True, help="VPS IP address")
    parser.add_argument("--email-api-key", required=True, help="EMAIL_API_KEY for VPS endpoint auth")
    parser.add_argument("--worker-dir", default="./cloudflare-worker", help="Worker source directory")

    args = parser.parse_args()

    worker_name = "tempmail-email-worker"
    domain = args.domain

    print(f"\n{'='*60}")
    print(f"  TempMail Cloudflare Auto-Setup")
    print(f"{'='*60}")
    print(f"  Domain:      {domain}")
    print(f"  Server URL:  {args.server_url}")
    print(f"  Server IP:   {args.server_ip}")
    print(f"  Worker:      {worker_name}")
    print(f"{'='*60}\n")

    client = httpx.Client(timeout=30)

    # 1. Get or create zone
    print("📍 Step 1: Zone setup")
    zone_id = get_zone_id(client, args.cf_token, domain)
    if not zone_id:
        print("  [error] Failed to get/create zone. Is the domain on this Cloudflare account?")
        sys.exit(1)

    # 2. Setup DNS records
    print("\n📍 Step 2: DNS records")
    setup_dns(client, args.cf_token, zone_id, domain, args.server_ip)

    # 3. Enable Email Routing
    print("\n📍 Step 3: Email Routing")
    enable_email_routing(client, args.cf_token, zone_id)

    # 4. Deploy Worker
    print("\n📍 Step 4: Worker deployment")
    deploy_worker(args.worker_dir, args.server_url, args.email_api_key)

    # 5. Setup catch-all routing to Worker
    print("\n📍 Step 5: Catch-all routing")
    setup_catch_all(client, args.cf_token, zone_id, worker_name)

    print(f"\n{'='*60}")
    print(f"  ✅ Setup Complete!")
    print(f"{'='*60}")
    print(f"\n  Your temp mail domain: {domain}")
    print(f"  Any email sent to *@{domain} will be forwarded to:")
    print(f"    {args.server_url}/api/email/receive")
    print(f"\n  Test it:")
    print(f"    1. Create inbox: POST {args.server_url}/api/inbox")
    print(f"    2. Send email to: testuser@{domain}")
    print(f"    3. Wait for message: GET {args.server_url}/api/inbox/testuser@{domain}/wait")
    print(f"\n  Next steps:")
    print(f"    - Set EMAIL_API_KEY={args.email_api_key} in VPS .env")
    print(f"    - Restart the tempmail service on VPS")
    print(f"    - Add {domain} to DOMAINS env var")
    print(f"{'='*60}\n")


if __name__ == "__main__":
    main()
