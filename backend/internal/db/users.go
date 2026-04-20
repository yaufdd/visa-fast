package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	DisplayName  *string    `json:"display_name"`
	Role         string     `json:"role"`
	IsActive     bool       `json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateUser inserts a new user row.
func CreateUser(ctx context.Context, pool *pgxpool.Pool, orgID, email, passwordHash string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO users (org_id, email, password_hash)
		 VALUES ($1, $2, $3) RETURNING id`,
		orgID, email, passwordHash,
	).Scan(&id)
	return id, err
}

// GetUserByEmail returns nil, nil when user does not exist (lookup miss).
func GetUserByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (*User, error) {
	var u User
	err := pool.QueryRow(ctx,
		`SELECT id, org_id, email, password_hash, display_name, role,
		        is_active, last_login_at, created_at, updated_at
		   FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.OrgID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Role,
		&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID returns nil, nil on lookup miss.
func GetUserByID(ctx context.Context, pool *pgxpool.Pool, id string) (*User, error) {
	var u User
	err := pool.QueryRow(ctx,
		`SELECT id, org_id, email, password_hash, display_name, role,
		        is_active, last_login_at, created_at, updated_at
		   FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.OrgID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Role,
		&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// TouchLastLogin sets last_login_at = NOW() for the given user id.
func TouchLastLogin(ctx context.Context, pool *pgxpool.Pool, userID string) error {
	_, err := pool.Exec(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = $1`, userID)
	return err
}
