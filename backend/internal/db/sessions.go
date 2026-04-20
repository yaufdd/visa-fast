package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionLookup is what middleware needs on every protected request:
// userID + orgID in one row (JOIN users).
type SessionLookup struct {
	UserID    string
	OrgID     string
	ExpiresAt time.Time
}

// CreateSession inserts a new session row with ttl lifetime.
func CreateSession(ctx context.Context, pool *pgxpool.Pool, userID, token string, ttl time.Duration) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO sessions (user_id, token, expires_at)
		 VALUES ($1, $2, NOW() + $3::interval)`,
		userID, token, ttl.String(),
	)
	return err
}

// LookupSession joins sessions + users to get user_id and org_id in one round-trip.
// Returns nil, nil if token does not exist, is expired, or user is inactive.
func LookupSession(ctx context.Context, pool *pgxpool.Pool, token string) (*SessionLookup, error) {
	var s SessionLookup
	err := pool.QueryRow(ctx,
		`SELECT s.user_id, u.org_id, s.expires_at
		   FROM sessions s
		   JOIN users u ON u.id = s.user_id
		  WHERE s.token = $1
		    AND s.expires_at > NOW()
		    AND u.is_active = TRUE`,
		token,
	).Scan(&s.UserID, &s.OrgID, &s.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// TouchSession updates last_seen_at and, when extend is true, extends expires_at.
func TouchSession(ctx context.Context, pool *pgxpool.Pool, token string, extend bool, ttl time.Duration) error {
	var q string
	var args []any
	if extend {
		q = `UPDATE sessions SET expires_at = NOW() + $2::interval, last_seen_at = NOW() WHERE token = $1`
		args = []any{token, ttl.String()}
	} else {
		q = `UPDATE sessions SET last_seen_at = NOW() WHERE token = $1`
		args = []any{token}
	}
	_, err := pool.Exec(ctx, q, args...)
	return err
}

// DeleteSession removes a session row (used by logout).
func DeleteSession(ctx context.Context, pool *pgxpool.Pool, token string) error {
	_, err := pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

// DeleteExpiredSessions is a cron helper — removes old rows.
func DeleteExpiredSessions(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
