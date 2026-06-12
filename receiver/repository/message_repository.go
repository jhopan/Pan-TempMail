package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func SaveMessage(
	conn *pgx.Conn,
	inboxID int,
	sender string,
	subject string,
	textBody string,
	htmlBody string,
) (int, error) {

	var messageID int
	err := conn.QueryRow(
		context.Background(),
		`
		INSERT INTO messages(
			inbox_id,
			sender,
			subject,
			text_body,
			html_body,
			received_at
		)
		VALUES($1,$2,$3,$4,$5,NOW())
		RETURNING id
		`,
		inboxID,
		sender,
		subject,
		textBody,
		htmlBody,
	).Scan(&messageID)

	return messageID, err
}
