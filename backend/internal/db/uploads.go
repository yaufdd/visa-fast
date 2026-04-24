package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Upload struct {
	ID              string     `json:"id"`
	GroupID         string     `json:"group_id"`
	TouristID       *string    `json:"tourist_id"`
	SubgroupID      *string    `json:"subgroup_id"`
	FileType        string     `json:"file_type"`
	FilePath        string     `json:"file_path"`
	AnthropicFileID *string    `json:"anthropic_file_id"`
	CreatedAt       time.Time  `json:"created_at"`
	ParsedAt        *time.Time `json:"parsed_at"`
}

func InsertUpload(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string, touristID *string, fileType, filePath string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO uploads (org_id, group_id, tourist_id, file_type, file_path)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		orgID, groupID, touristID, fileType, filePath,
	).Scan(&id)
	return id, err
}

func ListTouristUploads(ctx context.Context, pool *pgxpool.Pool, orgID, touristID string) ([]Upload, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, group_id, tourist_id, subgroup_id, file_type, file_path,
		        anthropic_file_id, created_at, parsed_at
		   FROM uploads WHERE tourist_id = $1 AND org_id = $2
		   ORDER BY created_at DESC`, touristID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Upload{}
	for rows.Next() {
		var u Upload
		if err := rows.Scan(&u.ID, &u.GroupID, &u.TouristID, &u.SubgroupID, &u.FileType,
			&u.FilePath, &u.AnthropicFileID, &u.CreatedAt, &u.ParsedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

func SetUploadAnthropicID(ctx context.Context, pool *pgxpool.Pool, orgID, uploadID, fileID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE uploads SET anthropic_file_id = $1 WHERE id = $2 AND org_id = $3`,
		fileID, uploadID, orgID)
	return err
}

// GetTouristUpload loads one upload row scoped to org + tourist. Returns
// pgx.ErrNoRows if not found.
func GetTouristUpload(ctx context.Context, pool *pgxpool.Pool, orgID, touristID, uploadID string) (Upload, error) {
	var u Upload
	err := pool.QueryRow(ctx,
		`SELECT id, group_id, tourist_id, subgroup_id, file_type, file_path,
		        anthropic_file_id, created_at, parsed_at
		   FROM uploads
		  WHERE id = $1 AND org_id = $2 AND tourist_id = $3`,
		uploadID, orgID, touristID,
	).Scan(&u.ID, &u.GroupID, &u.TouristID, &u.SubgroupID, &u.FileType,
		&u.FilePath, &u.AnthropicFileID, &u.CreatedAt, &u.ParsedAt)
	return u, err
}

// MarkUploadParsed stamps parsed_at = now() so the UI hides the "Распознать"
// button for this upload.
func MarkUploadParsed(ctx context.Context, pool *pgxpool.Pool, orgID, uploadID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE uploads SET parsed_at = NOW() WHERE id = $1 AND org_id = $2`,
		uploadID, orgID)
	return err
}

// DeleteTouristUpload removes an uploads row that belongs to the given org +
// tourist. Returns the on-disk file_path so the caller can remove the file,
// and ok=true if a row was deleted.
func DeleteTouristUpload(ctx context.Context, pool *pgxpool.Pool, orgID, touristID, uploadID string) (string, bool, error) {
	var filePath string
	err := pool.QueryRow(ctx,
		`DELETE FROM uploads
		  WHERE id = $1 AND org_id = $2 AND tourist_id = $3
		  RETURNING file_path`,
		uploadID, orgID, touristID,
	).Scan(&filePath)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return filePath, true, nil
}
