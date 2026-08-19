package repository

import (
	"database/sql"
	"fmt"
	"os"
)

// dbDriverName returns "postgres" or "sqlite3" based on DB_DRIVER env.
func dbDriverName() string {
	d := os.Getenv("DB_DRIVER")
	if d == "postgres" {
		return "postgres"
	}
	return "sqlite3"
}

// placeholder converts $1, $2 to ? for SQLite, keeps $1, $2 for PostgreSQL.
func placeholder(query string) string {
	if dbDriverName() == "sqlite3" {
		// Replace $1, $2, $3... with ?
		return strconvQuote(query)
	}
	return query
}

// strconvQuote replaces $N placeholders with ? for SQLite.
func strconvQuote(query string) string {
	var result []byte
	i := 0
	for i < len(query) {
		if query[i] == '$' && i+1 < len(query) && query[i+1] >= '1' && query[i+1] <= '9' {
			result = append(result, '?')
			// Skip the digit
			i += 2
			// Skip any additional digits (for $10, $11, etc.)
			for i < len(query) && query[i] >= '0' && query[i] <= '9' {
				i++
			}
		} else {
			result = append(result, query[i])
			i++
		}
	}
	return string(result)
}

// Ping is a simple health check for the database.
func Ping(db *sql.DB) error {
	return db.Ping()
}

// Stats returns database connection stats.
func Stats(db *sql.DB) sql.DBStats {
	return db.Stats()
}

// FormatError wraps a database error with context.
func FormatError(op string, err error) error {
	if err == nil {
		return nil
	}
	if err == sql.ErrNoRows {
		return fmt.Errorf("%s: not found", op)
	}
	return fmt.Errorf("%s: %w", op, err)
}
