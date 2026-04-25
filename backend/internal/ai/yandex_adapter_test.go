package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"fujitravel-admin/backend/internal/ai/yandex"
)

// fakeYandexClient is a recording substitute for *yandex.GPTClient. It
// implements the unexported yandexClient interface that yandexGPTAdapter
// holds, so we can drive the adapter without standing up an HTTP test
// server. One instance is single-shot per test by default; pass a
// `respond` closure to vary behaviour (e.g. error path).
type fakeYandexClient struct {
	mu       sync.Mutex
	calls    int
	requests []yandex.ChatRequest
	respond  func(req yandex.ChatRequest) (string, error)
}

func (f *fakeYandexClient) Chat(_ context.Context, req yandex.ChatRequest) (string, error) {
	f.mu.Lock()
	f.calls++
	f.requests = append(f.requests, req)
	respond := f.respond
	f.mu.Unlock()
	if respond != nil {
		return respond(req)
	}
	return "ok-from-fake", nil
}

// newAdapterWithFake returns an adapter pointing at a recording fake.
// Tests that need to assert on the underlying client invocations keep a
// reference to the fake.
func newAdapterWithFake(fake *fakeYandexClient) *yandexGPTAdapter {
	return &yandexGPTAdapter{client: fake}
}

func TestYandexAdapter_AuditLogOnSuccess(t *testing.T) {
	fake := &fakeYandexClient{
		respond: func(yandex.ChatRequest) (string, error) {
			return "translated-text", nil
		},
	}
	adapter := newAdapterWithFake(fake)

	log := &captureLogger{}
	ctx := WithLogger(context.Background(), log)
	ctx = WithFunctionName(ctx, "test-fn")
	ctx = WithOrgID(ctx, "org-x")
	ctx = WithGenerationID(ctx, "gen-y")
	ctx = WithGroupID(ctx, "group-z")

	out, err := adapter.Chat(ctx, yandex.ChatRequest{
		System:      "sys",
		User:        "hello",
		Temperature: 0,
		MaxTokens:   128,
		JSONOutput:  false,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if out != "translated-text" {
		t.Errorf("returned text = %q, want %q", out, "translated-text")
	}

	if fake.calls != 1 {
		t.Fatalf("fake.calls = %d, want 1", fake.calls)
	}

	if got := len(log.entries); got != 1 {
		t.Fatalf("log.entries len = %d, want 1", got)
	}
	entry := log.last()

	if entry.Provider != "yandex-gpt" {
		t.Errorf("Provider = %q, want yandex-gpt", entry.Provider)
	}
	if entry.FunctionName != "test-fn" {
		t.Errorf("FunctionName = %q, want test-fn", entry.FunctionName)
	}
	if entry.OrgID != "org-x" {
		t.Errorf("OrgID = %q, want org-x", entry.OrgID)
	}
	if entry.GenerationID != "gen-y" {
		t.Errorf("GenerationID = %q, want gen-y", entry.GenerationID)
	}
	if entry.GroupID != "group-z" {
		t.Errorf("GroupID = %q, want group-z", entry.GroupID)
	}
	if entry.Model != "yandexgpt/rc" {
		t.Errorf("Model = %q, want yandexgpt/rc", entry.Model)
	}
	if entry.Status != "success" {
		t.Errorf("Status = %q, want success", entry.Status)
	}
	if entry.ErrorMsg != "" {
		t.Errorf("ErrorMsg = %q, want empty", entry.ErrorMsg)
	}
	if entry.ResponseText != "translated-text" {
		t.Errorf("ResponseText = %q, want translated-text", entry.ResponseText)
	}
	if entry.DurationMs < 0 {
		t.Errorf("DurationMs = %d, want >= 0", entry.DurationMs)
	}
	if entry.StartedAt.IsZero() {
		t.Error("StartedAt is zero — must be set")
	}
	if entry.FinishedAt.IsZero() {
		t.Error("FinishedAt is zero — must be set")
	}
	// Sanity: request snapshot is captured and contains the user payload.
	var snap struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		System   string `json:"system,omitempty"`
		User     string `json:"user,omitempty"`
	}
	if err := json.Unmarshal(entry.RequestJSON, &snap); err != nil {
		t.Fatalf("RequestJSON not valid JSON: %v — raw: %s", err, entry.RequestJSON)
	}
	if snap.Provider != "yandex-gpt" {
		t.Errorf("snapshot.provider = %q, want yandex-gpt", snap.Provider)
	}
	if snap.User != "hello" {
		t.Errorf("snapshot.user = %q, want hello", snap.User)
	}
	if snap.System != "sys" {
		t.Errorf("snapshot.system = %q, want sys", snap.System)
	}
}

func TestYandexAdapter_AuditLogOnError(t *testing.T) {
	wantErr := errors.New("yandex-boom")
	fake := &fakeYandexClient{
		respond: func(yandex.ChatRequest) (string, error) {
			return "", wantErr
		},
	}
	adapter := newAdapterWithFake(fake)

	log := &captureLogger{}
	ctx := WithLogger(context.Background(), log)
	ctx = WithFunctionName(ctx, "test-fn")
	ctx = WithOrgID(ctx, "org-x")
	ctx = WithGenerationID(ctx, "gen-y")

	out, err := adapter.Chat(ctx, yandex.ChatRequest{
		User:      "x",
		MaxTokens: 64,
	})
	if err == nil {
		t.Fatal("Chat: expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		// Adapter currently returns the underlying error verbatim
		// (no wrapping). errors.Is is the canonical check; fall back
		// to substring for resilience if a future commit wraps it.
		if !strings.Contains(err.Error(), "yandex-boom") {
			t.Errorf("error chain missing underlying err: %v", err)
		}
	}
	if out != "" {
		t.Errorf("returned text on error = %q, want empty", out)
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
	if !strings.Contains(entry.ErrorMsg, "yandex-boom") {
		t.Errorf("ErrorMsg = %q, want substring 'yandex-boom'", entry.ErrorMsg)
	}
	if entry.ResponseText != "" {
		t.Errorf("ResponseText = %q on error, want empty", entry.ResponseText)
	}
	if entry.Provider != "yandex-gpt" {
		t.Errorf("Provider = %q on error path, want yandex-gpt", entry.Provider)
	}
}

func TestYandexAdapter_DefaultModel(t *testing.T) {
	// Empty Model on the request → audit row must record the same
	// default the underlying GPTClient would have used. If the
	// adapter ever drifts from yandex.gpt.go's defaultGPTModel, this
	// test fails first.
	fake := &fakeYandexClient{}
	adapter := newAdapterWithFake(fake)

	log := &captureLogger{}
	ctx := WithLogger(context.Background(), log)

	if _, err := adapter.Chat(ctx, yandex.ChatRequest{
		User:  "anything",
		Model: "",
	}); err != nil {
		t.Fatalf("Chat: %v", err)
	}

	entry := log.last()
	if entry.Model != "yandexgpt/rc" {
		t.Errorf("Model = %q, want yandexgpt/rc — adapter default may have drifted from yandex.defaultGPTModel", entry.Model)
	}

	// Snapshot inside RequestJSON must reflect the resolved model too,
	// not the empty string the caller passed.
	var snap struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(entry.RequestJSON, &snap); err != nil {
		t.Fatalf("RequestJSON not valid JSON: %v", err)
	}
	if snap.Model != "yandexgpt/rc" {
		t.Errorf("snapshot.model = %q, want yandexgpt/rc", snap.Model)
	}
}

func TestYandexAdapter_DefaultModel_ExplicitOverride(t *testing.T) {
	// Sanity companion to the default-model test: when the caller
	// supplies Model explicitly, the audit row records that value
	// verbatim (no silent rewrite).
	fake := &fakeYandexClient{}
	adapter := newAdapterWithFake(fake)

	log := &captureLogger{}
	ctx := WithLogger(context.Background(), log)

	if _, err := adapter.Chat(ctx, yandex.ChatRequest{
		User:  "x",
		Model: "yandexgpt-lite/latest",
	}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if got := log.last().Model; got != "yandexgpt-lite/latest" {
		t.Errorf("Model = %q, want yandexgpt-lite/latest", got)
	}
}

func TestYandexAdapter_NoLoggerInCtx_NoPanic(t *testing.T) {
	// Missing Logger in ctx must fall through to NopLogger without
	// panicking — every adapter must tolerate audit-less callers.
	fake := &fakeYandexClient{}
	adapter := newAdapterWithFake(fake)

	if _, err := adapter.Chat(context.Background(), yandex.ChatRequest{
		User: "x",
	}); err != nil {
		t.Fatalf("Chat without logger: %v", err)
	}
	if fake.calls != 1 {
		t.Errorf("fake.calls = %d, want 1", fake.calls)
	}
}

func TestYandexAdapter_ConcurrentCalls(t *testing.T) {
	// Three concurrent Chat calls produce exactly three audit-log rows
	// and exercise the captureLogger / fake client locking. Run with
	// -race to surface data races inside the adapter or the fakes.
	fake := &fakeYandexClient{
		respond: func(req yandex.ChatRequest) (string, error) {
			return "echo:" + req.User, nil
		},
	}
	adapter := newAdapterWithFake(fake)

	log := &captureLogger{}
	ctx := WithLogger(context.Background(), log)
	ctx = WithFunctionName(ctx, "concurrent-test")

	const n = 3
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := adapter.Chat(ctx, yandex.ChatRequest{
				User: string(rune('A' + i)),
			})
			if err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent Chat: %v", err)
	}

	if fake.calls != n {
		t.Errorf("fake.calls = %d, want %d", fake.calls, n)
	}
	if got := len(log.entries); got != n {
		t.Errorf("log.entries len = %d, want %d", got, n)
	}
	for i, e := range log.entries {
		if e.Provider != "yandex-gpt" {
			t.Errorf("entries[%d].Provider = %q, want yandex-gpt", i, e.Provider)
		}
		if e.Status != "success" {
			t.Errorf("entries[%d].Status = %q, want success", i, e.Status)
		}
		if e.FunctionName != "concurrent-test" {
			t.Errorf("entries[%d].FunctionName = %q, want concurrent-test", i, e.FunctionName)
		}
	}
}
