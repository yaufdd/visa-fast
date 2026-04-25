package yandex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// decodeGPTRequest reads and JSON-decodes the request body sent to the
// fake Foundation Models server.
func decodeGPTRequest(t *testing.T, r *http.Request) gptRequestBody {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var body gptRequestBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return body
}

// writeGPTResponse encodes a single-alternative successful response.
func writeGPTResponse(t *testing.T, w http.ResponseWriter, text string) {
	t.Helper()
	resp := map[string]any{
		"result": map[string]any{
			"alternatives": []map[string]any{
				{
					"message": map[string]any{"role": "assistant", "text": text},
					"status":  "ALTERNATIVE_STATUS_FINAL",
				},
			},
			"usage":        map[string]any{"inputTextTokens": "10", "completionTokens": "5", "totalTokens": "15"},
			"modelVersion": "test-model-v1",
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func TestChat_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want Bearer test-token", got)
		}
		if got := r.Header.Get("x-folder-id"); got != "test-folder" {
			t.Errorf("x-folder-id = %q, want test-folder", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		if r.URL.Path != "/foundationModels/v1/completion" {
			t.Errorf("path = %q", r.URL.Path)
		}

		body := decodeGPTRequest(t, r)
		if body.ModelURI != "gpt://test-folder/yandexgpt/rc" {
			t.Errorf("modelUri = %q", body.ModelURI)
		}
		if body.CompletionOptions.ResponseFormat != "text" {
			t.Errorf("responseFormat = %q, want text", body.CompletionOptions.ResponseFormat)
		}
		if body.CompletionOptions.MaxTokens != "2048" {
			t.Errorf("maxTokens = %q, want \"2048\" (string)", body.CompletionOptions.MaxTokens)
		}
		if body.CompletionOptions.Stream {
			t.Errorf("stream = true, want false")
		}
		if len(body.Messages) != 2 {
			t.Fatalf("messages len = %d, want 2", len(body.Messages))
		}
		if body.Messages[0].Role != "system" || body.Messages[0].Text != "be helpful" {
			t.Errorf("system message = %+v", body.Messages[0])
		}
		if body.Messages[1].Role != "user" || body.Messages[1].Text != "say hello" {
			t.Errorf("user message = %+v", body.Messages[1])
		}

		writeGPTResponse(t, w, "hello")
	}))
	defer srv.Close()

	c := NewGPTClient("test-token", "test-folder", srv.URL)
	got, err := c.Chat(context.Background(), ChatRequest{
		System: "be helpful",
		User:   "say hello",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "hello" {
		t.Errorf("Chat result = %q, want hello", got)
	}
}

func TestChat_JSONMode(t *testing.T) {
	const expected = `{"answer":"42"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeGPTRequest(t, r)
		if body.CompletionOptions.ResponseFormat != "json" {
			t.Errorf("responseFormat = %q, want json", body.CompletionOptions.ResponseFormat)
		}
		writeGPTResponse(t, w, expected)
	}))
	defer srv.Close()

	c := NewGPTClient("test-token", "test-folder", srv.URL)
	got, err := c.Chat(context.Background(), ChatRequest{
		System:     "json please",
		User:       "give me an answer",
		JSONOutput: true,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != expected {
		t.Errorf("Chat result = %q, want %q (raw, unparsed)", got, expected)
	}
}

func TestChat_NoSystemPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := decodeGPTRequest(t, r)
		if len(body.Messages) != 1 {
			t.Fatalf("messages len = %d, want 1 (no system)", len(body.Messages))
		}
		if body.Messages[0].Role != "user" {
			t.Errorf("only message role = %q, want user", body.Messages[0].Role)
		}
		if body.Messages[0].Text != "just a user line" {
			t.Errorf("user text = %q", body.Messages[0].Text)
		}
		writeGPTResponse(t, w, "ok")
	}))
	defer srv.Close()

	c := NewGPTClient("test-token", "test-folder", srv.URL)
	got, err := c.Chat(context.Background(), ChatRequest{
		User: "just a user line",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "ok" {
		t.Errorf("Chat result = %q, want ok", got)
	}
}

func TestChat_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, "too many requests")
	}))
	defer srv.Close()

	c := NewGPTClient("test-token", "test-folder", srv.URL)
	_, err := c.Chat(context.Background(), ChatRequest{User: "hi"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "429") {
		t.Errorf("error = %q, want substring 429", msg)
	}
	if !strings.Contains(msg, "too many requests") {
		t.Errorf("error = %q, want substring of body", msg)
	}
}

func TestChat_EmptyAlternatives(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result":{"alternatives":[]}}`)
	}))
	defer srv.Close()

	c := NewGPTClient("test-token", "test-folder", srv.URL)
	_, err := c.Chat(context.Background(), ChatRequest{User: "hi"})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty completion alternatives") {
		t.Errorf("error = %q, want substring 'empty completion alternatives'", err.Error())
	}
}

func TestChat_FromTokenSource(t *testing.T) {
	keyJSON, _ := makeFixtureKeyJSON(t)

	// Fake IAM endpoint that hands out one specific token.
	var iamCalls int32
	const issuedToken = "iam-token-from-fake"
	iamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&iamCalls, 1)
		resp := map[string]string{
			"iamToken":  issuedToken,
			"expiresAt": time.Now().Add(1 * time.Hour).Format(time.RFC3339Nano),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer iamSrv.Close()
	setIAMEndpointForTest(t, iamSrv.URL)

	// Fake Foundation Models endpoint that asserts the bearer header
	// exactly matches the IAM-issued token.
	var gptCalls int32
	gptSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&gptCalls, 1)
		want := "Bearer " + issuedToken
		if got := r.Header.Get("Authorization"); got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		writeGPTResponse(t, w, "from-source-ok")
	}))
	defer gptSrv.Close()

	ts, err := NewTokenSource(keyJSON)
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}

	c := NewGPTClientFromSource(ts, "test-folder", gptSrv.URL)
	got, err := c.Chat(context.Background(), ChatRequest{User: "ping"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "from-source-ok" {
		t.Errorf("Chat result = %q", got)
	}
	if n := atomic.LoadInt32(&iamCalls); n != 1 {
		t.Errorf("iam calls = %d, want exactly 1", n)
	}
	if n := atomic.LoadInt32(&gptCalls); n != 1 {
		t.Errorf("gpt calls = %d, want exactly 1", n)
	}
}

// TestChat_DefaultEndpointWhenEmpty exercises the small constructor
// branch where endpoint == "" falls back to defaultGPTEndpoint. We
// flip the package var to point at a httptest server so no real
// network call goes out.
func TestChat_DefaultEndpointWhenEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeGPTResponse(t, w, "default-ok")
	}))
	defer srv.Close()

	prev := defaultGPTEndpoint
	defaultGPTEndpoint = srv.URL
	defer func() { defaultGPTEndpoint = prev }()

	c := NewGPTClient("test-token", "test-folder", "")
	got, err := c.Chat(context.Background(), ChatRequest{User: "x"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got != "default-ok" {
		t.Errorf("Chat result = %q", got)
	}
}
