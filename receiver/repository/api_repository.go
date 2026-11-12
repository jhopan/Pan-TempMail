package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"receiver/database"
)

// PageParams holds pagination parameters.
type PageParams struct {
	Page    int // 1-indexed page number
	PerPage int // items per page
}

// DefaultPageParams returns sensible defaults.
func DefaultPageParams(page, perPage int) PageParams {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}
	return PageParams{Page: page, PerPage: perPage}
}

// PaginatedResult wraps a page of data with total count and pagination metadata.
type PaginatedResult[T any] struct {
	Data       []T `json:"data"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
}

type MessageListItem struct {
	ID            int    `json:"id"`
	Sender        string `json:"sender"`
	Subject       string `json:"subject"`
	ReceivedAt    string `json:"received_at"`
	HasAttachment bool   `json:"has_attachment"`
}

type MessageDetail struct {
	ID       int    `json:"id"`
	Sender   string `json:"sender"`
	Subject  string `json:"subject"`
	TextBody string `json:"text_body"`
	HtmlBody string `json:"html_body"`
	Codes    string `json:"codes"`
	Links    string `json:"links"`
}

// GetInboxMessages returns a paginated list of messages for an inbox.
func GetInboxMessages(db *sql.DB, email string, page PageParams) (*PaginatedResult[MessageListItem], error) {
	// Count total
	var total int
	countQuery := `SELECT COUNT(*) FROM messages m JOIN inboxes i ON m.inbox_id = i.id WHERE i.email = $1`
	countQuery = placeholder(countQuery)
	if err := db.QueryRowContext(context.Background(), countQuery, email).Scan(&total); err != nil {
		return nil, fmt.Errorf("count messages: %w", err)
	}

	// Calculate pagination
	offset := (page.Page - 1) * page.PerPage
	totalPages := 0
	if total > 0 {
		totalPages = (total + page.PerPage - 1) / page.PerPage
	}

	// Query messages with pagination
	query := `SELECT m.id, m.sender, m.subject, m.received_at,
		EXISTS(SELECT 1 FROM attachments a WHERE a.message_id = m.id) AS has_attachment
		FROM messages m
		JOIN inboxes i ON m.inbox_id = i.id
		WHERE i.email = $1
		ORDER BY m.received_at DESC
		LIMIT $2 OFFSET $3`
	query = placeholder(query)

	rows, err := db.QueryContext(context.Background(), query, email, page.PerPage, offset)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []MessageListItem
	for rows.Next() {
		var m MessageListItem
		if err := rows.Scan(&m.ID, &m.Sender, &m.Subject, &m.ReceivedAt, &m.HasAttachment); err != nil {
			continue
		}
		messages = append(messages, m)
	}

	if messages == nil {
		messages = []MessageListItem{}
	}

	return &PaginatedResult[MessageListItem]{
		Data:       messages,
		Total:      total,
		Page:       page.Page,
		PerPage:    page.PerPage,
		TotalPages: totalPages,
	}, nil
}

// GetMessageByID returns a single message by ID, including extracted codes/links.
func GetMessageByID(db *sql.DB, id string) (*MessageDetail, error) {
	var m MessageDetail
	query := `SELECT id, sender, subject, text_body, html_body, codes, links FROM messages WHERE id = $1`
	query = placeholder(query)

	err := db.QueryRowContext(context.Background(), query, id).Scan(
		&m.ID, &m.Sender, &m.Subject, &m.TextBody, &m.HtmlBody, &m.Codes, &m.Links,
	)
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	return &m, nil
}

// SaveMessage saves a new message with extracted OTP codes and links.
func SaveMessage(db *sql.DB, inboxID int, sender, subject, textBody, htmlBody string, codes, links []string) (int, error) {
	codesJSON := arrayToJSON(codes)
	linksJSON := arrayToJSON(links)

	query := `INSERT INTO messages (inbox_id, sender, subject, html_body, text_body, codes, links)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`
	query = placeholder(query)

	var messageID int
	err := db.QueryRowContext(context.Background(), query,
		inboxID, sender, subject, htmlBody, textBody, codesJSON, linksJSON,
	).Scan(&messageID)
	if err != nil {
		return 0, fmt.Errorf("save message: %w", err)
	}
	return messageID, nil
}

// WaitForMessage blocks until a message matching criteria arrives, or timeout.
// This is the automation-friendly feature: instead of polling, hold the connection.
func WaitForMessage(db *sql.DB, email string, timeout time.Duration, matchSubject, matchFrom string) (*MessageDetail, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	for time.Now().Before(deadline) {
		// Check for new messages
		var m MessageDetail
		query := `SELECT id, sender, subject, text_body, html_body, codes, links
			FROM messages
			WHERE inbox_id = (SELECT id FROM inboxes WHERE email = $1)
			AND received_at >= $2`
		// SQLite stores received_at as CURRENT_TIMESTAMP (UTC, "2006-01-02 15:04:05").
		// Pass cutoff in the same format so string comparison works.
		args := []interface{}{email, time.Now().UTC().Add(-timeout).Format("2006-01-02 15:04:05")}
		if matchSubject != "" {
			query += " AND subject LIKE $3"
			args = append(args, "%"+matchSubject+"%")
		}
		if matchFrom != "" {
			nextArg := strconv.Itoa(len(args) + 1)
			query += " AND sender LIKE $" + nextArg
			args = append(args, "%"+matchFrom+"%")
		}
		query += " ORDER BY received_at DESC LIMIT 1"
		query = placeholder(query)

		err := db.QueryRowContext(context.Background(), query, args...).Scan(
			&m.ID, &m.Sender, &m.Subject, &m.TextBody, &m.HtmlBody, &m.Codes, &m.Links,
		)
		if err == nil {
			return &m, nil
		}
		if err != sql.ErrNoRows {
			return nil, fmt.Errorf("wait for message: %w", err)
		}
		time.Sleep(pollInterval)
	}
	return nil, fmt.Errorf("timeout waiting for message")
}

// MessageSeries counts messages per hour for the last `hours` hours, oldest bucket first.
// The bool reports whether the row cap was hit, so callers can mark the series as a lower bound.
//
// Bucket terakhir adalah jam yang memuat `now`, jadi grafik 24 bar selalu berakhir di jam ini.
// received_at dibaca sebagai string dan dibucket di Go, sehingga tidak ada fungsi tanggal khusus
// SQLite atau Postgres di query.
func MessageSeries(db *sql.DB, now time.Time, hours int, cap int) ([]int, bool, error) {
	if hours <= 0 {
		return []int{}, false, nil
	}
	series := make([]int, hours)
	if db == nil {
		return series, false, nil
	}

	now = now.UTC()
	lastStart := now.Truncate(time.Hour)
	// windowStart adalah awal bucket 0; baris lebih tua dari ini diabaikan.
	windowStart := lastStart.Add(-time.Duration(hours-1) * time.Hour)
	// SQLite menyimpan received_at sebagai CURRENT_TIMESTAMP (UTC, "2006-01-02 15:04:05"):
	// cutoff dengan format yang sama membuat perbandingan string tetap benar.
	cutoff := now.Add(-time.Duration(hours) * time.Hour).Format("2006-01-02 15:04:05")

	// Urut dari yang terbaru: kalau cap tercapai, yang dipotong adalah jam paling lama,
	// bukan jam terakhir yang justru paling penting di grafik.
	query := `SELECT received_at FROM messages WHERE received_at >= $1 ORDER BY received_at DESC`
	if cap > 0 {
		query += fmt.Sprintf(" LIMIT %d", cap+1)
	}
	query = placeholder(query)

	rows, err := db.QueryContext(context.Background(), query, cutoff)
	if err != nil {
		return series, false, fmt.Errorf("message series: %w", err)
	}
	defer rows.Close()

	fetched := 0
	for rows.Next() {
		fetched++
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue // satu baris rusak tidak boleh menggagalkan seluruh seri
		}
		hour, ok := parseMessageHour(raw)
		if !ok {
			continue // timestamp aneh diabaikan, bukan dihitung di bucket mana pun
		}
		index := int(hour.Sub(windowStart) / time.Hour)
		if index < 0 {
			continue // lebih tua dari jendela
		}
		if index >= hours {
			index = hours - 1 // clock skew: baris masa depan masuk bucket terakhir
		}
		series[index]++
	}

	if err := rows.Err(); err != nil {
		return series, false, fmt.Errorf("message series rows: %w", err)
	}
	// fetched menghitung baris yang dikembalikan query (termasuk yang diabaikan): kalau lebih
	// dari cap, seri ini hanya batas bawah.
	return series, cap > 0 && fetched > cap, nil
}

// parseMessageHour reads the hour a stored timestamp belongs to. Hanya 13 karakter pertama
// dipakai ("2006-01-02 15"), dan "T" dinormalkan jadi spasi supaya bentuk ISO juga terbaca.
func parseMessageHour(raw string) (time.Time, bool) {
	s := strings.TrimSpace(raw)
	if len(s) < 13 {
		return time.Time{}, false
	}
	s = strings.Replace(s[:13], "T", " ", 1)
	hour, err := time.Parse("2006-01-02 15", s)
	if err != nil {
		return time.Time{}, false
	}
	return hour.UTC(), true
}

// arrayToJSON converts a string slice to a JSON array string for DB storage.
func arrayToJSON(arr []string) string {
	if len(arr) == 0 {
		return "[]"
	}
	var b []byte
	b = append(b, '[')
	for i, s := range arr {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '"')
		// Escape quotes and backslashes
		for _, c := range s {
			if c == '"' || c == '\\' {
				b = append(b, '\\')
			}
			b = append(b, string(c)...)
		}
		b = append(b, '"')
	}
	b = append(b, ']')
	return string(b)
}

// unused import guard
var _ = database.Conn
