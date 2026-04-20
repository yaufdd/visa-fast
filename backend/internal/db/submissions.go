package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TouristSubmission struct {
	ID                string          `json:"id"`
	Payload           json.RawMessage `json:"payload"`
	ConsentAccepted   bool            `json:"consent_accepted"`
	ConsentAcceptedAt time.Time       `json:"consent_accepted_at"`
	ConsentVersion    string          `json:"consent_version"`
	Source            string          `json:"source"`
	Status            string          `json:"status"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// CreateSubmissionForOrg is used by both the public slug endpoint and
// manager "create manually" flow. orgID comes from either slug resolve
// or the session.
func CreateSubmissionForOrg(ctx context.Context, pool *pgxpool.Pool, orgID string, payload []byte, consentVersion, source string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO tourist_submissions
		   (org_id, payload, consent_accepted, consent_accepted_at, consent_version, source)
		 VALUES ($1, $2, TRUE, NOW(), $3, $4) RETURNING id`,
		orgID, payload, consentVersion, source,
	).Scan(&id)
	return id, err
}

func ListSubmissions(ctx context.Context, pool *pgxpool.Pool, orgID, q, status string) ([]TouristSubmission, error) {
	args := []any{orgID}
	where := []string{"org_id = $1"}
	if q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf("payload ->> 'name_lat' ILIKE $%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	sql := `SELECT id, payload, consent_accepted, consent_accepted_at, consent_version,
	               source, status, created_at, updated_at
	          FROM tourist_submissions
	         WHERE ` + strings.Join(where, " AND ") + `
	         ORDER BY created_at DESC LIMIT 500`

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TouristSubmission{}
	for rows.Next() {
		var s TouristSubmission
		var payload []byte
		if err := rows.Scan(&s.ID, &payload, &s.ConsentAccepted, &s.ConsentAcceptedAt,
			&s.ConsentVersion, &s.Source, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Payload = payload
		out = append(out, s)
	}
	return out, nil
}

func GetSubmission(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (*TouristSubmission, error) {
	var s TouristSubmission
	var payload []byte
	err := pool.QueryRow(ctx,
		`SELECT id, payload, consent_accepted, consent_accepted_at, consent_version,
		        source, status, created_at, updated_at
		   FROM tourist_submissions WHERE id = $1 AND org_id = $2`, id, orgID,
	).Scan(&s.ID, &payload, &s.ConsentAccepted, &s.ConsentAcceptedAt,
		&s.ConsentVersion, &s.Source, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Payload = payload
	return &s, nil
}

func UpdateSubmission(ctx context.Context, pool *pgxpool.Pool, orgID, id string, payload []byte) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE tourist_submissions SET payload = $1, updated_at = NOW()
		  WHERE id = $2 AND org_id = $3`, payload, id, orgID)
	return tag.RowsAffected() > 0, err
}

func ArchiveSubmission(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE tourist_submissions SET status = 'archived', updated_at = NOW()
		  WHERE id = $1 AND org_id = $2 AND status != 'archived'`, id, orgID)
	return tag.RowsAffected() > 0, err
}

func EraseSubmission(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`UPDATE tourists SET submission_snapshot = NULL, submission_id = NULL
		  WHERE submission_id = $1 AND org_id = $2`, id, orgID); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM tourist_submissions WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, tx.Commit(ctx)
}
