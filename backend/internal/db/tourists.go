package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Tourist struct {
	ID                 string          `json:"id"`
	GroupID            string          `json:"group_id"`
	SubgroupID         *string         `json:"subgroup_id"`
	SubmissionID       *string         `json:"submission_id"`
	SubmissionSnapshot json.RawMessage `json:"submission_snapshot"`
	FlightData         json.RawMessage `json:"flight_data"`
	Translations       json.RawMessage `json:"translations"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

func ListTouristsByGroup(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string) ([]Tourist, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, group_id, subgroup_id, submission_id, submission_snapshot,
		        flight_data, translations, created_at, updated_at
		   FROM tourists
		  WHERE group_id = $1 AND org_id = $2
		  ORDER BY created_at`, groupID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tourist{}
	for rows.Next() {
		var t Tourist
		var snap, flight, tr []byte
		if err := rows.Scan(&t.ID, &t.GroupID, &t.SubgroupID, &t.SubmissionID,
			&snap, &flight, &tr, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.SubmissionSnapshot = snap
		t.FlightData = flight
		t.Translations = tr
		out = append(out, t)
	}
	return out, nil
}

func DeleteTourist(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM tourists WHERE id = $1 AND org_id = $2`, id, orgID)
	return tag.RowsAffected() > 0, err
}

func AssignTouristSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, touristID string, subgroupID *string) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE tourists SET subgroup_id = $1, updated_at = NOW()
		  WHERE id = $2 AND org_id = $3`, subgroupID, touristID, orgID)
	return tag.RowsAffected() > 0, err
}

func UpdateFlightData(ctx context.Context, pool *pgxpool.Pool, orgID, touristID string, data []byte) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE tourists SET flight_data = $1, updated_at = NOW()
		  WHERE id = $2 AND org_id = $3`, data, touristID, orgID)
	return tag.RowsAffected() > 0, err
}

var (
	ErrNotFound        = errors.New("not found")
	ErrAlreadyAttached = errors.New("submission already attached")
)

// AttachSubmissionToGroup — used by POST /api/submissions/:id/attach.
// Single transaction: check submission is pending, insert tourist, mark submission attached.
// All three scoped to orgID.
func AttachSubmissionToGroup(ctx context.Context, pool *pgxpool.Pool, orgID, submissionID, groupID string, subgroupID *string) (string, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var payload []byte
	var status string
	err = tx.QueryRow(ctx,
		`SELECT payload, status FROM tourist_submissions
		  WHERE id = $1 AND org_id = $2 FOR UPDATE`, submissionID, orgID,
	).Scan(&payload, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if status == "attached" {
		return "", ErrAlreadyAttached
	}

	var ok bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1 AND org_id = $2)`,
		groupID, orgID,
	).Scan(&ok)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNotFound
	}

	var touristID string
	err = tx.QueryRow(ctx,
		`INSERT INTO tourists (org_id, group_id, subgroup_id, submission_id, submission_snapshot)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		orgID, groupID, subgroupID, submissionID, payload,
	).Scan(&touristID)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE tourist_submissions SET status = 'attached', updated_at = NOW()
		  WHERE id = $1`, submissionID); err != nil {
		return "", err
	}
	return touristID, tx.Commit(ctx)
}
