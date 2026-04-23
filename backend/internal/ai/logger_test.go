package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// captureLogger collects entries written via Logger.Log for assertions.
type captureLogger struct {
	mu      sync.Mutex
	entries []CallLog
}

func (l *captureLogger) Log(_ context.Context, e CallLog) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, e)
	return nil
}

func (l *captureLogger) last() CallLog {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.entries) == 0 {
		return CallLog{}
	}
	return l.entries[len(l.entries)-1]
}

func TestNewGenerationID_isUUIDv4Shape(t *testing.T) {
	id := NewGenerationID()
	if len(id) != 36 {
		t.Fatalf("id length = %d, want 36 — got %q", len(id), id)
	}
	// 8-4-4-4-12 hex with dashes, v4 nibble at position 14 == '4'.
	if id[14] != '4' {
		t.Errorf("version nibble = %c, want '4' — %q", id[14], id)
	}
	// Variant nibble at position 19 is one of 8,9,a,b.
	v := id[19]
	if !(v == '8' || v == '9' || v == 'a' || v == 'b') {
		t.Errorf("variant nibble = %c, want 8/9/a/b — %q", v, id)
	}
}

func TestCallClaude_writesAuditLog_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"content": []map[string]string{{"type": "text", "text": "ok"}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	orig := AnthropicAPIOverride
	AnthropicAPIOverride = srv.URL
	defer func() { AnthropicAPIOverride = orig }()

	log := &captureLogger{}
	ctx := context.Background()
	ctx = WithLogger(ctx, log)
	ctx = WithGenerationID(ctx, "gen-123")
	ctx = WithOrgID(ctx, "org-xyz")
	ctx = WithGroupID(ctx, "group-abc")
	ctx = WithFunctionName(ctx, "translate")

	if _, err := callClaude(ctx, "test-key", anthropicRequest{
		Model:       ModelHaikuTranslate,
		MaxTokens:   256,
		Temperature: 0,
		System:      "test system",
		Messages: []anthropicMessage{{
			Role:    "user",
			Content: []anthropicContent{{Type: "text", Text: "hi"}},
		}},
	}); err != nil {
		t.Fatalf("callClaude: %v", err)
	}

	entry := log.last()
	if entry.Status != "success" {
		t.Errorf("status = %q, want success", entry.Status)
	}
	if entry.FunctionName != "translate" {
		t.Errorf("function_name = %q", entry.FunctionName)
	}
	if entry.OrgID != "org-xyz" || entry.GenerationID != "gen-123" || entry.GroupID != "group-abc" {
		t.Errorf("ctx fields not captured: %+v", entry)
	}
	if entry.Model != ModelHaikuTranslate {
		t.Errorf("model = %q", entry.Model)
	}
	if entry.ResponseText != "ok" {
		t.Errorf("response = %q, want 'ok'", entry.ResponseText)
	}
	if entry.DurationMs < 0 {
		t.Errorf("duration_ms = %d, want >= 0", entry.DurationMs)
	}
}

func TestCallClaude_writesAuditLog_error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()
	orig := AnthropicAPIOverride
	AnthropicAPIOverride = srv.URL
	defer func() { AnthropicAPIOverride = orig }()

	log := &captureLogger{}
	ctx := WithLogger(context.Background(), log)
	ctx = WithGenerationID(ctx, "gen-err")
	ctx = WithOrgID(ctx, "org-err")
	ctx = WithFunctionName(ctx, "programme")

	_, err := callClaude(ctx, "test-key", anthropicRequest{
		Model:     ModelOpusProgramme,
		MaxTokens: 128,
		Messages:  []anthropicMessage{{Role: "user", Content: []anthropicContent{{Type: "text", Text: "x"}}}},
	})
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}

	entry := log.last()
	if entry.Status != "error" {
		t.Errorf("status = %q, want error", entry.Status)
	}
	if entry.ErrorMsg == "" {
		t.Error("error_msg empty on failed call")
	}
}

func TestRedactRequestForLog_stripsBase64Image(t *testing.T) {
	req := anthropicRequest{
		Model:     ModelOpusParser,
		MaxTokens: 256,
		Messages: []anthropicMessage{{
			Role: "user",
			Content: []anthropicContent{
				{Type: "text", Text: "analyse this"},
				{Type: "image", Source: &contentSource{
					Type:      "base64",
					MediaType: "image/jpeg",
					Data:      strings.Repeat("A", 1000), // simulate 1KB base64
				}},
			},
		}},
	}
	redacted := redactRequestForLog(req)
	s := string(redacted)
	if strings.Contains(s, strings.Repeat("A", 1000)) {
		t.Error("redacted JSON still contains the raw base64 payload")
	}
	if !strings.Contains(s, "redacted") {
		t.Errorf("expected placeholder with 'redacted', got: %s", s)
	}
	if !strings.Contains(s, "image/jpeg") {
		t.Error("media_type should be preserved for debug context")
	}
	// Original request must be untouched.
	if req.Messages[0].Content[1].Source.Data != strings.Repeat("A", 1000) {
		t.Error("redaction mutated the original request — must be a copy")
	}
}

func TestCallClaude_noLoggerInCtx_usesNopLogger(t *testing.T) {
	// No panic when callClaude runs without a Logger set up in ctx.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{"content": []map[string]string{{"type": "text", "text": "x"}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()
	orig := AnthropicAPIOverride
	AnthropicAPIOverride = srv.URL
	defer func() { AnthropicAPIOverride = orig }()

	_, err := callClaude(context.Background(), "k", anthropicRequest{
		Model: ModelHaikuTranslate, MaxTokens: 16,
		Messages: []anthropicMessage{{Role: "user", Content: []anthropicContent{{Type: "text", Text: "x"}}}},
	})
	if err != nil {
		t.Fatalf("no-logger path failed: %v", err)
	}
}
