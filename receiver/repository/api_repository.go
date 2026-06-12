package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type MessageListItem struct {
	ID         int    `json:"id"`
	Sender     string `json:"sender"`
	Subject    string `json:"subject"`
	ReceivedAt string `json:"received_at"`
}

type MessageDetail struct {
	ID       int    `json:"id"`
	Sender   string `json:"sender"`
	Subject  string `json:"subject"`
	TextBody string `json:"text_body"`
	HtmlBody string `json:"html_body"`
}

func GetInboxMessages(conn *pgx.Conn, email string) ([]MessageListItem, error) {

	rows, err := conn.Query(
		context.Background(),
		`
SELECT
m.id,
m.sender,
m.subject,
m.received_at
FROM messages m
JOIN inboxes i ON m.inbox_id=i.id
WHERE i.email=$1
ORDER BY m.received_at DESC
`,
		email,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var messages []MessageListItem

	for rows.Next() {

		var m MessageListItem

		rows.Scan(
			&m.ID,
			&m.Sender,
			&m.Subject,
			&m.ReceivedAt,
		)

		messages = append(messages, m)

	}

	return messages, nil

}

func GetMessageByID(conn *pgx.Conn, id string) (*MessageDetail, error) {

	var m MessageDetail

	err := conn.QueryRow(
		context.Background(),
		`
SELECT
id,
sender,
subject,
text_body,
html_body
FROM messages
WHERE id=$1
`,
		id,
	).Scan(
		&m.ID,
		&m.Sender,
		&m.Subject,
		&m.TextBody,
		&m.HtmlBody,
	)

	if err != nil {
		return nil, err
	}

	return &m, nil
}
