package repository

import (
	"database/sql"
	"fmt"
)

// placeholder mengubah penanda $1, $2 menjadi ?, bentuk yang dipakai SQLite.
//
// Semua kueri berparameter menulis $1/$2 lalu melewati fungsi ini. SQLite adalah satu-satunya
// mesin yang didukung sekarang, jadi fungsinya satu arah; kalau suatu hari ada mesin kedua,
// hanya berkas ini yang perlu bercabang.
func placeholder(query string) string {
	return strconvQuote(query)
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
