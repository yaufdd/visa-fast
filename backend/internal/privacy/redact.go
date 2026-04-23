// Package privacy provides local (no-AI) pre-processing that strips
// personally identifying information from files BEFORE they are uploaded
// to external services like Anthropic. Today it offers one capability —
// blacking out the passenger/guest name region on a ticket or hotel
// voucher scan — so 152-ФЗ PII never leaves the server.
package privacy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RedactResult mirrors the JSON the Python script prints on success.
type RedactResult struct {
	RegionsRedacted int      `json:"regions_redacted"`
	LabelsFound     []string `json:"labels_found"`
	OutputPath      string   `json:"output_path"`
	// RedactedBytes is filled by RedactScan (not by the Python script).
	// The redacted PNG bytes are what callers upload to Anthropic.
	RedactedBytes []byte `json:"-"`
}

// pythonError is the failure JSON shape used by redact_scan.py.
type pythonError struct {
	Error string `json:"error"`
}

// ErrNoLabelsFound is returned when the redactor cannot locate any known
// name label on the scan. Callers MUST surface this as a user-visible error
// — shipping an un-redacted image to AI would defeat the purpose.
var ErrNoLabelsFound = fmt.Errorf("redact: no name labels found on scan")

// RedactScan writes `inputBytes` to a temporary file, invokes
// `docgen/redact_scan.py` to blacken the name region, and returns the
// redacted PNG bytes plus metadata.
//
// On failure — including the "fail loud when no labels are found" case —
// an error is returned. The caller must NOT fall back to the un-redacted
// bytes; that would undo the whole point of this function.
func RedactScan(ctx context.Context, pythonScript string, inputFilename string, inputBytes []byte) (*RedactResult, error) {
	if pythonScript == "" {
		return nil, fmt.Errorf("redact: pythonScript path is empty")
	}
	tmpDir, err := os.MkdirTemp("", "redact-*")
	if err != nil {
		return nil, fmt.Errorf("redact: tmp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	inExt := strings.ToLower(filepath.Ext(inputFilename))
	if inExt == "" {
		inExt = ".jpg"
	}
	nonce := make([]byte, 6)
	_, _ = rand.Read(nonce)
	inPath := filepath.Join(tmpDir, "in_"+hex.EncodeToString(nonce)+inExt)
	outPath := filepath.Join(tmpDir, "out_"+hex.EncodeToString(nonce)+".png")

	if err := os.WriteFile(inPath, inputBytes, 0o600); err != nil {
		return nil, fmt.Errorf("redact: write input: %w", err)
	}

	cmd := exec.CommandContext(ctx, "python3", pythonScript,
		"--in="+inPath, "--out="+outPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var pe pythonError
		if jerr := json.Unmarshal(stderr.Bytes(), &pe); jerr == nil && pe.Error != "" {
			if strings.Contains(pe.Error, "no name labels found") {
				return nil, ErrNoLabelsFound
			}
			return nil, fmt.Errorf("redact: %s", pe.Error)
		}
		return nil, fmt.Errorf("redact: python exit: %w — stderr: %s", err, stderr.String())
	}

	var res RedactResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return nil, fmt.Errorf("redact: decode python output: %w — stdout: %s", err, stdout.String())
	}
	// The Python script MAY rewrite out_path to `.pdf` for multi-page PDFs
	// (single PNG can't hold multi-page content). Use the path it actually
	// wrote, not the one we requested.
	actualPath := res.OutputPath
	if actualPath == "" {
		actualPath = outPath
	}
	data, err := os.ReadFile(actualPath)
	if err != nil {
		return nil, fmt.Errorf("redact: read redacted output (%s): %w", actualPath, err)
	}
	res.RedactedBytes = data
	res.OutputPath = actualPath
	return &res, nil
}

// OutputFilename returns the suggested filename extension the redactor
// produced for `inputName`. Useful when the caller wants to preserve a
// meaningful filename when uploading to Anthropic.
func OutputFilename(inputName string, actualPath string) string {
	base := inputName
	if i := strings.LastIndex(inputName, "."); i >= 0 {
		base = inputName[:i]
	}
	outExt := filepath.Ext(actualPath)
	if outExt == "" {
		outExt = ".png"
	}
	return base + "_redacted" + outExt
}
