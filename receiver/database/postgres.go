package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	conn *sql.DB
	once sync.Once
)

// Connect initializes the SQLite database.
// If DB_DRIVER=postgres, it connects to PostgreSQL instead (for scaling up).
func Connect() error {
	driver := os.Getenv("DB_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}

	var dsn string
	var dbDriver string

	switch driver {
	case "postgres":
		dbDriver = "postgres"
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_PORT"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
		)
	default:
		dbDriver = "sqlite3"
		dbPath := os.Getenv("DB_PATH")
		if dbPath == "" {
			// Default: data/tempmail.db relative to working dir
			dbPath = "data/tempmail.db"
		}
		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			return fmt.Errorf("create db directory: %w", err)
		}
		dsn = dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_sync=NORMAL&_foreign_keys=on"
	}

	db, err := sql.Open(dbDriver, dsn)
	if err != nil {
		return fmt.Errorf("open %s: %w", dbDriver, err)
	}

	// Connection pool settings
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping %s: %w", dbDriver, err)
	}

	conn = db

	// Apply migrations
	if err := migrate(db); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	log.Printf("Connected to %s (%s)", dbDriver, dsn)
	return nil
}

// Conn returns the singleton database connection.
func Conn() *sql.DB {
	return conn
}

// Close gracefully closes the database.
func Close() error {
	if conn != nil {
		log.Println("Closing database connection...")
		return conn.Close()
	}
	return nil
}

// migrate creates tables if they don't exist.
func migrate(db *sql.DB) error {
	// Use SQLite-compatible schema (works for both sqlite and postgres with minor tweaks)
	queries := []string{
		`CREATE TABLE IF NOT EXISTS inboxes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL DEFAULT 'TEMP',
			owner_id INTEGER,
			expires_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			inbox_id INTEGER NOT NULL REFERENCES inboxes(id) ON DELETE CASCADE,
			sender TEXT,
			subject TEXT,
			html_body TEXT,
			text_body TEXT,
			codes TEXT,
			links TEXT,
			received_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS attachments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
			filename TEXT,
			content_type TEXT,
			size INTEGER,
			data BLOB
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email VARCHAR(255) UNIQUE NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			role VARCHAR(50) NOT NULL DEFAULT 'user',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		// Index for fast inbox lookups
		`CREATE INDEX IF NOT EXISTS idx_messages_inbox_id ON messages(inbox_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_received_at ON messages(received_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_inboxes_email ON inboxes(email)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("exec migration: %w (query: %s)", err, q)
		}
	}

	return nil
}
