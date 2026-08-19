package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"receiver/database"
	"receiver/models"

	"golang.org/x/crypto/bcrypt"
)

type UserRepository struct{}

func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var u models.User
	query := placeholder(`SELECT id, email, password_hash, role, created_at, updated_at FROM users WHERE email = $1`)

	err := database.Conn().QueryRowContext(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int) (*models.User, error) {
	var u models.User
	query := placeholder(`SELECT id, email, password_hash, role, created_at, updated_at FROM users WHERE id = $1`)

	err := database.Conn().QueryRowContext(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) GetAll(ctx context.Context) ([]models.User, error) {
	query := placeholder(`SELECT id, email, password_hash, role, created_at, updated_at FROM users ORDER BY id`)

	rows, err := database.Conn().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("get all users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}
	return users, nil
}

func (r *UserRepository) Create(ctx context.Context, email, passwordHash, role string) (*models.User, error) {
	query := placeholder(`INSERT INTO users (email, password_hash, role) VALUES ($1, $2, $3) RETURNING id, email, password_hash, role, created_at, updated_at`)

	var u models.User
	err := database.Conn().QueryRowContext(ctx, query, email, passwordHash, role).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) Update(ctx context.Context, id int, email, role string) error {
	query := placeholder(`UPDATE users SET email = $1, role = $2, updated_at = $3 WHERE id = $4`)
	_, err := database.Conn().ExecContext(ctx, query, email, role, time.Now().UTC(), id)
	return err
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id int, passwordHash string) error {
	query := placeholder(`UPDATE users SET password_hash = $1, updated_at = $2 WHERE id = $3`)
	_, err := database.Conn().ExecContext(ctx, query, passwordHash, time.Now().UTC(), id)
	return err
}

func (r *UserRepository) Delete(ctx context.Context, id int) error {
	query := placeholder(`DELETE FROM users WHERE id = $1`)
	_, err := database.Conn().ExecContext(ctx, query, id)
	return err
}

// HashPassword generates a bcrypt hash.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword verifies a bcrypt hash.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// guard against unused import
var _ = sql.ErrNoRows
