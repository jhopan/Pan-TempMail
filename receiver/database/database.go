package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite, no CGO needed
)

var (
	conn *sql.DB
	once sync.Once
)

// Connect membuka database SQLite. Satu node, satu berkas, WAL: cukup untuk temp mail
// pribadi, dan tidak ada dialek kedua yang harus dirawat.
//
// DB_DRIVER hanya diterima sebagai "sqlite" (atau kosong). Nilai lain ditolak dengan pesan
// jelas supaya deployment lama yang masih menyetel postgres tidak diam-diam salah koneksi.
func Connect() error {
	if driver := strings.TrimSpace(os.Getenv("DB_DRIVER")); driver != "" && driver != "sqlite" {
		return fmt.Errorf("DB_DRIVER=%s tidak didukung lagi: aplikasi ini hanya memakai SQLite, hapus DB_DRIVER dari env", driver)
	}

	dbDriver := "sqlite"
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		// Default: data/tempmail.db relatif terhadap working dir
		dbPath = "data/tempmail.db"
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create db directory: %w", err)
	}
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(on)"

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
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		// Index for fast inbox lookups
		`CREATE INDEX IF NOT EXISTS idx_messages_inbox_id ON messages(inbox_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_received_at ON messages(received_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_inboxes_email ON inboxes(email)`,
		`CREATE TABLE IF NOT EXISTS cf_profiles (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL DEFAULT '',
			account_id TEXT NOT NULL,
			token TEXT NOT NULL DEFAULT '',
			worker_deployed INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS cf_domains (
			domain TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL,
			zone_id TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 0,
			routing INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cf_domains_profile ON cf_domains(profile_id)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("exec migration: %w (query: %s)", err, q)
		}
	}

	return nil
}
