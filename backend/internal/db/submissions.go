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
	// Set only when status='attached' — identifies the group the submission
	// is currently bound to so the manager UI can show context / decide
	// whether re-attach is permitted (depends on CurrentGroupStatus).
	CurrentGroupID     *string `json:"current_group_id,omitempty"`
	CurrentGroupName   *string `json:"current_group_name,omitempty"`
	CurrentGroupStatus *string `json:"current_group_status,omitempty"`
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

// CreateDraftSubmission inserts a placeholder row used by either the
// public form (source="tourist") or the manager-side wizard
// (source="manager") before the row is finalised. The row exists only so
// file uploads (submission_files) have a foreign key to bind to.
// consent_accepted is FALSE — the real consent stamp happens at finalize
// time via UpdateSubmissionPayloadByID. consent_accepted_at is NOT NULL
// on the schema, so we write NOW() as a sentinel; finalize re-stamps it.
//
// The source argument lets reports / audits distinguish drafts that came
// from a tourist filling the public slug form vs. a manager creating a
// submission directly in the dashboard. Callers should pass "tourist" or
// "manager"; any other string is accepted by the DB as long as it passes
// the source CHECK constraint on tourist_submissions.
func CreateDraftSubmission(ctx context.Context, pool *pgxpool.Pool, orgID, consentVersion, source string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO tourist_submissions
		   (org_id, payload, consent_accepted, consent_accepted_at, consent_version, source, status)
		 VALUES ($1, '{}'::jsonb, FALSE, NOW(), $2, $3, 'draft') RETURNING id`,
		orgID, consentVersion, source,
	).Scan(&id)
	return id, err
}

// UpdateSubmissionPayloadByID flips a draft submission to 'pending' with
// the final payload and consent stamp. Returns pgx.ErrNoRows if no row
// matched (wrong org, wrong id, or status != 'draft' — all collapsed to
// "not found" so the caller cannot distinguish, preventing ID enumeration
// and preventing accidental overwrites of an already-pending submission).
func UpdateSubmissionPayloadByID(ctx context.Context, pool *pgxpool.Pool, orgID, submissionID string, payload []byte, consentVersion string) error {
	tag, err := pool.Exec(ctx,
		`UPDATE tourist_submissions
		    SET payload = $1,
		        consent_accepted = TRUE,
		        consent_accepted_at = NOW(),
		        consent_version = $2,
		        status = 'pending',
		        updated_at = NOW()
		  WHERE id = $3 AND org_id = $4 AND status = 'draft'`,
		payload, consentVersion, submissionID, orgID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func ListSubmissions(ctx context.Context, pool *pgxpool.Pool, orgID, q, status string) ([]TouristSubmission, error) {
	args := []any{orgID}
	where := []string{"ts.org_id = $1"}
	if q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf("ts.payload ->> 'name_lat' ILIKE $%d", len(args)))
	}
	// Support comma-separated status filter ("pending,attached") so the
	// manager UI can request the re-attach-eligible pool in one call.
	if status != "" {
		statuses := strings.Split(status, ",")
		placeholders := make([]string, 0, len(statuses))
		for _, s := range statuses {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			args = append(args, s)
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		if len(placeholders) > 0 {
			where = append(where, "ts.status IN ("+strings.Join(placeholders, ",")+")")
		}
	}
	// LATERAL pulls the MOST RECENT tourists row for each submission — a
	// submission may now have multiple rows (a finalized group + a fresh
	// one for the tourist's next trip), and the UI filters based on the
	// latest attachment's group status.
	sql := `SELECT ts.id, ts.payload, ts.consent_accepted, ts.consent_accepted_at,
	               ts.consent_version, ts.source, ts.status,
	               ts.created_at, ts.updated_at,
	               latest.group_id, latest.group_name, latest.group_status
	          FROM tourist_submissions ts
	          LEFT JOIN LATERAL (
	            SELECT g.id AS group_id, g.name AS group_name, g.status AS group_status
	              FROM tourists t
	              JOIN groups   g ON g.id = t.group_id AND g.org_id = ts.org_id
	             WHERE t.submission_id = ts.id
	               AND t.org_id = ts.org_id
	             ORDER BY t.created_at DESC
	             LIMIT 1
	          ) latest ON TRUE
	         WHERE ` + strings.Join(where, " AND ") + `
	         ORDER BY ts.created_at DESC LIMIT 500`

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TouristSubmission{}
	for rows.Next() {
		var s TouristSubmission
		var payload []byte
		var groupID, groupName, groupStatus *string
		if err := rows.Scan(&s.ID, &payload, &s.ConsentAccepted, &s.ConsentAcceptedAt,
			&s.ConsentVersion, &s.Source, &s.Status, &s.CreatedAt, &s.UpdatedAt,
			&groupID, &groupName, &groupStatus); err != nil {
			return nil, err
		}
		s.Payload = payload
		s.CurrentGroupID = groupID
		s.CurrentGroupName = groupName
		s.CurrentGroupStatus = groupStatus
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
