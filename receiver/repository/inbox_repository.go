package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func GetInboxID(conn *pgx.Conn, email string) (int, error) {

	var id int

	err := conn.QueryRow(
		context.Background(),
		"SELECT id FROM inboxes WHERE email=$1",
		email,
	).Scan(&id)

	return id, err
}

func CreateInbox(conn *pgx.Conn, email string) (int, error) {

	var id int

	err := conn.QueryRow(
		context.Background(),
		`
		INSERT INTO inboxes(
			email,
			type,
			expires_at
		)
		VALUES(
			$1,
			'TEMP',
			NOW()+INTERVAL '1 day'
		)
		RETURNING id
		`,
		email,
	).Scan(&id)

	return id, err
}
