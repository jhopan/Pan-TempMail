package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

type Attachment struct {
	ID          int    `json:"id"`
	MessageID   int    `json:"message_id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
	Path        string `json:"path"`
}

// SaveAttachment saves an attachment record to the database.
func SaveAttachment(db *sql.DB, messageID int, filename, contentType, filePath string) error {
	query := `INSERT INTO attachments (message_id, filename, content_type, size, data) VALUES ($1, $2, $3, $4, $5)`
	query = placeholder(query)

	// Read file stats for size
	var size int64
	if stat, err := os.Stat(filePath); err == nil {
		size = stat.Size()
	}

	// Read file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read attachment file: %w", err)
	}

	_, err = db.ExecContext(context.Background(), query, messageID, filename, contentType, size, data)
	return err
}

// GetAttachmentsByMessageID returns all attachments for a message.
func GetAttachmentsByMessageID(db *sql.DB, messageID int) ([]Attachment, error) {
	query := `SELECT id, message_id, filename, content_type, size FROM attachments WHERE message_id = $1 ORDER BY id`
	query = placeholder(query)

	rows, err := db.QueryContext(context.Background(), query, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attachments []Attachment
	for rows.Next() {
		var a Attachment
		if err := rows.Scan(&a.ID, &a.MessageID, &a.Filename, &a.ContentType, &a.Size); err != nil {
			continue
		}
		attachments = append(attachments, a)
	}
	return attachments, nil
}

// GetAttachmentByID returns a single attachment with its data.
func GetAttachmentByID(db *sql.DB, id int) (*Attachment, []byte, error) {
	var a Attachment
	var data []byte
	query := `SELECT id, message_id, filename, content_type, size, data FROM attachments WHERE id = $1`
	query = placeholder(query)

	err := db.QueryRowContext(context.Background(), query, id).Scan(
		&a.ID, &a.MessageID, &a.Filename, &a.ContentType, &a.Size, &data,
	)
	if err != nil {
		return nil, nil, err
	}
	return &a, data, nil
}

// DeleteAttachment deletes an attachment by message ID.
func DeleteAttachment(db *sql.DB, messageID int) error {
	query := `DELETE FROM attachments WHERE message_id = $1`
	query = placeholder(query)
	_, err := db.ExecContext(context.Background(), query, messageID)
	return err
}

// GetAttachmentPath returns the storage path for an attachment.
// Kept for backward compatibility — used by delete.go.
func GetAttachmentPath(filename string) string {
	attachmentsDir := os.Getenv("ATTACHMENTS_DIR")
	if attachmentsDir == "" {
		attachmentsDir = "data/attachments"
	}
	ext := filepath.Ext(filename)
	uniqueName := uuid.New().String() + ext
	return filepath.Join(attachmentsDir, uniqueName)
}
