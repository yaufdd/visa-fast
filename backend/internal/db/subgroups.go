package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Subgroup struct {
	ID             string    `json:"id"`
	GroupID        string    `json:"group_id"`
	Name           string    `json:"name"`
	SortOrder      int       `json:"sort_order"`
	ProgrammeNotes *string   `json:"programme_notes"`
	CreatedAt      time.Time `json:"created_at"`
}

func ListSubgroups(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string) ([]Subgroup, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, group_id, name, sort_order, programme_notes, created_at
		   FROM subgroups WHERE group_id = $1 AND org_id = $2
		   ORDER BY sort_order, created_at`, groupID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Subgroup{}
	for rows.Next() {
		var s Subgroup
		if err := rows.Scan(&s.ID, &s.GroupID, &s.Name, &s.SortOrder, &s.ProgrammeNotes, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// CreateSubgroup only succeeds if the parent group belongs to the same org.
func CreateSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, groupID, name string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO subgroups (org_id, group_id, name)
		 SELECT $1, $2, $3 WHERE EXISTS (SELECT 1 FROM groups WHERE id = $2 AND org_id = $1)
		 RETURNING id`, orgID, groupID, name,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil // group not found or not owned by this org
	}
	return id, err
}

func UpdateSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, id, name string, sortOrder int) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE subgroups SET name = $1, sort_order = $2
		  WHERE id = $3 AND org_id = $4`, name, sortOrder, id, orgID)
	return tag.RowsAffected() > 0, err
}

// UpdateSubgroupProgrammeNotes stores the manager's free-text hints fed
// into programme generation for this specific subgroup. Empty string is
// stored as SQL NULL.
func UpdateSubgroupProgrammeNotes(ctx context.Context, pool *pgxpool.Pool, orgID, id string, notes *string) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE subgroups SET programme_notes = $1
		  WHERE id = $2 AND org_id = $3`, notes, id, orgID)
	return tag.RowsAffected() > 0, err
}

// GetSubgroupProgrammeNotes returns the subgroup's programme_notes (or nil
// if no subgroup / no notes). Used by the generation pipeline.
func GetSubgroupProgrammeNotes(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (*string, error) {
	var notes *string
	err := pool.QueryRow(ctx,
		`SELECT programme_notes FROM subgroups WHERE id = $1 AND org_id = $2`,
		id, orgID,
	).Scan(&notes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return notes, nil
}

func DeleteSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM subgroups WHERE id = $1 AND org_id = $2`, id, orgID)
	return tag.RowsAffected() > 0, err
}
