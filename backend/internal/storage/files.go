package storage

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
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
