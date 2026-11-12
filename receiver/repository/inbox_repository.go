package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"receiver/database"
)

// GetInboxID returns the ID of an inbox by email address.
func GetInboxID(db *sql.DB, email string) (int, error) {
	var id int
	query := placeholder(`SELECT id FROM inboxes WHERE email = $1`)
	err := db.QueryRowContext(context.Background(), query, email).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("inbox not found")
	}
	return id, err
}

// CreateInbox creates a new temporary inbox.
func CreateInbox(db *sql.DB, email string) (int, error) {
	var id int
	query := placeholder(`INSERT INTO inboxes (email, type, expires_at) VALUES ($1, 'TEMP', datetime('now', '+1 day')) RETURNING id`)
	err := db.QueryRowContext(context.Background(), query, email).Scan(&id)
	return id, err
}

// GetInboxOwnerID returns the account that owns an inbox.
// The value is NULL for inboxes created through the automation API key: those belong to no account.
func GetInboxOwnerID(db *sql.DB, email string) (sql.NullInt64, error) {
	var owner sql.NullInt64
	query := placeholder(`SELECT owner_id FROM inboxes WHERE email = $1`)
	err := db.QueryRowContext(context.Background(), query, email).Scan(&owner)
	if err == sql.ErrNoRows {
		return owner, fmt.Errorf("inbox not found")
	}
	return owner, err
}

// CreateInboxOwned creates a temporary inbox owned by a user account.
func CreateInboxOwned(db *sql.DB, email string, ownerID int) (int, error) {
	var id int
	query := placeholder(`INSERT INTO inboxes (email, type, owner_id, expires_at) VALUES ($1, 'TEMP', $2, datetime('now', '+1 day')) RETURNING id`)
	err := db.QueryRowContext(context.Background(), query, email, ownerID).Scan(&id)
	return id, err
}

// GetMessageInboxEmail returns the address of the inbox that holds a message.
func GetMessageInboxEmail(db *sql.DB, messageID string) (string, error) {
	var email string
	query := placeholder(`SELECT i.email FROM messages m JOIN inboxes i ON m.inbox_id = i.id WHERE m.id = $1`)
	err := db.QueryRowContext(context.Background(), query, messageID).Scan(&email)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("message not found")
	}
	return email, err
}

// ExtendInboxExpiry resets the inbox expiry to 24h from now.
func ExtendInboxExpiry(db *sql.DB, email string) error {
	query := placeholder(`UPDATE inboxes SET expires_at = datetime('now', '+1 day') WHERE email = $1`)
	_, err := db.ExecContext(context.Background(), query, email)
	return err
}

// EnsureDBAlive is a health-check helper.
func EnsureDBAlive() error {
	return database.Conn().Ping()
}

// guard against unused import
var _ = time.Now
