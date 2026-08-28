#!/bin/bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

SERVER_IP="__SERVER_IP__"
MAIL_HOST="__MAIL_HOST__"
MADDY_CONF="/etc/maddy/maddy.conf"
PAGE_TSX="/opt/mail-platform/frontend-backup-zh/src/app/page.tsx"
HELPER="/opt/mail-platform/maddy-add-domain.py"

DOMAIN="${1:-}"
CF_API_TOKEN=***
C...
if [ -z "$DOMAIN" ]; then read -p "Domain to add (e.g. mymail.com): " DOMAIN; fi
if [ -z "$CF_API_TOKEN" ]; then read -sp "Cloudflare API Token: " CF_API_TOKEN; echo ""; fi
if [ -z "$CF_ACCOUNT_ID" ]; then read -sp "Cloudflare Account ID: " CF_ACCOUNT_ID; echo ""; fi

AUTH_HDR="Authorization: Bearer $CF_API_TOKEN"

echo ""
echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}  Temp Mail - Auto Domain Setup${NC}"
echo -e "${CYAN}  Cloudflare API Integration${NC}"
echo -e "${CYAN}========================================${NC}"
echo ""
echo -e "Domain:  ${CYAN}${DOMAIN}${NC}"
echo -e "Server:  ${CYAN}${SERVER_IP}${NC}"
echo ""

echo -e "${YELLOW}[1/5]${NC} Checking Cloudflare zone..."

ZONE_RESP=$(curl -s -X GET \
    "https://api.cloudflare.com/client/v4/zones?name=${DOMAIN}&account.id=${CF_ACCOUNT_ID}" \
    -H "$AUTH_HDR" \
    -H "Content-Type: application/json")

