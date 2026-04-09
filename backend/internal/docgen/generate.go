package docgen

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
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

// GenerateWithSubgroup generates tourist docs for one subgroup into docs/{subgroupName}/.
// It does NOT create a ZIP — call ZipDocsDir afterwards to collect all subgroups.
func GenerateWithSubgroup(ctx context.Context, pythonScript, uploadsDir, groupID, subgroupName string, pass2JSON json.RawMessage) error {
	tmpDir := filepath.Join(uploadsDir, groupID)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return fmt.Errorf("create group dir: %w", err)
	}

	// Write per-subgroup pass2 JSON.
	jsonPath := filepath.Join(tmpDir, "pass2_"+safeFilename(subgroupName)+".json")
	if err := os.WriteFile(jsonPath, pass2JSON, 0644); err != nil {
		return fmt.Errorf("write pass2 json: %w", err)
	}

	// zip_path arg is ignored by Python when subgroup_name is set, but required positionally.
	zipPath := filepath.Join(tmpDir, "output.zip")

	cmd := exec.CommandContext(ctx, "python3", pythonScript, groupID, jsonPath, zipPath, "tourists", subgroupName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("python docgen subgroup: %w", err)
	}
	return nil
}

// ZipDocsDir walks uploadsDir/{groupID}/docs/ and creates a ZIP with folder structure.
func ZipDocsDir(uploadsDir, groupID, zipName string) (string, error) {
	docsDir := filepath.Join(uploadsDir, groupID, "docs")
	zipPath := filepath.Join(uploadsDir, groupID, zipName)

	zf, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("create zip: %w", err)
	}
	defer zf.Close()

	zw := zip.NewWriter(zf)
	defer zw.Close()

	err = filepath.WalkDir(docsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(docsDir, path)
		if err != nil {
			return err
		}
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("walk docs dir: %w", err)
	}
	return zipPath, nil
}

// safeFilename strips characters unsafe for filenames.
func safeFilename(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			out = append(out, '_')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
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
