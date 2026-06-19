package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrUserConflict = errors.New("user already exists")
)

type User struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
	Role         string
	Status       string
	AvatarURL    *string
}

type CreateUserParams struct {
	Email        string
	DisplayName  string
	PasswordHash string
}

func (s *PostgresStore) CreateUser(ctx context.Context, params CreateUserParams) (*User, error) {
	var user User
	err := s.db.QueryRow(ctx, `
		INSERT INTO users (email, display_name, password_hash, role, status)
		VALUES ($1, $2, $3, 'user', 'active')
		RETURNING id, email, display_name, password_hash, role, status, avatar_url
	`, params.Email, params.DisplayName, params.PasswordHash).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.AvatarURL,
	)
	if isUniqueViolation(err) {
		return nil, ErrUserConflict
	}
	if err != nil {
		return nil, fmt.Errorf("create user: %w", scrubDatabaseError(err))
	}
	return &user, nil
}

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return s.getUser(ctx, `email = $1`, email)
}

func (s *PostgresStore) GetUserByID(ctx context.Context, id string) (*User, error) {
	return s.getUser(ctx, `id = $1`, id)
}

func (s *PostgresStore) getUser(ctx context.Context, predicate string, value string) (*User, error) {
	var user User
	err := s.db.QueryRow(ctx, `
		SELECT id, email, display_name, password_hash, role, status, avatar_url
		FROM users
		WHERE `+predicate+`
	`, value).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.PasswordHash,
		&user.Role,
		&user.Status,
		&user.AvatarURL,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", scrubDatabaseError(err))
	}
	return &user, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
