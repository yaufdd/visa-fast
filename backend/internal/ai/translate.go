package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"fujitravel-admin/backend/internal/ai/yandex"
)

const translateSystemPrompt = `You are a Russian → English translator for Japanese visa documents. Translate each string to natural English. For proper names (companies, people, street names), transliterate using the standard Russian → Latin system. For descriptive words (job titles, address parts), translate them fully. Return ONLY a JSON array of translations, same length and order as the input array. No markdown fences, no prose.

Examples:
- "Директор по развитию" → "Director of Development"
- "ООО Ромашка" → "LLC Romashka"
- "Москва, ул. Ленина 5, кв. 12" → "Moscow, Lenin St. 5, Apt. 12"
- "ИП Иванов Петр" → "IE Ivanov Petr"
- "ОУФМС России по г. Москве" → "Federal Migration Service in Moscow"
- "МВД 77810" → "MVD 77810"
- "СССР" → "USSR"
- "январь 2020" → "January 2020"`

// Translator abstracts a YandexGPT-shaped chat call. Implemented by
// *yandex.GPTClient (production) and by the test fakes in
// translate_test.go. Keeping this interface inside the ai package lets
// callers (api/generate.go) depend on a small, mockable surface instead
// of pulling the full yandex package into their dependency graph.
type Translator interface {
	Chat(ctx context.Context, req yandex.ChatRequest) (string, error)
}

// TranslateStrings sends a batch of Russian strings to YandexGPT for
// English translation. Nil or empty input → nil output, no API call.
// The result slice is exactly the same length as the input.
//
// The translator parameter is the only seam that touches Yandex —
// production code passes a *yandexGPTAdapter (which wraps a real
// *yandex.GPTClient and writes an audit-log row per call); tests pass
// a small struct returning a canned response.
func TranslateStrings(ctx context.Context, t Translator, src []string) ([]string, error) {
	if len(src) == 0 {
		return nil, nil
	}
	if t == nil {
		return nil, fmt.Errorf("translate: nil translator")
	}
	ctx = WithFunctionName(ctx, "translate")
	userBody, err := json.Marshal(map[string]any{"strings": src})
	if err != nil {
		return nil, fmt.Errorf("marshal translate input: %w", err)
	}

	raw, err := t.Chat(ctx, yandex.ChatRequest{
		System:      translateSystemPrompt,
		User:        string(userBody),
		Temperature: 0,
		MaxTokens:   2048,
		JSONOutput:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("translate yandex call: %w", err)
	}

	js := extractJSON(raw)
	var out []string
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return nil, fmt.Errorf("translate decode array: %w — raw: %s", err, raw)
	}
	if len(out) != len(src) {
		return nil, fmt.Errorf("translate length mismatch: got %d, want %d — raw: %s", len(out), len(src), raw)
	}
	return out, nil
}

// yandexClient is the small surface yandexGPTAdapter actually needs from
// a Yandex GPT client. *yandex.GPTClient satisfies it in production; the
// adapter tests pass a recording fake. Kept unexported because it is an
// implementation detail of the audit-log seam, not part of the package's
// public API.
type yandexClient interface {
	Chat(ctx context.Context, req yandex.ChatRequest) (string, error)
}

// yandexGPTAdapter wraps a yandexClient and writes one ai_call_logs
// audit row per Chat call. It is the production implementation of
// Translator: see NewYandexAdapter for construction. We keep the audit
// instrumentation here (not in the yandex package) so the yandex client
// stays dependency-free and reusable; the audit shape lives next to the
// other AI seam (callClaude in client.go) for symmetry.
type yandexGPTAdapter struct {
	client yandexClient
}

// NewYandexAdapter wires a *yandex.GPTClient through ai_call_logs as the
// audit-log seam for all YandexGPT calls. Audit logging is performed by
// reading the Logger that the caller installs in ctx via WithLogger; if
// no logger is installed the NopLogger silently swallows the row,
// matching callClaude's behaviour.
//
// PII CONTRACT (152-ФЗ): the audit row records the FULL request body
// including ChatRequest.User and ChatRequest.System. Callers MUST NOT
// pass any of the PII fields listed in CLAUDE.md "AI Privacy" section
// (full names, passport numbers, dates of birth, home/registration
// addresses, phone numbers) in either field. Translate (Task 1.B1)
// satisfies this by sending only de-duplicated dry fields. Programme
// (Task 1.B2) satisfies this by stripping tourist names and passport
// data before composing the prompt. New call sites must verify their
// payloads against this contract before adding callers.
func NewYandexAdapter(client *yandex.GPTClient) Translator {
	return &yandexGPTAdapter{client: client}
}

// Chat performs one Yandex completion and emits one audit row.
//
// Mirrors the contract of callClaude: every code path (success or
// error) writes exactly one CallLog entry via the Logger installed in
// ctx. The deferred closure assigns the final fields just before the
// row is persisted, so an early return cannot skip the audit.
func (a *yandexGPTAdapter) Chat(ctx context.Context, req yandex.ChatRequest) (string, error) {
	started := time.Now()
	model := req.Model
	if model == "" {
		// Mirror the GPTClient default so audit rows show the actual
		// model that served the call rather than an empty string.
		model = "yandexgpt/rc"
	}

	requestSnapshot := redactYandexRequestForLog(req, model)

	logEntry := CallLog{
		OrgID:        OrgIDFromContext(ctx),
		GroupID:      GroupIDFromContext(ctx),
		SubgroupID:   SubgroupIDFromContext(ctx),
		GenerationID: GenerationIDFromContext(ctx),
		FunctionName: FunctionNameFromContext(ctx),
		Provider:     "yandex-gpt",
		Model:        model,
		RequestJSON:  requestSnapshot,
		StartedAt:    started,
		Status:       "error", // overridden on success
	}
	logger := LoggerFromContext(ctx)
	defer func() {
		logEntry.FinishedAt = time.Now()
		logEntry.DurationMs = int(logEntry.FinishedAt.Sub(started) / time.Millisecond)
		_ = logger.Log(ctx, logEntry)
	}()

	text, err := a.client.Chat(ctx, req)
	if err != nil {
		logEntry.ErrorMsg = err.Error()
		return "", err
	}
	logEntry.Status = "success"
	logEntry.ResponseText = text
	return text, nil
}

// redactYandexRequestForLog returns a JSON snapshot of the chat request
// suitable for the audit log. Yandex GPT calls in this codebase do not
// carry image/PDF bytes (those go via the Vision OCR seam, not Chat),
// so we can serialize the request as-is. We DO record system/user
// prompts verbatim — the `translate` path sends only de-duplicated
// dry fields, never PII (see CLAUDE.md "AI Privacy" section).
func redactYandexRequestForLog(req yandex.ChatRequest, model string) json.RawMessage {
	snapshot := struct {
		Provider    string  `json:"provider"`
		Model       string  `json:"model"`
		System      string  `json:"system,omitempty"`
		User        string  `json:"user,omitempty"`
		Temperature float64 `json:"temperature"`
		MaxTokens   int     `json:"max_tokens"`
		JSONOutput  bool    `json:"json_output"`
	}{
		Provider:    "yandex-gpt",
		Model:       model,
		System:      req.System,
		User:        req.User,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		JSONOutput:  req.JSONOutput,
	}
	b, err := json.Marshal(snapshot)
	if err != nil {
		return json.RawMessage(`{"error":"marshal_failed"}`)
	}
	return b
}
