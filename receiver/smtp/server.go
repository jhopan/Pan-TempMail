package smtp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"receiver/api"
	"receiver/database"
	"receiver/parser"
	"receiver/repository"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
)

// codePattern matches 4-8 digit OTP/verification codes
var codePattern = regexp.MustCompile(`\b(\d{4,8})\b`)

// linkPattern matches verification URLs
var linkPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

type Backend struct{}

func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	return &Session{}, nil
}

type Session struct {
	From string
	To   string
}

func (s *Session) AuthPlain(username, password string) error {
	return nil
}

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.From = from
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.To = to
	return nil
}

func (s *Session) Data(r io.Reader) error {
	parsed, err := parser.ParseEmail(r)
	if err != nil {
		log.Printf("smtp: parse email from %s: %v", s.From, err)
		return err
	}

	log.Printf("EMAIL RECEIVED: from=%s to=%s subject=%s attachments=%d",
		s.From, s.To, parsed.Subject, len(parsed.Attachments))

	// Get or create inbox
	inboxID, err := repository.GetInboxID(database.Conn(), s.To)
	if err != nil {
		inboxID, err = repository.CreateInbox(database.Conn(), s.To)
		if err != nil {
			log.Printf("smtp: create inbox %s: %v", s.To, err)
			return err
		}
	} else {
		_ = repository.ExtendInboxExpiry(database.Conn(), s.To)
	}

	// Extract OTP codes and verification links from email body
	codes := extractCodes(parsed.TextBody + " " + parsed.HtmlBody)
	links := extractLinks(parsed.TextBody + " " + parsed.HtmlBody)

	// Save message
	messageID, err := repository.SaveMessage(
		database.Conn(),
		inboxID,
		s.From,
		parsed.Subject,
		parsed.TextBody,
		parsed.HtmlBody,
		codes,
		links,
	)
	if err != nil {
		log.Printf("smtp: save message: %v", err)
		return err
	}
	log.Printf("MESSAGE SAVED (ID: %d) for %s", messageID, s.To)

	// Save attachments
	for i, att := range parsed.Attachments {
		ext := filepath.Ext(att.FileName)
		if ext == "" && att.ContentType != "" {
			switch att.ContentType {
			case "image/jpeg", "image/jpg":
				ext = ".jpg"
			case "image/png":
				ext = ".png"
			case "image/gif":
				ext = ".gif"
			case "application/pdf":
				ext = ".pdf"
			}
		}
		uniqueName := fmt.Sprintf("%s%s", uuid.New().String(), ext)
		attachmentsDir := os.Getenv("ATTACHMENTS_DIR")
		if attachmentsDir == "" {
			attachmentsDir = "data/attachments"
		}
		if err := os.MkdirAll(attachmentsDir, 0755); err != nil {
			log.Printf("smtp: create attachments dir: %v", err)
			continue
		}
		filePath := filepath.Join(attachmentsDir, uniqueName)
		if err := os.WriteFile(filePath, att.Data, 0644); err != nil {
			log.Printf("smtp: save attachment %d: %v", i+1, err)
			continue
		}
		if err := repository.SaveAttachment(database.Conn(), messageID, att.FileName, att.ContentType, filePath); err != nil {
			log.Printf("smtp: save attachment %d to db: %v", i+1, err)
			os.Remove(filePath)
			continue
		}
		log.Printf("ATTACHMENT %d SAVED: %s (%s)", i+1, att.FileName, att.ContentType)
	}

	// Notify SSE clients about new email
	broker := api.GetBroker()
	notification, _ := json.Marshal(map[string]interface{}{
		"id":      messageID,
		"sender":  s.From,
		"subject": parsed.Subject,
	})
	broker.Notify(s.To, "new_email", string(notification))

	// Fire webhook if configured
	if webhookURL := os.Getenv("WEBHOOK_URL"); webhookURL != "" {
		go fireWebhook(webhookURL, s.To, s.From, parsed.Subject, messageID, codes, links)
	}

	log.Printf("EMAIL FULLY PROCESSED: id=%d to=%s", messageID, s.To)
	return nil
}

func (s *Session) Reset() {}
func (s *Session) Logout() error { return nil }

// extractCodes finds OTP/verification codes in text.
func extractCodes(text string) []string {
	matches := codePattern.FindAllString(text, -1)
	// Deduplicate
	seen := make(map[string]bool)
	var codes []string
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			codes = append(codes, m)
		}
	}
	return codes
}

// extractLinks finds verification URLs in text.
func extractLinks(text string) []string {
	matches := linkPattern.FindAllString(text, -1)
	// Filter: only keep links that look like verification/confirm
	var links []string
	seen := make(map[string]bool)
	for _, m := range matches {
		m = strings.TrimRight(m, ".,;)")
		if seen[m] {
			continue
		}
		seen[m] = true
		lower := strings.ToLower(m)
		if strings.Contains(lower, "verify") ||
			strings.Contains(lower, "confirm") ||
			strings.Contains(lower, "activate") ||
			strings.Contains(lower, "reset") ||
			strings.Contains(lower, "token") ||
			strings.Contains(lower, "click") {
			links = append(links, m)
		}
	}
	// If no verification links found, keep all links (up to 5)
	if len(links) == 0 {
		for i, m := range matches {
			if i >= 5 {
				break
			}
			m = strings.TrimRight(m, ".,;)")
			if !seen[m] {
				links = append(links, m)
				seen[m] = true
			}
		}
	}
	return links
}

// fireWebhook sends a POST callback to an external URL.
func fireWebhook(url, recipient, sender, subject string, messageID int, codes, links []string) {
	// Simple webhook implementation — can be enhanced with HMAC signing
	payload, _ := json.Marshal(map[string]interface{}{
		"event":      "email.received",
		"message_id": messageID,
		"to":         recipient,
		"from":       sender,
		"subject":    subject,
		"codes":      codes,
		"links":      links,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("webhook: post to %s: %v", url, err)
		return
	}
	defer resp.Body.Close()
	log.Printf("webhook: delivered to %s (status: %d)", url, resp.StatusCode)
}

// Start launches the built-in SMTP server on port 25.
// This replaces Maddy entirely — one less process to run.
func Start() {
	port := os.Getenv("SMTP_PORT")
	if port == "" {
		port = "25"
	}

	host := os.Getenv("SMTP_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	server := smtp.NewServer(&Backend{})
	server.Addr = fmt.Sprintf("%s:%s", host, port)
	server.MaxMessageBytes = 10 * 1024 * 1024 // 10MB
	server.MaxRecipients = 5

	log.Printf("SMTP server listening on %s:%s (built-in, no Maddy needed)", host, port)

	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("smtp: server error: %v", err)
		}
	}()
}
