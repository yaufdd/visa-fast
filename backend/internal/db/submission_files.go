package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SubmissionFile mirrors the submission_files row.
type SubmissionFile struct {
	ID           string    `json:"id"`
	OrgID        string    `json:"org_id"`
	SubmissionID string    `json:"submission_id"`
	FileType     string    `json:"file_type"`
	FilePath     string    `json:"file_path"`
	OriginalName string    `json:"original_name"`
	MIMEType     string    `json:"mime_type"`
	SizeBytes    int64     `json:"size_bytes"`
	CreatedAt    time.Time `json:"created_at"`
}

// InsertOrReplaceSubmissionFile inserts a new row, or replaces an existing
// one of the same (submission_id, file_type). Returns the id of the row
// after the upsert, plus the previous file_path (or "" + replaced=false if
// no previous row existed) so the caller can delete the obsolete file from
// disk.
//
// file_type values are validated by the DB CHECK constraint
// ('passport_internal','passport_foreign','ticket','voucher'); HTTP
// handlers should reject bad input earlier with a 400.
func InsertOrReplaceSubmissionFile(ctx context.Context, pool *pgxpool.Pool, f SubmissionFile) (id string, oldPath string, replaced bool, err error) {
	// The partial unique index created by migration 000024 covers only
	// file_type = 'passport_foreign'. PostgreSQL requires the WHERE clause
	// to be spelled out explicitly in ON CONFLICT — a plain
	// ON CONFLICT (submission_id, file_type) cannot match a partial index.
	err = pool.QueryRow(ctx,
		`WITH old AS (
		   SELECT file_path FROM submission_files
		    WHERE submission_id = $2 AND file_type = $3
		 )
		 INSERT INTO submission_files
		   (org_id, submission_id, file_type, file_path, original_name, mime_type, size_bytes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (submission_id, file_type) WHERE file_type = 'passport_foreign' DO UPDATE
		   SET file_path = EXCLUDED.file_path,
		       original_name = EXCLUDED.original_name,
		       mime_type = EXCLUDED.mime_type,
		       size_bytes = EXCLUDED.size_bytes,
		       created_at = NOW()
		 RETURNING id, COALESCE((SELECT file_path FROM old), '')`,
		f.OrgID, f.SubmissionID, f.FileType, f.FilePath, f.OriginalName, f.MIMEType, f.SizeBytes,
	).Scan(&id, &oldPath)
	if err != nil {
		return "", "", false, err
	}
	return id, oldPath, oldPath != "", nil
}

// InsertSubmissionFile inserts a fresh submission_files row without any
// conflict handling. Use for file_types that allow multiple rows per
// submission (ticket / voucher post-migration 000023). The caller must
// supply a unique file_path — typically built via storage.BuildSubmission
// MultiFilePath with a random suffix.
func InsertSubmissionFile(ctx context.Context, pool *pgxpool.Pool, f SubmissionFile) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO submission_files
		   (org_id, submission_id, file_type, file_path, original_name, mime_type, size_bytes)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING id`,
		f.OrgID, f.SubmissionID, f.FileType, f.FilePath, f.OriginalName, f.MIMEType, f.SizeBytes,
	).Scan(&id)
	return id, err
}

// ListSubmissionFiles returns all files for a submission within the org.
// Empty slice (not nil error) when none.
func ListSubmissionFiles(ctx context.Context, pool *pgxpool.Pool, orgID, submissionID string) ([]SubmissionFile, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, org_id, submission_id, file_type, file_path,
		        original_name, mime_type, size_bytes, created_at
		   FROM submission_files
		  WHERE org_id = $1 AND submission_id = $2
		  ORDER BY created_at DESC`, orgID, submissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SubmissionFile{}
	for rows.Next() {
		var f SubmissionFile
		if err := rows.Scan(&f.ID, &f.OrgID, &f.SubmissionID, &f.FileType,
			&f.FilePath, &f.OriginalName, &f.MIMEType, &f.SizeBytes, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// GetSubmissionFile loads one row scoped to org. Returns pgx.ErrNoRows
// when not found (callers compare with errors.Is).
func GetSubmissionFile(ctx context.Context, pool *pgxpool.Pool, orgID, fileID string) (SubmissionFile, error) {
	var f SubmissionFile
	err := pool.QueryRow(ctx,
		`SELECT id, org_id, submission_id, file_type, file_path,
		        original_name, mime_type, size_bytes, created_at
		   FROM submission_files
		  WHERE id = $1 AND org_id = $2`,
		fileID, orgID,
	).Scan(&f.ID, &f.OrgID, &f.SubmissionID, &f.FileType,
		&f.FilePath, &f.OriginalName, &f.MIMEType, &f.SizeBytes, &f.CreatedAt)
	return f, err
}

// DeleteSubmissionFile removes a row scoped to org + file id, returning
// the on-disk file_path so the caller can rm the file. ok=false (no
// error) when no row matched.
func DeleteSubmissionFile(ctx context.Context, pool *pgxpool.Pool, orgID, fileID string) (string, bool, error) {
	var filePath string
	err := pool.QueryRow(ctx,
		`DELETE FROM submission_files
		  WHERE id = $1 AND org_id = $2
		  RETURNING file_path`,
		fileID, orgID,
	).Scan(&filePath)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return filePath, true, nil
}
