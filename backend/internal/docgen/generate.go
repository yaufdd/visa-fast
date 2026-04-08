package docgen

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Generate writes pass2JSON to a temp file, invokes the Python docgen script,
// and returns the path to the output ZIP file.
//
// pythonScript should be the absolute path to docgen/generate.py.
// uploadsDir is the base uploads directory (ZIP will be placed inside
// uploadsDir/{groupID}/output.zip).
func Generate(ctx context.Context, pythonScript, uploadsDir, groupID string, pass2JSON json.RawMessage) (string, error) {
	// Write pass2 JSON to a temp file so Python can read it.
	tmpDir := filepath.Join(uploadsDir, groupID)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("create group dir: %w", err)
	}

	jsonPath := filepath.Join(tmpDir, "pass2.json")
	if err := os.WriteFile(jsonPath, pass2JSON, 0644); err != nil {
		return "", fmt.Errorf("write pass2 json: %w", err)
	}

	zipPath := filepath.Join(tmpDir, "output.zip")

	cmd := exec.CommandContext(ctx, "python3", pythonScript, groupID, jsonPath, zipPath, "tourists")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("python docgen: %w", err)
	}

	if _, err := os.Stat(zipPath); err != nil {
		return "", fmt.Errorf("output zip not found after docgen: %w", err)
	}

	return zipPath, nil
}

// GenerateFinal generates group-level docs (для Инны, заявка ВЦ).
func GenerateFinal(ctx context.Context, pythonScript, uploadsDir, groupID string, pass2JSON json.RawMessage) (string, error) {
	tmpDir := filepath.Join(uploadsDir, groupID)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("create group dir: %w", err)
	}

	jsonPath := filepath.Join(tmpDir, "pass2.json")
	if err := os.WriteFile(jsonPath, pass2JSON, 0644); err != nil {
		return "", fmt.Errorf("write pass2 json: %w", err)
	}

	zipPath := filepath.Join(tmpDir, "final.zip")

	cmd := exec.CommandContext(ctx, "python3", pythonScript, groupID, jsonPath, zipPath, "final")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("python docgen final: %w", err)
	}

	if _, err := os.Stat(zipPath); err != nil {
		return "", fmt.Errorf("final zip not found: %w", err)
	}

	return zipPath, nil
}
