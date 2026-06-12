package repository

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5"
)

// DeleteMessage deletes a message and all its attachments
func DeleteMessage(conn *pgx.Conn, messageID int) error {
	// Get attachments to delete files
	attachments, err := GetAttachmentsByMessageID(conn, messageID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}

	// Delete attachment files from disk
	for _, att := range attachments {
		os.Remove(att.Path)
	}

	// Delete attachments from DB
	_, err = conn.Exec(context.Background(),
		"DELETE FROM attachments WHERE message_id = $1", messageID)
	if err != nil {
		return err
	}

	// Delete message
	_, err = conn.Exec(context.Background(),
		"DELETE FROM messages WHERE id = $1", messageID)
	return err
}

// DeleteInbox deletes an inbox and all its messages + attachments
func DeleteInbox(conn *pgx.Conn, email string) error {
	// Get inbox ID
	var inboxID int
	err := conn.QueryRow(context.Background(),
		"SELECT id FROM inboxes WHERE email = $1", email).Scan(&inboxID)
	if err != nil {
		return err
	}

	// Get all message IDs
	rows, err := conn.Query(context.Background(),
		"SELECT id FROM messages WHERE inbox_id = $1", inboxID)
	if err != nil {
		return err
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
		if err := DeleteMessage(conn, msgID); err != nil {
			return err
		}
	}

	// Delete inbox
	_, err = conn.Exec(context.Background(),
		"DELETE FROM inboxes WHERE id = $1", inboxID)
	return err
}

// ExtendInboxExpiry extends inbox expiry by 24 hours from now
func ExtendInboxExpiry(conn *pgx.Conn, email string) error {
	_, err := conn.Exec(context.Background(),
		"UPDATE inboxes SET expires_at = NOW() + INTERVAL '1 day' WHERE email = $1", email)
	return err
}
