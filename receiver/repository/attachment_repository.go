package repository

import (
	"context"
	"receiver/models"

	"github.com/jackc/pgx/v5"
)

func SaveAttachment(conn *pgx.Conn, messageID int, filename, contentType, path string) error {
	query := `
		INSERT INTO attachments (message_id, filename, content_type, path, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`
	_, err := conn.Exec(context.Background(), query, messageID, filename, contentType, path)
	return err
}

func GetAttachmentsByMessageID(conn *pgx.Conn, messageID int) ([]models.Attachment, error) {
	query := `
		SELECT id, message_id, filename, content_type, path
		FROM attachments
		WHERE message_id = $1
		ORDER BY id
	`

	rows, err := conn.Query(context.Background(), query, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attachments []models.Attachment
	for rows.Next() {
		var a models.Attachment
		err := rows.Scan(&a.ID, &a.MessageID, &a.FileName, &a.ContentType, &a.Path)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, a)
	}

	return attachments, nil
}

func GetAttachmentByID(conn *pgx.Conn, id int) (*models.Attachment, error) {
	query := `
		SELECT id, message_id, filename, content_type, path
		FROM attachments
		WHERE id = $1
	`

	var a models.Attachment
	err := conn.QueryRow(context.Background(), query, id).Scan(
		&a.ID, &a.MessageID, &a.FileName, &a.ContentType, &a.Path,
	)

	if err != nil {
		return nil, err
	}

	return &a, nil
}
