package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

// SaveFile saves an uploaded file to UPLOADS_DIR/{groupID}/{fileType}_{filename}.
// Returns the full path to the saved file.
func SaveFile(uploadsDir, groupID, fileType, filename string, file multipart.File) (string, error) {
	dir := filepath.Join(uploadsDir, groupID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}

	destName := fmt.Sprintf("%s_%s", fileType, filepath.Base(filename))
	destPath := filepath.Join(dir, destName)

	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create destination file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return destPath, nil
}

// SaveFileBytes saves raw bytes to UPLOADS_DIR/{groupID}/{fileType}_{filename}.
func SaveFileBytes(uploadsDir, groupID, fileType, filename string, data []byte) (string, error) {
	dir := filepath.Join(uploadsDir, groupID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create upload dir: %w", err)
	}
	destName := fmt.Sprintf("%s_%s", fileType, filepath.Base(filename))
	destPath := filepath.Join(dir, destName)
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return destPath, nil
}

// ReadFile reads the contents of a file at the given path.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", path, err)
	}
	return data, nil
}

// extForSubmissionFile picks the on-disk extension. The original filename
// is preferred (lowercased); if it has no extension we fall back to a
// mime-derived suffix so the file at least has a sensible name on disk for
// ops debugging. Only ".pdf", ".jpg", ".jpeg", ".png" are allowed through;
// anything else collapses to "" (caller writes a bare "<type>" file).
func extForSubmissionFile(originalFilename, mime string) string {
	ext := strings.ToLower(filepath.Ext(originalFilename))
	switch ext {
	case ".pdf", ".jpg", ".jpeg", ".png":
		return ext
	}
	switch mime {
	case "application/pdf":
		return ".pdf"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	}
	return ""
}

// SubmissionDirPerm / SubmissionFilePerm are the umask used for the public
// submission-file tree. Public-form scans contain passport PII; we keep
// them stricter than the existing SaveFile / SaveFileBytes (0755 / 0644)
// which are behind auth and inside per-group dirs.
const (
	SubmissionDirPerm  os.FileMode = 0700
	SubmissionFilePerm os.FileMode = 0600
)

// BuildSubmissionFilePath returns the canonical on-disk path for a public
// submission scan, without writing anything. Used by the streaming upload
// handler which needs the final path before the bytes are committed (it
// writes to a sibling tmp file and renames after the DB upsert succeeds).
//
// Layout:
//
//	<uploadsDir>/<orgID>/submissions/<submissionID>/<fileType><ext>
func BuildSubmissionFilePath(uploadsDir, orgID, submissionID, fileType, originalFilename, mime string) (string, error) {
	dir := filepath.Join(uploadsDir, orgID, "submissions", submissionID)
	return filepath.Join(dir, fileType+extForSubmissionFile(originalFilename, mime)), nil
}

// SaveSubmissionFile saves a tourist-uploaded scan associated with a public
// draft submission. Thin wrapper around BuildSubmissionFilePath + an
// in-memory write. Only kept for callers that already have the bytes in
// memory; the streaming upload handler bypasses this and writes to a tmp
// file directly.
//
// The filename is the file_type plus an extension; we deliberately drop the
// original name on disk because the submission_files table only allows ONE
// row per (submission_id, file_type) — replacements overwrite the same path,
// which keeps the on-disk view consistent with the DB without extra cleanup.
func SaveSubmissionFile(uploadsDir, orgID, submissionID, fileType, originalFilename string, data []byte, mime string) (string, error) {
	destPath, err := BuildSubmissionFilePath(uploadsDir, orgID, submissionID, fileType, originalFilename, mime)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, SubmissionDirPerm); err != nil {
		return "", fmt.Errorf("create submission upload dir: %w", err)
	}
	if err := os.WriteFile(destPath, data, SubmissionFilePerm); err != nil {
		return "", fmt.Errorf("write submission file: %w", err)
	}
	return destPath, nil
}
