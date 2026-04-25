package ai

import (
	"context"
	"encoding/json"
	"time"

	"fujitravel-admin/backend/internal/ai/yandex"
)

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
// prompts verbatim — the `translate` and `programme` paths send only
// dry, non-PII fields (see CLAUDE.md "AI Privacy" section).
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
