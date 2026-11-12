package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"receiver/database"
	"receiver/events"
	"receiver/parser"
	"receiver/repository"

	"github.com/gin-gonic/gin"
)

// EmailReceiveRequest is the JSON body for the HTTP email receive endpoint.
// Cloudflare Worker sends this when forwarding an email.
type EmailReceiveRequest struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Raw     string `json:"raw"`     // Full RFC822 email (headers + body)
	Subject string `json:"subject"` // Optional, parsed from Raw if empty
}

const defaultMaxRawEmailBytes = 5 << 20

var signingWarningOnce sync.Once

// MaxRawEmailBytes caps one forwarded email. It is exported so the SMTP path
// applies the same limit.
func MaxRawEmailBytes() int64 {
	if value := os.Getenv("MAX_RAW_EMAIL_BYTES"); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultMaxRawEmailBytes
}

func maxRawEmailBytes() int64 { return MaxRawEmailBytes() }

// verifyWorkerSignature checks the HMAC-SHA256 of the raw body that the Worker sends once
// WORKER_SIGNING_SECRET is set. Without the secret the endpoint only needs EMAIL_API_KEY,
// which is the previous behaviour, and it says so once in the log.
func verifyWorkerSignature(c *gin.Context, body []byte) bool {
	secret := os.Getenv("WORKER_SIGNING_SECRET")
	if secret == "" {
		signingWarningOnce.Do(func() {
			log.Print("worker request signing is OFF: set WORKER_SIGNING_SECRET and redeploy the Worker to require X-Email-Signature")
		})
		return true
	}
	provided := strings.TrimPrefix(c.GetHeader("X-Email-Signature"), "sha256=")
	if provided == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(provided))
}

// ReceiveEmailViaHTTP handles POST /api/email/receive
// This is the Cloudflare Worker endpoint — accepts email via HTTP instead of SMTP.
// Auth: X-Email-API-Key header (for Worker) or JWT Bearer token.
func ReceiveEmailViaHTTP(c *gin.Context) {
	// Auth: check X-Email-API-Key first (service-to-service)
	apiKey := c.GetHeader("X-Email-API-Key")
	expectedKey := os.Getenv("EMAIL_API_KEY")

	authenticated := false
	if expectedKey != "" && apiKey == expectedKey {
		authenticated = true
	}

	if !authenticated {
		// Fall back to JWT Bearer token
		tokenStr := c.GetHeader("Authorization")
		if strings.HasPrefix(tokenStr, "Bearer ") {
			// If JWT middleware is on this route, it would've already validated.
			// For this public route, we accept Bearer tokens if EMAIL_API_KEY is not set.
			if expectedKey == "" {
				authenticated = true
			}
		}
	}

	if !authenticated && expectedKey != "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or missing API key"})
		return
	}

	limit := maxRawEmailBytes()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit+4096)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Email terlalu besar", "limit_bytes": limit})
		return
	}
	if !verifyWorkerSignature(c, body) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Signature Worker tidak valid"})
		return
	}

	var req EmailReceiveRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.Raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "raw email content required"})
		return
	}

	if int64(len(req.Raw)) > limit {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Email terlalu besar", "limit_bytes": limit})
		return
	}

	if req.To == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to address required"})
		return
	}

	// Parse the raw RFC822 email
	parsed, err := parser.ParseEmail(bytes.NewReader([]byte(req.Raw)))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse email: " + err.Error()})
		return
	}

	// Use From from request if parsing didn't find it
	sender := req.From
	if sender == "" {
		sender = "unknown@unknown"
	}

	log.Printf("HTTP EMAIL RECEIVED: from=%s to=%s subject=%s", sender, req.To, parsed.Subject)

	// Get or create inbox
	inboxID, err := repository.GetInboxID(database.Conn(), req.To)
	if err != nil {
		inboxID, err = repository.CreateInbox(database.Conn(), req.To)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create inbox"})
			return
		}
	} else {
		_ = repository.ExtendInboxExpiry(database.Conn(), req.To)
	}

	// Extract OTP codes and verification links
	fullBody := parsed.TextBody + " " + parsed.HtmlBody
	codes := extractCodesFromText(fullBody)
	links := extractLinksFromText(fullBody)

	// Save message
	messageID, err := repository.SaveMessage(
		database.Conn(),
		inboxID,
		sender,
		parsed.Subject,
		parsed.TextBody,
		parsed.HtmlBody,
		codes,
		links,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save message"})
		return
	}

	// Save attachments
	for i, att := range parsed.Attachments {
		attachmentsDir := os.Getenv("ATTACHMENTS_DIR")
		if attachmentsDir == "" {
			attachmentsDir = "data/attachments"
		}
		_ = os.MkdirAll(attachmentsDir, 0755)
		uniqueName := fmt.Sprintf("%d_%d_%s", time.Now().UnixNano(), i, att.FileName)
		filePath := attachmentsDir + "/" + uniqueName
		if err := os.WriteFile(filePath, att.Data, 0644); err != nil {
			continue
		}
		_ = repository.SaveAttachment(database.Conn(), messageID, att.FileName, att.ContentType, filePath)
	}

	// Notify SSE clients
	broker := events.Get()
	notification, _ := json.Marshal(map[string]interface{}{
		"id":      messageID,
		"sender":  sender,
		"subject": parsed.Subject,
	})
	broker.Notify(req.To, "new_email", string(notification))

	// Fire webhook if configured
	if webhookURL := os.Getenv("WEBHOOK_URL"); webhookURL != "" {
		go fireHTTPWebhook(webhookURL, req.To, sender, parsed.Subject, messageID, codes, links)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "received",
		"message_id":  messageID,
		"to":          req.To,
		"from":        sender,
		"subject":     parsed.Subject,
		"codes":       codes,
		"links":       links,
		"attachments": len(parsed.Attachments),
	})
}

// fireHTTPWebhook sends a webhook notification for HTTP-received emails.
func fireHTTPWebhook(url, recipient, sender, subject string, messageID int, codes, links []string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"event":      "email.received",
		"message_id": messageID,
		"to":         recipient,
		"from":       sender,
		"subject":    subject,
		"codes":      codes,
		"links":      links,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"source":     "cloudflare_worker",
	})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}

// extractCodesFromText finds OTP/verification codes in text.
func extractCodesFromText(text string) []string {
	// Reuse the regex from smtp package via simple pattern
	// 4-8 digit codes
	return extractPattern(text, `\b\d{4,8}\b`)
}

// extractLinksFromText finds verification URLs in text.
func extractLinksFromText(text string) []string {
	urls := extractPattern(text, `https?://[^\s<>"']+`)
	var links []string
	seen := make(map[string]bool)
	for _, u := range urls {
		u = strings.TrimRight(u, ".,;)")
		if seen[u] {
			continue
		}
		seen[u] = true
		links = append(links, u)
	}
	return links
}

// extractPattern extracts all matches of a regex pattern from text.
func extractPattern(text, pattern string) []string {
	// Use simple regex matching
	import_re := regexpCompile(pattern)
	if import_re == nil {
		return nil
	}
	matches := import_re.FindAllString(text, -1)
	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}
	return result
}
