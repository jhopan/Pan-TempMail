package cleanup

import (
	"context"
	"database/sql"
	"log"
	"os"
	"time"
)

// StartCleanupWorker runs a background goroutine that periodically purges
// expired inboxes and their messages/attachments.
func StartCleanupWorker(db *sql.DB) {
	interval := os.Getenv("CLEANUP_INTERVAL")
	if interval == "" {
		interval = "1h"
	}
	d, err := time.ParseDuration(interval)
	if err != nil {
		log.Printf("cleanup: invalid interval %q, defaulting to 1h", interval)
		d = time.Hour
	}

	retention := os.Getenv("MESSAGE_RETENTION")
	if retention == "" {
		retention = "24h"
	}
	r, err := time.ParseDuration(retention)
	if err != nil {
		log.Printf("cleanup: invalid retention %q, defaulting to 24h", retention)
		r = 24 * time.Hour
	}

	go func() {
		ticker := time.NewTicker(d)
		defer ticker.Stop()

		log.Printf("Cleanup worker started (interval=%s, retention=%s)", d, r)

		for range ticker.C {
			purgeExpired(db, r)
		}
	}()
}

// purgeExpired deletes inboxes past their expiry and messages older than retention.
func purgeExpired(db *sql.DB, retention time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Delete expired inboxes (past their expires_at)
	res, err := db.ExecContext(ctx, `DELETE FROM inboxes WHERE expires_at IS NOT NULL AND expires_at < CURRENT_TIMESTAMP`)
	if err != nil {
		log.Printf("cleanup: purge expired inboxes: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("cleanup: purged %d expired inboxes", n)
	}

	// 2. Delete old messages past retention period (even if inbox not expired)
	cutoff := time.Now().Add(-retention)
	res, err = db.ExecContext(ctx, `DELETE FROM messages WHERE received_at < ?`, cutoff)
	if err != nil {
		log.Printf("cleanup: purge old messages: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("cleanup: purged %d old messages (older than %s)", n, retention)
	}

	// 3. Vacuum SQLite periodically to reclaim space (only for sqlite)
	if os.Getenv("DB_DRIVER") == "" || os.Getenv("DB_DRIVER") == "sqlite" {
		if _, err := db.ExecContext(ctx, `PRAGMA optimize`); err != nil {
			log.Printf("cleanup: sqlite optimize: %v", err)
		}
	}
}