ZONE_ID=$(echo "$ZONE_RESP" | python3 -c "
import sys,json
try:
    d=json.load(sys.stdin)
    r=d.get('result',[])
    print(r[0]['id'] if r else '')
except: print('')
" 2>/dev/null)

NS1=""
NS2=""

if [ -z "$ZONE_ID" ]; then
    echo -e "${YELLOW}[1/5]${NC} Zone not found. Creating zone..."

    CREATE_BODY="{\"name\":\"${DOMAIN}\",\"account\":{\"id\":\"${CF_ACCOUNT_ID}\"},\"type\":\"full\"}"
    CREATE_RESP=$(curl -s -X POST "https://api.cloudflare.com/client/v4/zones" \
        -H "$AUTH_HDR" \
        -H "Content-Type: application/json" \
        --data "$CREATE_BODY")

    ZONE_ID=$(echo "$CREATE_RESP" | python3 -c "
import sys,json
try:
    d=json.load(sys.stdin)
    print(d.get('result',{}).get('id',''))
except: print('')
" 2>/dev/null)

    if [ -z "$ZONE_ID" ]; then
        ERROR=$(echo "$CREATE_RESP" | python3 -c "
import sys,json
try:
    d=json.load(sys.stdin)
    errs=d.get('errors',[])
    print(errs[0]['message'] if errs else 'Unknown error')
except: print('Parse error')
" 2>/dev/null)
        echo -e "${RED}[FAIL]${NC} Could not create zone: $ERROR"
        exit 1
    fi

    NS1=$(echo "$CREATE_RESP" | python3 -c "
import sys,json
try:
    d=json.load(sys.stdin)
    ns=d.get('result',{}).get('name_servers',[])
    print(ns[0] if len(ns)>0 else '')
except: print('')
" 2>/dev/null)

    NS2=$(echo "$CREATE_RESP" | python3 -c "
import sys,json
try:
    d=json.load(sys.stdin)
    ns=d.get('result',{}).get('name_servers',[])
    print(ns[1] if len(ns)>1 else '')
except: print('')
" 2>/dev/null)

    echo -e "${GREEN}[1/5]${NC} Zone created! ID: $ZONE_ID"
    echo -e "  Nameservers: $NS1, $NS2"
else
    echo -e "${GREEN}[1/5]${NC} Zone found! ID: $ZONE_ID"
fi

echo -e "${YELLOW}[2/5]${NC} Creating DNS records..."

mkrecord() {
    local rtype="$1" rname="$2" rcontent="$3" rpri="$4" rprox="$5"

    local json="{\"type\":\"${rtype}\",\"name\":\"${rname}\",\"content\":\"${rcontent}\""
    [ -n "$rpri" ] && json="${json},\"priority\":${rpri}"
    [ -n "$rprox" ] && json="${json},\"proxied\":${rprox}"
    json="${json}}"

    local resp
    resp=$(curl -s -X POST \
        "https://api.cloudflare.com/client/v4/zones/${ZONE_ID}/dns_records" \
        -H "$AUTH_HDR" \
        -H "Content-Type: application/json" \
        --data "$json")

    local ok
    ok=$(echo "$resp" | python3 -c "
import sys,json
try:
    print(json.load(sys.stdin).get('success',''))
except: print('')
" 2>/dev/null)

    if [ "$ok" = "True" ]; then
        echo -e "  ${GREEN}OK${NC} ${rtype} ${rname} -> ${rcontent}"
    else
        echo -e "  ${YELLOW}SKIP${NC} ${rtype} ${rname} [may already exist]"
    fi
}

mkrecord "MX" "$DOMAIN" "$MAIL_HOST" "10" ""
mkrecord "A" "mail.$DOMAIN" "$SERVER_IP" "" "false"
mkrecord "A" "$DOMAIN" "$SERVER_IP" "" "false"
mkrecord "TXT" "$DOMAIN" "v=spf1 ip4:${SERVER_IP} ~all" "" ""
mkrecord "TXT" "_dmarc.$DOMAIN" "v=DMARC1; p=none; rua=mailto:admin@__PRIMARY_DOMAIN__" "" ""

echo -e "${GREEN}[2/5]${NC} DNS records done!"

echo -e "${YELLOW}[3/5]${NC} Updating Maddy..."
python3 "$HELPER" "$DOMAIN"
systemctl restart maddy
echo -e "  ${GREEN}OK${NC} Maddy restarted"
echo -e "${GREEN}[3/5]${NC} Maddy updated!"

echo -e "${YELLOW}[4/5]${NC} Updating frontend..."
if grep -q "\"$DOMAIN\"" "$PAGE_TSX"; then
    echo -e "  ${YELLOW}SKIP${NC} Already in frontend"
else
    sed -i "s|// Tambah domain|\"$DOMAIN\",\n  // Tambah domain|" "$PAGE_TSX"
    echo -e "  ${GREEN}OK${NC} Added to DOMAINS array"
fi

echo "  Building Next.js..."
cd /opt/mail-platform/frontend-backup-zh && npx next build > /tmp/next-build.log 2>&1
if [ $? -eq 0 ]; then
    echo -e "  ${GREEN}OK${NC} Frontend built"
else
    echo -e "  ${RED}FAIL${NC} Build failed! Log: /tmp/next-build.log"
    exit 1
fi

systemctl restart tempmail-frontend
echo -e "  ${GREEN}OK${NC} Frontend restarted"
echo -e "${GREEN}[4/5]${NC} Frontend updated!"

echo -e "${YELLOW}[5/5]${NC} Verifying..."
sleep 3

systemctl is-active --quiet maddy && echo -e "  ${GREEN}OK${NC} Maddy running" || echo -e "  ${RED}FAIL${NC} Maddy down"
HTTP=$(curl -s -o /dev/null -w "%{http_code}" -H "Host: ${SERVER_IP}" http://127.0.0.1/)
[ "$HTTP" = "200" ] && echo -e "  ${GREEN}OK${NC} Frontend HTTP $HTTP" || echo -e "  ${RED}FAIL${NC} Frontend HTTP $HTTP"

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Domain Setup Complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "  Domain:  ${CYAN}${DOMAIN}${NC}"
echo -e "  Zone ID: ${CYAN}${ZONE_ID}${NC}"
echo -e "  Email:   ${CYAN}anything@${DOMAIN}${NC}"
echo -e "${GREEN}========================================${NC}"

if [ -n "$NS1" ]; then
    echo ""
    echo -e "${YELLOW}ACTION REQUIRED - Update nameservers at registrar:${NC}"
    echo -e "  ${GREEN}${NS1}${NC}"
    echo -e "  ${GREEN}${NS2}${NC}"
fi

MX=$(dig +short MX "$DOMAIN" 2>/dev/null | head -1)
echo ""
if echo "$MX" | grep -q "mail.your-domain"; then
    echo -e "${GREEN}MX record active:${NC} $MX"
else
    echo -e "${YELLOW}MX record pending propagation${NC} [up to 24h]"
    echo -e "  Expected: 10 $MAIL_HOST"
fi
echo ""
