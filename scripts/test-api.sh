#!/bin/bash
# Full end-to-end API test
BASE="http://localhost:8080"

echo "=== 1. LOGIN ==="
LOGIN=$(curl -s -X POST "$BASE/api/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@tempmail.test.local","password":"admin123456"}')
TOKEN=$(echo "$LOGIN" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
echo "Token: ${TOKEN:0:30}..."

echo ""
echo "=== 2. CREATE INBOX ==="
curl -s -X POST "$BASE/api/inbox" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","domain":"tempmail.test.local"}'
echo ""

echo "=== 3. GET INBOX (empty) ==="
curl -s "$BASE/api/inbox/testuser@tempmail.test.local" \
  -H "Authorization: Bearer $TOKEN"
echo ""

echo "=== 4. SEND TEST EMAIL (via SMTP localhost) ==="
python3 -c "
import smtplib
from email.mime.text import MIMEText
msg = MIMEText('Your verification code is 483920. Click https://verify.test/confirm?token=abc123 to confirm your account.')
msg['Subject'] = 'Verify your account'
msg['From'] = 'noreply@test.test'
msg['To'] = 'testuser@tempmail.test.local'
try:
    s = smtplib.SMTP('localhost', 25, timeout=5)
    s.sendmail('noreply@test.test', ['testuser@tempmail.test.local'], msg.as_string())
    s.quit()
    print('EMAIL SENT OK')
except Exception as e:
    print(f'SMTP ERROR: {e}')
"

echo ""
echo "=== 5. GET INBOX (should have 1 message) ==="
curl -s "$BASE/api/inbox/testuser@tempmail.test.local" \
  -H "Authorization: Bearer $TOKEN"
echo ""

echo "=== 6. GET MESSAGE DETAIL (with OTP extraction) ==="
# Get first message ID
MSG_ID=$(curl -s "$BASE/api/inbox/testuser@tempmail.test.local" \
  -H "Authorization: Bearer $TOKEN" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
echo "Message ID: $MSG_ID"
curl -s "$BASE/api/message/$MSG_ID" \
  -H "Authorization: Bearer $TOKEN"
echo ""

echo ""
echo "=== 7. RAM USAGE ==="
ps aux | grep tempmail | grep -v grep | awk '{print "PID:" $2, "RAM:" $6/1024 "MB", "CPU:" $3 "%"}'
