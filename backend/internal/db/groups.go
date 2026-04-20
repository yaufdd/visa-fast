package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Group struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"-"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Notes     *string   `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ListGroups(ctx context.Context, pool *pgxpool.Pool, orgID string) ([]Group, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, org_id, name, status, notes, created_at, updated_at
		   FROM groups WHERE org_id = $1
		   ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.OrgID, &g.Name, &g.Status, &g.Notes, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func GetGroup(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (*Group, error) {
	var g Group
	err := pool.QueryRow(ctx,
		`SELECT id, org_id, name, status, notes, created_at, updated_at
		   FROM groups WHERE id = $1 AND org_id = $2`, id, orgID,
	).Scan(&g.ID, &g.OrgID, &g.Name, &g.Status, &g.Notes, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func CreateGroup(ctx context.Context, pool *pgxpool.Pool, orgID, name string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO groups (org_id, name) VALUES ($1, $2) RETURNING id`,
		orgID, name,
	).Scan(&id)
	return id, err
}

func DeleteGroup(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM groups WHERE id = $1 AND org_id = $2`, id, orgID)
	return tag.RowsAffected() > 0, err
}

func UpdateGroupStatus(ctx context.Context, pool *pgxpool.Pool, orgID, id, status string) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE groups SET status = $1, updated_at = NOW()
		  WHERE id = $2 AND org_id = $3`, status, id, orgID)
	return tag.RowsAffected() > 0, err
}

func UpdateGroupNotes(ctx context.Context, pool *pgxpool.Pool, orgID, id string, notes *string) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE groups SET notes = $1, updated_at = NOW()
		  WHERE id = $2 AND org_id = $3`, notes, id, orgID)
	return tag.RowsAffected() > 0, err
}
