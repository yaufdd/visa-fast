package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"
)

// isTransientHTTP returns true for network-level errors worth retrying.
func isTransientHTTP(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "EOF") ||
		strings.Contains(s, "connection reset") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "TLS handshake") ||
		strings.Contains(s, "broken pipe")
}

const anthropicFilesAPI = "https://api.anthropic.com/v1/files"

// UploadFileToAnthropic uploads a file to the Anthropic Files API and returns
// the file_id. The file_id can be referenced in future Claude API calls instead
// of sending the raw bytes each time, avoiding request-size limits.
func UploadFileToAnthropic(apiKey, filename string, data []byte) (string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Determine MIME type from extension.
	ext := strings.ToLower(filepath.Ext(filename))
	mimeType := "application/octet-stream"
	switch ext {
	case ".pdf":
		mimeType = "application/pdf"
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".png":
		mimeType = "image/png"
	}

	// Write file field with explicit Content-Type.
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	h.Set("Content-Type", mimeType)
	fw, err := mw.CreatePart(h)
	if err != nil {
		return "", fmt.Errorf("create multipart part: %w", err)
	}
	if _, err := fw.Write(data); err != nil {
		return "", fmt.Errorf("write file data: %w", err)
	}
	mw.Close()

	bodyBytes := buf.Bytes()
	contentType := mw.FormDataContentType()

	// Retry transient network errors (Anthropic endpoint occasionally drops EOF mid-upload).
	var resp *http.Response
	var body []byte
	backoff := 500 * time.Millisecond
	client := &http.Client{Timeout: 120 * time.Second}
	for attempt := 0; attempt < 4; attempt++ {
		req, rerr := http.NewRequest(http.MethodPost, anthropicFilesAPI, bytes.NewReader(bodyBytes))
		if rerr != nil {
			return "", fmt.Errorf("build files API request: %w", rerr)
		}
		req.ContentLength = int64(len(bodyBytes))
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("anthropic-beta", "files-api-2025-04-14")

		var derr error
		resp, derr = client.Do(req)
		if derr == nil {
			body, derr = io.ReadAll(resp.Body)
			resp.Body.Close()
			if derr == nil {
				break
			}
		}
		if !isTransientHTTP(derr) {
			return "", fmt.Errorf("files API http: %w", derr)
		}
		time.Sleep(backoff)
		backoff *= 2
		if attempt == 3 {
			return "", fmt.Errorf("files API http (after retries): %w", derr)
		}
	}

	var result struct {
		ID    string `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("unmarshal files API response: %w — body: %s", err, body)
	}
	if result.Error != nil {
		return "", fmt.Errorf("Anthropic Files API error: %s", result.Error.Message)
	}
	if result.ID == "" {
		return "", fmt.Errorf("Anthropic Files API returned empty id — body: %s", body)
	}
	return result.ID, nil
}
