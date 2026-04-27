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

// SaveSubmissionFile saves a tourist-uploaded scan associated with a public
// draft submission. The on-disk layout is:
//
//	<uploadsDir>/<orgID>/submissions/<submissionID>/<fileType><ext>
//
// The filename is the file_type plus an extension; we deliberately drop the
// original name on disk because the submission_files table only allows ONE
// row per (submission_id, file_type) — replacements overwrite the same path,
// which keeps the on-disk view consistent with the DB without extra cleanup.
//
// extOf the original filename takes priority; if the original has no
// extension we fall back to a mime-derived ext (.pdf / .jpg / .png) so the
// file at least has a sensible suffix on disk for ops debugging.
func SaveSubmissionFile(uploadsDir, orgID, submissionID, fileType, originalFilename string, data []byte, mime string) (string, error) {
	dir := filepath.Join(uploadsDir, orgID, "submissions", submissionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create submission upload dir: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(originalFilename))
	if ext == "" {
		switch mime {
		case "application/pdf":
			ext = ".pdf"
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		}
	}
	destPath := filepath.Join(dir, fileType+ext)
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", fmt.Errorf("write submission file: %w", err)
	}
	return destPath, nil
}
