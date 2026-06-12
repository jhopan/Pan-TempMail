package lmtp

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"

	"receiver/api"
	"receiver/database"
	"receiver/parser"
	"receiver/repository"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

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
	log.Println("FROM:", from)
	return nil
}

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.To = to
	log.Println("TO:", to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	parsed, err := parser.ParseEmail(r)
	if err != nil {
		fmt.Println("PARSE EMAIL ERROR:", err)
		return err
	}

	log.Println("EMAIL RECEIVED")
	fmt.Println("Sender :", s.From)
	fmt.Println("To     :", s.To)
	fmt.Println("Subject:", parsed.Subject)
	fmt.Printf("Attachments: %d\n", len(parsed.Attachments))

	inboxID, err := repository.GetInboxID(database.Conn, s.To)
	if err != nil {
		if err == pgx.ErrNoRows {
			inboxID, err = repository.CreateInbox(database.Conn, s.To)
			if err != nil {
				fmt.Println("CREATE INBOX ERROR:", err)
				return err
			}
		} else {
			fmt.Println("GET INBOX ERROR:", err)
			return err
		}
	} else {
		// Extend expiry for active inbox
		repository.ExtendInboxExpiry(database.Conn, s.To)
	}

	messageID, err := repository.SaveMessage(
		database.Conn,
		inboxID,
		s.From,
		parsed.Subject,
		parsed.TextBody,
		parsed.HtmlBody,
	)
	if err != nil {
		fmt.Println("SAVE MESSAGE ERROR:", err)
		return err
	}
	fmt.Printf("MESSAGE SAVED (ID: %d)\n", messageID)

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
		filePath := filepath.Join("/opt/mail-platform/attachments", uniqueName)
		err = os.WriteFile(filePath, att.Data, 0644)
		if err != nil {
			fmt.Printf("SAVE ATTACHMENT %d ERROR: %v\n", i, err)
			continue
		}
		err = repository.SaveAttachment(database.Conn, messageID, att.FileName, att.ContentType, filePath)
		if err != nil {
			fmt.Printf("SAVE ATTACHMENT %d TO DB ERROR: %v\n", i, err)
			os.Remove(filePath)
			continue
		}
		fmt.Printf("ATTACHMENT %d SAVED: %s (%s)\n", i+1, att.FileName, att.ContentType)
	}

	// Notify SSE clients about new email
	broker := api.GetBroker()
	notification, _ := json.Marshal(map[string]interface{}{
		"id":      messageID,
		"sender":  s.From,
		"subject": parsed.Subject,
	})
	broker.Notify(s.To, "new_email", string(notification))
	log.Printf("SSE notification sent for %s", s.To)

	fmt.Println("EMAIL FULLY PROCESSED")
	return nil
}

func (s *Session) Reset() {}
func (s *Session) Logout() error { return nil }

func Start() {
	server := smtp.NewServer(&Backend{})
	server.LMTP = true
	ln, err := net.Listen("tcp", "127.0.0.1:8024")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("LMTP server listening on tcp://127.0.0.1:8024")
	if err := server.Serve(ln); err != nil {
		log.Fatal(err)
	}
}
