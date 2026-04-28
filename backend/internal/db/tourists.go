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

// GetTouristByID returns a single tourist scoped to the org. Returns
// pgx.ErrNoRows when the tourist doesn't exist or belongs to another org —
// callers should map that to 404 (never 403, to prevent ID enumeration).
func GetTouristByID(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (*Tourist, error) {
	var t Tourist
	var snap, flight, tr []byte
	err := pool.QueryRow(ctx,
		`SELECT id, group_id, subgroup_id, submission_id, submission_snapshot,
		        flight_data, translations, created_at, updated_at
		   FROM tourists
		  WHERE id = $1 AND org_id = $2`, id, orgID).
		Scan(&t.ID, &t.GroupID, &t.SubgroupID, &t.SubmissionID,
			&snap, &flight, &tr, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	t.SubmissionSnapshot = snap
	t.FlightData = flight
	t.Translations = tr
	return &t, nil
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

// DeleteTourist removes the tourist row. If the submission is no longer
// referenced by ANY tourists row afterwards, the submission is released
// back to the 'pending' pool. When parallel attachments exist (same
// submission across multiple finalized groups), status stays 'attached'
// so the remaining rows still reflect the correct pool state.
func DeleteTourist(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var submissionID *string
	err = tx.QueryRow(ctx,
		`DELETE FROM tourists WHERE id = $1 AND org_id = $2 RETURNING submission_id`,
		id, orgID,
	).Scan(&submissionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if submissionID != nil {
		var remaining int
		if err := tx.QueryRow(ctx,
			`SELECT COUNT(*) FROM tourists
			  WHERE submission_id = $1 AND org_id = $2`,
			*submissionID, orgID,
		).Scan(&remaining); err != nil {
			return false, err
		}
		if remaining == 0 {
			if _, err := tx.Exec(ctx,
				`UPDATE tourist_submissions SET status = 'pending', updated_at = NOW()
				  WHERE id = $1 AND org_id = $2 AND status = 'attached'`,
				*submissionID, orgID,
			); err != nil {
				return false, err
			}
		}
	}
	return true, tx.Commit(ctx)
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

// ApplyFlightDataToSubgroup copies the source tourist's flight_data to every
// other tourist in the same subgroup, scoped to org. Returns (found, copiedTo,
// err). found=false when the source tourist does not exist or is not assigned
// to a subgroup. copiedTo counts tourists overwritten (excludes the source).
func ApplyFlightDataToSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, touristID string) (bool, int64, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var groupID string
	var subgroupID *string
	var flight []byte
	err = tx.QueryRow(ctx,
		`SELECT group_id, subgroup_id, flight_data FROM tourists
		  WHERE id = $1 AND org_id = $2`, touristID, orgID,
	).Scan(&groupID, &subgroupID, &flight)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if subgroupID == nil {
		return false, 0, nil
	}

	tag, err := tx.Exec(ctx,
		`UPDATE tourists
		    SET flight_data = $1, updated_at = NOW()
		  WHERE org_id = $2
		    AND group_id = $3
		    AND subgroup_id = $4
		    AND id <> $5`,
		flight, orgID, groupID, *subgroupID, touristID,
	)
	if err != nil {
		return false, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, err
	}
	return true, tag.RowsAffected(), nil
}

var (
	ErrNotFound        = errors.New("not found")
	ErrAlreadyAttached = errors.New("submission already attached")
)

// AttachSubmissionToGroup — used by POST /api/submissions/:id/attach.
// Single transaction: check submission is pending (or its existing
// attachment is in a finalized group → allow parallel insert), insert
// tourist, mark submission attached.
// Returns ErrAlreadyAttached when the submission is currently attached to
// a group that is still active (draft / docs_ready).
// All operations scoped to orgID.
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

	// When the submission is already attached we must confirm that EVERY
	// existing tourists row belongs to a finalized group — otherwise the
	// tourist is still actively being processed in another case and we
	// refuse the second attach. Finalized rows are left in place as
	// historical records; the new INSERT creates a parallel row.
	if status == "attached" {
		var activeExists bool
		err := tx.QueryRow(ctx,
			`SELECT EXISTS (
			   SELECT 1 FROM tourists t
			     JOIN groups g ON g.id = t.group_id
			    WHERE t.submission_id = $1
			      AND t.org_id = $2
			      AND g.status NOT IN ('submitted', 'visa_issued')
			 )`, submissionID, orgID,
		).Scan(&activeExists)
		if err != nil {
			return "", err
		}
		if activeExists {
			return "", ErrAlreadyAttached
		}
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
