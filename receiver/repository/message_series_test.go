package repository

import (
	"database/sql"
	"testing"
	"time"
)

// insertSeriesMessage menyisipkan satu pesan dengan received_at eksplisit.
func insertSeriesMessage(t *testing.T, db *sql.DB, inboxID int64, receivedAt string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO messages (inbox_id, sender, subject, received_at) VALUES (?, ?, ?, ?)`,
		inboxID, "series@test.invalid", "series", receivedAt); err != nil {
		t.Fatalf("insert message %q: %v", receivedAt, err)
	}
}

func seriesInbox(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO inboxes (email) VALUES (?)`, "series@test.invalid")
	if err != nil {
		t.Fatalf("insert inbox: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("inbox id: %v", err)
	}
	return id
}

func TestMessageSeriesBucketsInsertedMessages(t *testing.T) {
	db := newCFTestDB(t)
	inboxID := seriesInbox(t, db)

	// now 14:37 -> bucket terakhir (23) adalah jam 14:00, bucket 0 adalah jam 15:00 hari sebelumnya.
	now := time.Date(2026, 11, 12, 14, 37, 21, 0, time.UTC)
	stamp := func(at time.Time) string { return at.UTC().Format("2006-01-02 15:04:05") }

	insertSeriesMessage(t, db, inboxID, stamp(now.Add(-3*time.Hour)))                 // jam 11:00 -> bucket 20
	insertSeriesMessage(t, db, inboxID, stamp(now.Add(-3*time.Hour+10*time.Minute)))  // jam 11:00 -> bucket 20
	insertSeriesMessage(t, db, inboxID, stamp(now))                                   // jam berjalan -> bucket 23
	insertSeriesMessage(t, db, inboxID, stamp(now.Add(-1*time.Hour)))                 // jam 13:00 -> bucket 22
	insertSeriesMessage(t, db, inboxID, "bukan-tanggal")                              // rusak, harus diabaikan
	insertSeriesMessage(t, db, inboxID, stamp(now.Add(-30*time.Hour)))                // di luar query
	insertSeriesMessage(t, db, inboxID, stamp(now.Add(-23*time.Hour-50*time.Minute))) // di luar jendela bucket
	insertSeriesMessage(t, db, inboxID, stamp(now.Add(2*time.Hour)))                  // clock skew -> bucket 23

	series, capped, err := MessageSeries(db, now, 24, 5000)
	if err != nil {
		t.Fatalf("message series: %v", err)
	}
	if len(series) != 24 {
		t.Fatalf("series length = %d, want 24", len(series))
	}
	if capped {
		t.Error("series reported capped, want false with cap 5000")
	}
	if series[20] != 2 {
		t.Errorf("series[20] = %d, want 2 (dua baris 3 jam lalu)", series[20])
	}
	if series[21] != 0 {
		t.Errorf("series[21] = %d, want 0", series[21])
	}
	if series[22] != 1 {
		t.Errorf("series[22] = %d, want 1 (baris satu jam lalu)", series[22])
	}
	if series[23] != 2 {
		t.Errorf("series[23] = %d, want 2 (jam berjalan + clock skew)", series[23])
	}
	total := 0
	for _, n := range series {
		total += n
	}
	if total != 5 {
		t.Errorf("sum(series) = %d, want 5: baris rusak dan baris di luar jendela harus diabaikan", total)
	}
}

func TestMessageSeriesCapFlag(t *testing.T) {
	db := newCFTestDB(t)
	inboxID := seriesInbox(t, db)

	now := time.Date(2026, 11, 12, 14, 37, 21, 0, time.UTC)
	stamp := func(at time.Time) string { return at.UTC().Format("2006-01-02 15:04:05") }
	for i := range 4 {
		insertSeriesMessage(t, db, inboxID, stamp(now.Add(-3*time.Hour).Add(time.Duration(i)*time.Minute)))
	}

	series, capped, err := MessageSeries(db, now, 24, 2)
	if err != nil {
		t.Fatalf("message series with cap: %v", err)
	}
	if !capped {
		t.Error("series did not report capped with cap 2 over 4 rows")
	}
	if len(series) != 24 {
		t.Fatalf("series length = %d, want 24 even when capped", len(series))
	}

	// cap <= 0 berarti tanpa batas: semua baris ikut dihitung.
	series, capped, err = MessageSeries(db, now, 24, 0)
	if err != nil {
		t.Fatalf("message series without cap: %v", err)
	}
	if capped {
		t.Error("series reported capped with cap 0, want no cap")
	}
	if series[20] != 4 {
		t.Errorf("series[20] = %d, want 4 uncapped rows", series[20])
	}
}

func TestMessageSeriesEdgeCases(t *testing.T) {
	db := newCFTestDB(t)
	now := time.Date(2026, 11, 12, 14, 37, 21, 0, time.UTC)

	empty, capped, err := MessageSeries(db, now, 0, 10)
	if err != nil {
		t.Fatalf("message series with hours 0: %v", err)
	}
	if len(empty) != 0 || capped {
		t.Errorf("hours 0 = %v capped=%v, want empty slice and no cap", empty, capped)
	}

	zeros, capped, err := MessageSeries(nil, now, 24, 5000)
	if err != nil {
		t.Fatalf("message series with nil db: %v", err)
	}
	if len(zeros) != 24 || capped {
		t.Errorf("nil db = %d buckets capped=%v, want 24 zeros", len(zeros), capped)
	}
	for i, n := range zeros {
		if n != 0 {
			t.Fatalf("nil db bucket %d = %d, want 0", i, n)
		}
	}
}
