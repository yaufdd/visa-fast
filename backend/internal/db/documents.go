package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Document struct {
	ID          string          `json:"id"`
	GroupID     string          `json:"group_id"`
	Pass2JSON   json.RawMessage `json:"pass2_json"`
	ZipPath     string          `json:"zip_path"`
	GeneratedAt time.Time       `json:"generated_at"`
	CreatedAt   time.Time       `json:"created_at"`
}

// CreateDocument inserts a documents row and returns the new row's id.
func CreateDocument(ctx context.Context, pool *pgxpool.Pool, orgID, groupID, zipPath string, pass2 []byte, generatedAt time.Time) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO documents (org_id, group_id, pass2_json, zip_path, generated_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		orgID, groupID, pass2, zipPath, generatedAt,
	).Scan(&id)
	return id, err
}

func ListDocumentsForGroup(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string) ([]Document, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, group_id, pass2_json, zip_path, generated_at, created_at
		   FROM documents WHERE group_id = $1 AND org_id = $2
		   ORDER BY created_at DESC`, groupID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var d Document
		var pass2 []byte
		if err := rows.Scan(&d.ID, &d.GroupID, &pass2, &d.ZipPath, &d.GeneratedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		d.Pass2JSON = pass2
		out = append(out, d)
	}
	return out, nil
}

func LatestDocumentForGroup(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string) (*Document, error) {
	var d Document
	var pass2 []byte
	err := pool.QueryRow(ctx,
		`SELECT id, group_id, pass2_json, zip_path, generated_at, created_at
		   FROM documents WHERE group_id = $1 AND org_id = $2
		   ORDER BY generated_at DESC LIMIT 1`, groupID, orgID,
	).Scan(&d.ID, &d.GroupID, &pass2, &d.ZipPath, &d.GeneratedAt, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	d.Pass2JSON = pass2
	return &d, nil
}
