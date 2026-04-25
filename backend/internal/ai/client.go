package ai

import (
	"bytes"
	"context"
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

// Anthropic API constants. Model ids are overridable at call sites.
const (
	AnthropicAPI           = "https://api.anthropic.com/v1/messages"
	ModelOpusParser        = "claude-opus-4-6"  // ticket/voucher scan parsing
	ModelOpusProgramme     = "claude-opus-4-7"  // creative itinerary
	ModelHaikuTranslate    = "claude-haiku-4-5" // free-text translation
	anthropicVersionHeader = "2023-06-01"
	anthropicBetaFilesHdr  = "files-api-2025-04-14"
)

// AnthropicAPIOverride, if non-empty, is used instead of AnthropicAPI.
// Test-only hook (do not set in production).
var AnthropicAPIOverride string

func anthropicURL() string {
	if AnthropicAPIOverride != "" {
		return AnthropicAPIOverride
	}
	return AnthropicAPI
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	System      string             `json:"system"`
	Messages    []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type   string         `json:"type"`
	Text   string         `json:"text,omitempty"`
	Source *contentSource `json:"source,omitempty"`
}

type contentSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	FileID    string `json:"file_id,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// FileInput represents one file to send to Claude (either an Anthropic
// Files API reference or raw inline bytes).
type FileInput struct {
	AnthropicFileID string
	Name            string
	Data            []byte
}

// callClaude POSTs to /v1/messages with retries on transient failures.
//
// Every call writes an audit row via the Logger installed in ctx (or
// NopLogger when absent). Retries count as ONE audit row — we record the
// final outcome, not each retry attempt.
func callClaude(ctx context.Context, apiKey string, reqBody anthropicRequest) (string, error) {
	started := time.Now()
	logEntry := CallLog{
		OrgID:        OrgIDFromContext(ctx),
		GroupID:      GroupIDFromContext(ctx),
		SubgroupID:   SubgroupIDFromContext(ctx),
		GenerationID: GenerationIDFromContext(ctx),
		FunctionName: FunctionNameFromContext(ctx),
		Provider:     "anthropic",
		Model:        reqBody.Model,
		RequestJSON:  redactRequestForLog(reqBody),
		StartedAt:    started,
		// Default to error — success path overrides. Any early return
		// skipping the explicit assignments below still persists a valid row.
		Status: "error",
	}
	logger := LoggerFromContext(ctx)
	defer func() {
		logEntry.FinishedAt = time.Now()
		logEntry.DurationMs = int(logEntry.FinishedAt.Sub(started) / time.Millisecond)
		// Best-effort; ignore logger errors so an audit failure can never
		// take down the AI call path.
		_ = logger.Log(ctx, logEntry)
	}()

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		logEntry.Status = "error"
		logEntry.ErrorMsg = err.Error()
		return "", fmt.Errorf("marshal claude request: %w", err)
	}

	client := &http.Client{Timeout: 300 * time.Second}
	backoff := 1 * time.Second
	const maxAttempts = 4

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, anthropicURL(), bytes.NewReader(bodyBytes))
		if rerr != nil {
			return "", fmt.Errorf("build claude request: %w", rerr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", anthropicVersionHeader)
		req.Header.Set("anthropic-beta", anthropicBetaFilesHdr)

		resp, derr := client.Do(req)
		if derr != nil {
			lastErr = fmt.Errorf("claude http (attempt %d/%d): %w", attempt, maxAttempts, derr)
			if !isTransientHTTP(derr) || attempt == maxAttempts {
				return "", lastErr
			}
			if err := sleepCtx(ctx, backoff); err != nil {
				return "", err
			}
			backoff *= 2
			continue
		}

		body, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr != nil {
			lastErr = fmt.Errorf("read claude response (attempt %d/%d): %w", attempt, maxAttempts, rerr)
			if attempt == maxAttempts {
				return "", lastErr
			}
			if err := sleepCtx(ctx, backoff); err != nil {
				return "", err
			}
			backoff *= 2
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("claude upstream %d (attempt %d/%d): %s",
				resp.StatusCode, attempt, maxAttempts, summarizeUpstreamBody(resp, body))
			if attempt == maxAttempts {
				return "", lastErr
			}
			if err := sleepCtx(ctx, backoff); err != nil {
				return "", err
			}
			backoff *= 2
			continue
		}

		if !isJSONResponse(resp, body) {
			return "", fmt.Errorf("claude returned non-JSON response (status %d, content-type %q): %s",
				resp.StatusCode, resp.Header.Get("Content-Type"), summarizeUpstreamBody(resp, body))
		}

		var ar anthropicResponse
		if err := json.Unmarshal(body, &ar); err != nil {
			wrapped := fmt.Errorf("unmarshal claude response (status %d): %w — body: %s", resp.StatusCode, err, body)
			logEntry.Status = "error"
			logEntry.ErrorMsg = wrapped.Error()
			return "", wrapped
		}
		if ar.Error != nil {
			logEntry.Status = "error"
			logEntry.ErrorMsg = "claude API error: " + ar.Error.Message
			return "", fmt.Errorf("claude API error: %s", ar.Error.Message)
		}
		if len(ar.Content) == 0 {
			logEntry.Status = "error"
			logEntry.ErrorMsg = "claude returned empty content"
			return "", fmt.Errorf("claude returned empty content")
		}
		logEntry.Status = "success"
		logEntry.ResponseText = ar.Content[0].Text
		return ar.Content[0].Text, nil
	}
	logEntry.Status = "error"
	if lastErr != nil {
		logEntry.ErrorMsg = lastErr.Error()
	}
	return "", lastErr
}

// extractJSON strips prose around the outermost JSON value. It picks
// whichever of `{` or `[` appears first in the string and slices to the
// matching last `}` / `]`. This correctly handles both bare objects and
// arrays of objects (where naively searching for `{...}` first would
// slice from the first inner `{` to the last inner `}`).
func extractJSON(s string) string {
	firstObj := strings.IndexByte(s, '{')
	firstArr := strings.IndexByte(s, '[')
	var start int
	var closeCh byte
	switch {
	case firstObj >= 0 && (firstArr < 0 || firstObj < firstArr):
		start, closeCh = firstObj, '}'
	case firstArr >= 0:
		start, closeCh = firstArr, ']'
	default:
		return s
	}
	end := strings.LastIndexByte(s, closeCh)
	if end > start {
		return s[start : end+1]
	}
	return s
}

func isJSONResponse(resp *http.Response, body []byte) bool {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "json") {
		return true
	}
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

func summarizeUpstreamBody(resp *http.Response, body []byte) string {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	lower := strings.ToLower(string(body))
	if strings.Contains(ct, "html") || strings.Contains(lower, "<html") {
		if strings.Contains(lower, "cloudflare") {
			return fmt.Sprintf("cloudflare HTML error page (status %d) — likely Anthropic upstream outage", resp.StatusCode)
		}
		return fmt.Sprintf("HTML error page (status %d)", resp.StatusCode)
	}
	const maxLen = 300
	if len(body) > maxLen {
		return string(body[:maxLen]) + "…"
	}
	return string(body)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// snippet returns a short, log-friendly view of a response body.
func snippet(body []byte) string {
	s := string(body)
	if strings.Contains(strings.ToLower(s), "cloudflare") && strings.Contains(strings.ToLower(s), "<html") {
		return "cloudflare HTML error page — likely Anthropic upstream outage"
	}
	const maxLen = 300
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}

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
				// Retry 5xx (Anthropic overloaded or Cloudflare upstream error).
				if resp.StatusCode >= 500 {
					if attempt == 3 {
						return "", fmt.Errorf("files API upstream %d after retries: %s", resp.StatusCode, snippet(body))
					}
					time.Sleep(backoff)
					backoff *= 2
					continue
				}
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
