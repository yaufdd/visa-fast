package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

// fakeOCRClient is the recording substitute for the unexported
// yandexOCRClient interface that yandexOCRAdapter wraps. Mirrors the
// shape of fakeYandexClient in yandex_adapter_test.go.
type fakeOCRClient struct {
	mu       sync.Mutex
	calls    int
	respond  func(content []byte, mime string) ([]string, error)
}

func (f *fakeOCRClient) Recognize(_ context.Context, content []byte, mime string) ([]string, error) {
	f.mu.Lock()
	f.calls++
	respond := f.respond
	f.mu.Unlock()
	if respond != nil {
		return respond(content, mime)
	}
	return []string{""}, nil
}

func newOCRAdapterWithFake(fake *fakeOCRClient) *yandexOCRAdapter {
	return &yandexOCRAdapter{client: fake}
}

func TestYandexOCRAdapter_AuditLogOnSuccess(t *testing.T) {
	fake := &fakeOCRClient{
		respond: func(_ []byte, _ string) ([]string, error) {
			return []string{"page-1-text", "page-2-text"}, nil
		},
	}
	adapter := newOCRAdapterWithFake(fake)

	log := &captureLogger{}
	ctx := WithLogger(context.Background(), log)
	ctx = WithFunctionName(ctx, "ticket_parse")
	ctx = WithOrgID(ctx, "org-x")
	ctx = WithGenerationID(ctx, "gen-y")
	ctx = WithGroupID(ctx, "group-z")

	pages, err := adapter.Recognize(ctx, []byte("scan-bytes"), "image/jpeg")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(pages) != 2 || pages[0] != "page-1-text" || pages[1] != "page-2-text" {
		t.Errorf("pages = %v, want [page-1-text page-2-text]", pages)
	}

	if got := len(log.entries); got != 1 {
		t.Fatalf("log.entries len = %d, want 1", got)
	}
	entry := log.last()

	if entry.Provider != "yandex-vision" {
		t.Errorf("Provider = %q, want yandex-vision", entry.Provider)
	}
	if entry.Model != "ocr/v1" {
		t.Errorf("Model = %q, want ocr/v1", entry.Model)
	}
	if entry.FunctionName != "ticket_parse" {
		t.Errorf("FunctionName = %q, want ticket_parse", entry.FunctionName)
	}
	if entry.OrgID != "org-x" {
		t.Errorf("OrgID = %q, want org-x", entry.OrgID)
	}
	if entry.GenerationID != "gen-y" {
		t.Errorf("GenerationID = %q, want gen-y", entry.GenerationID)
	}
	if entry.Status != "success" {
		t.Errorf("Status = %q, want success", entry.Status)
	}
	// ResponseText records the joined pages so the audit viewer mirrors what
	// the GPT extractor will see downstream.
	if !strings.Contains(entry.ResponseText, "page-1-text") || !strings.Contains(entry.ResponseText, "page-2-text") {
		t.Errorf("ResponseText = %q, want both page texts", entry.ResponseText)
	}
	if !strings.Contains(entry.ResponseText, "PAGE BREAK") {
		t.Errorf("ResponseText = %q, want PAGE BREAK separator", entry.ResponseText)
	}

	// Request snapshot must NOT contain the raw scan bytes — they can
	// carry PII; only metadata is logged.
	var snap struct {
		Provider string `json:"provider"`
		MimeType string `json:"mime_type"`
		ByteLen  int    `json:"byte_len"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal(entry.RequestJSON, &snap); err != nil {
		t.Fatalf("RequestJSON not valid JSON: %v — raw: %s", err, entry.RequestJSON)
	}
	if snap.Provider != "yandex-vision" {
		t.Errorf("snapshot.provider = %q, want yandex-vision", snap.Provider)
	}
	if snap.MimeType != "image/jpeg" {
		t.Errorf("snapshot.mime_type = %q, want image/jpeg", snap.MimeType)
	}
	if snap.ByteLen != len("scan-bytes") {
		t.Errorf("snapshot.byte_len = %d, want %d", snap.ByteLen, len("scan-bytes"))
	}
	if strings.Contains(snap.Content, "scan-bytes") {
		t.Errorf("snapshot.content leaked raw bytes: %q", snap.Content)
	}
	if !strings.Contains(snap.Content, "redacted") {
		t.Errorf("snapshot.content = %q, want substring 'redacted'", snap.Content)
	}
}

func TestYandexOCRAdapter_AuditLogOnError(t *testing.T) {
	wantErr := errors.New("vision-boom")
	fake := &fakeOCRClient{
		respond: func(_ []byte, _ string) ([]string, error) {
			return nil, wantErr
		},
	}
	adapter := newOCRAdapterWithFake(fake)

	log := &captureLogger{}
	ctx := WithLogger(context.Background(), log)
	ctx = WithFunctionName(ctx, "ticket_parse")
	ctx = WithOrgID(ctx, "org-x")
	ctx = WithGenerationID(ctx, "gen-y")

	pages, err := adapter.Recognize(ctx, []byte("x"), "application/pdf")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		// Adapter currently returns the underlying error verbatim. errors.Is
		// is the canonical check; fall back to substring for resilience if a
		// future commit wraps it.
		if !strings.Contains(err.Error(), "vision-boom") {
			t.Errorf("error chain missing underlying err: %v", err)
		}
	}
	if pages != nil {
		t.Errorf("pages on error = %v, want nil", pages)
	}

	if got := len(log.entries); got != 1 {
		t.Fatalf("log.entries len = %d, want 1", got)
	}
	entry := log.last()
	if entry.Status != "error" {
		t.Errorf("Status = %q, want error", entry.Status)
	}
	if entry.ErrorMsg == "" {
		t.Error("ErrorMsg empty on failed call — audit row must capture cause")
	}
	if entry.Provider != "yandex-vision" {
		t.Errorf("Provider = %q on error path, want yandex-vision", entry.Provider)
	}
}

func TestYandexOCRAdapter_NoLoggerInCtx_NoPanic(t *testing.T) {
	fake := &fakeOCRClient{}
	adapter := newOCRAdapterWithFake(fake)

	if _, err := adapter.Recognize(context.Background(), []byte("x"), "image/jpeg"); err != nil {
		t.Fatalf("Recognize without logger: %v", err)
	}
	if fake.calls != 1 {
		t.Errorf("fake.calls = %d, want 1", fake.calls)
	}
}
