package repository

import (
	"context"
	"database/sql"
)

// GetSetting reads a settings value by key. Returns "" if not found.
func GetSetting(db *sql.DB, key string) string {
	var v string
	err := db.QueryRowContext(context.Background(), placeholder(`SELECT value FROM settings WHERE key = $1`), key).Scan(&v)
	if err != nil {
		return ""
	}
	return v
}

// SetSetting upserts a settings value.
func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.ExecContext(context.Background(),
		placeholder(`INSERT INTO settings (key, value) VALUES ($1, $2)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`),
		key, value)
	return err
}
