package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
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
	ID         int    `json:"id"`
	Sender     string `json:"sender"`
	Subject    string `json:"subject"`
	ReceivedAt string `json:"received_at"`
	HasAttachment bool `json:"has_attachment"`
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
		args := []interface{}{email, time.Now().Add(-timeout)}
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
