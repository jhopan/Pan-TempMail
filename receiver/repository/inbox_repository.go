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
	var query string
	if dbDriverName() == "postgres" {
		query = `INSERT INTO inboxes (email, type, expires_at) VALUES ($1, 'TEMP', NOW() + INTERVAL '1 day') RETURNING id`
	} else {
		query = `INSERT INTO inboxes (email, type, expires_at) VALUES (?, 'TEMP', datetime('now', '+1 day')) RETURNING id`
	}
	err := db.QueryRowContext(context.Background(), query, email).Scan(&id)
	return id, err
}

// ExtendInboxExpiry resets the inbox expiry to 24h from now.
func ExtendInboxExpiry(db *sql.DB, email string) error {
	var query string
	if dbDriverName() == "postgres" {
		query = `UPDATE inboxes SET expires_at = NOW() + INTERVAL '1 day' WHERE email = $1`
	} else {
		query = `UPDATE inboxes SET expires_at = datetime('now', '+1 day') WHERE email = ?`
	}
	_, err := db.ExecContext(context.Background(), query, email)
	return err
}

// EnsureDBAlive is a health-check helper.
func EnsureDBAlive() error {
	return database.Conn().Ping()
}

// guard against unused import
var _ = time.Now
