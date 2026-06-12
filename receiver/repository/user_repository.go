package repository

import (
	"context"

	"receiver/database"
	"receiver/models"
)

type UserRepository struct{}

func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

func (r *UserRepository) Create(ctx context.Context, email, passwordHash, role string) (*models.User, error) {
	var user models.User
	err := database.Conn.QueryRow(ctx,
		"INSERT INTO users (email, password_hash, role) VALUES ($1, $2, $3) RETURNING id, email, password_hash, role, created_at, updated_at",
		email, passwordHash, role,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	err := database.Conn.QueryRow(ctx,
		"SELECT id, email, password_hash, role, created_at, updated_at FROM users WHERE email = $1",
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int) (*models.User, error) {
	var user models.User
	err := database.Conn.QueryRow(ctx,
		"SELECT id, email, password_hash, role, created_at, updated_at FROM users WHERE id = $1",
		id,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) GetAll(ctx context.Context) ([]models.User, error) {
	rows, err := database.Conn.Query(ctx,
		"SELECT id, email, password_hash, role, created_at, updated_at FROM users ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (r *UserRepository) Update(ctx context.Context, id int, email, role string) error {
	_, err := database.Conn.Exec(ctx,
		"UPDATE users SET email = $1, role = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3",
		email, role, id,
	)
	return err
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id int, passwordHash string) error {
	_, err := database.Conn.Exec(ctx,
		"UPDATE users SET password_hash = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2",
		passwordHash, id,
	)
	return err
}

func (r *UserRepository) Delete(ctx context.Context, id int) error {
	_, err := database.Conn.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	return err
}
