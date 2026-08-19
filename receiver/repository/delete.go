package repository

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
)

// DeleteMessage deletes a message and all its attachments (DB + files).
func DeleteMessage(db *sql.DB, messageID int) error {
	// Get attachments to delete their files
	attachments, err := GetAttachmentsByMessageID(db, messageID)
	if err != nil {
		log.Printf("delete: get attachments for message %d: %v", messageID, err)
	}

	// Delete attachment files from disk
	for _, att := range attachments {
		if att.Path != "" {
			os.Remove(att.Path)
		}
	}

	// Delete attachments from DB (cascade should handle this, but be explicit)
	if err := DeleteAttachment(db, messageID); err != nil {
		log.Printf("delete: attachments for message %d: %v", messageID, err)
	}

	// Delete message
	query := `DELETE FROM messages WHERE id = $1`
	query = placeholder(query)
	_, err = db.ExecContext(context.Background(), query, messageID)
	return err
}

// DeleteInbox deletes an inbox and all its messages + attachments.
func DeleteInbox(db *sql.DB, email string) error {
	// Get inbox ID
	var inboxID int
	query := `SELECT id FROM inboxes WHERE email = $1`
	query = placeholder(query)
	err := db.QueryRowContext(context.Background(), query, email).Scan(&inboxID)
	if err != nil {
		return fmt.Errorf("find inbox: %w", err)
	}

	// Get all message IDs
	msgQuery := `SELECT id FROM messages WHERE inbox_id = $1`
	msgQuery = placeholder(msgQuery)
	rows, err := db.QueryContext(context.Background(), msgQuery, inboxID)
	if err != nil {
		return fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	var messageIDs []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			continue
		}
		messageIDs = append(messageIDs, id)
	}

	// Delete each message (with attachments)
	for _, msgID := range messageIDs {
		if err := DeleteMessage(db, msgID); err != nil {
			log.Printf("delete: message %d: %v", msgID, err)
		}
	}

	// Delete inbox
	delQuery := `DELETE FROM inboxes WHERE id = $1`
	delQuery = placeholder(delQuery)
	_, err = db.ExecContext(context.Background(), delQuery, inboxID)
	return err
}

// ExtendInboxExpiryDB extends inbox expiry — alias for clarity.
func ExtendInboxExpiryDB(db *sql.DB, email string) error {
	return ExtendInboxExpiry(db, email)
}
