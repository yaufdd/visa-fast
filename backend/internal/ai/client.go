package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
func callClaude(ctx context.Context, apiKey string, reqBody anthropicRequest) (string, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
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
			return "", fmt.Errorf("unmarshal claude response (status %d): %w — body: %s", resp.StatusCode, err, body)
		}
		if ar.Error != nil {
			return "", fmt.Errorf("claude API error: %s", ar.Error.Message)
		}
		if len(ar.Content) == 0 {
			return "", fmt.Errorf("claude returned empty content")
		}
		return ar.Content[0].Text, nil
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

// Note: isTransientHTTP lives in files.go and is shared with UploadFileToAnthropic.
// Keeping a single definition preserves retry behavior for both code paths.
