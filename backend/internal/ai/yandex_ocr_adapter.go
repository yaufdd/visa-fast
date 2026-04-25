package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// OCRRecognizer is the small surface our scan parsers need from a Yandex
// Vision OCR client. *yandex.OCRClient satisfies it indirectly via
// yandexOCRAdapter; tests pass a fake. Kept exported because callers in
// other ai-package files (ticket_parser, future passport_parser) reference
// it by name.
type OCRRecognizer interface {
	Recognize(ctx context.Context, content []byte, mime string) ([]string, error)
}

// yandexOCRClient is the underlying contract that yandexOCRAdapter wraps.
// *yandex.OCRClient satisfies it in production; tests pass a recording
// fake. Unexported because it is an implementation detail of the audit-
// log seam, mirroring yandexClient in yandex_adapter.go.
type yandexOCRClient interface {
	Recognize(ctx context.Context, content []byte, mime string) ([]string, error)
}

// yandexOCRAdapter wraps a yandexOCRClient and writes one ai_call_logs
// audit row per Recognize call. Symmetrical to yandexGPTAdapter but for
// Vision OCR calls. See NewYandexOCRAdapter for construction.
type yandexOCRAdapter struct {
	client yandexOCRClient
}

// NewYandexOCRAdapter wires a *yandex.OCRClient (or any yandexOCRClient
// implementation) through ai_call_logs as the audit-log seam for all
// Yandex Vision calls. Audit logging is performed by reading the Logger
// the caller installs in ctx via WithLogger; if no logger is installed
// the NopLogger silently swallows the row, matching the behaviour of
// yandexGPTAdapter.
//
// PII CONTRACT (152-ФЗ): the audit row records the input mime type, byte
// length, and the recognized text per page. The input bytes themselves
// are NOT logged — they are passport/ticket/voucher scans which can
// contain PII; logging them would defeat the purpose of the on-prem
// audit trail. Recognized text IS logged so a manager can later see what
// the OCR pass extracted (this matches the response_text behaviour of
// the GPT adapter).
func NewYandexOCRAdapter(client yandexOCRClient) OCRRecognizer {
	return &yandexOCRAdapter{client: client}
}

// Recognize performs one Vision OCR call and emits one audit row.
//
// Mirrors the contract of yandexGPTAdapter.Chat: every code path (success
// or error) writes exactly one CallLog entry via the Logger installed in
// ctx. The deferred closure assigns the final fields just before the row
// is persisted, so an early return cannot skip the audit.
func (a *yandexOCRAdapter) Recognize(ctx context.Context, content []byte, mime string) ([]string, error) {
	started := time.Now()

	requestSnapshot := redactOCRRequestForLog(content, mime)

	logEntry := CallLog{
		OrgID:        OrgIDFromContext(ctx),
		GroupID:      GroupIDFromContext(ctx),
		SubgroupID:   SubgroupIDFromContext(ctx),
		GenerationID: GenerationIDFromContext(ctx),
		FunctionName: FunctionNameFromContext(ctx),
		Provider:     "yandex-vision",
		Model:        "ocr/v1",
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

	pages, err := a.client.Recognize(ctx, content, mime)
	if err != nil {
		logEntry.ErrorMsg = err.Error()
		return nil, err
	}
	logEntry.Status = "success"
	// Join pages with the same marker the parsers will use downstream so the
	// audit row reflects the exact text the GPT extractor will receive.
	logEntry.ResponseText = strings.Join(pages, "\n\n--- PAGE BREAK ---\n\n")
	return pages, nil
}

// redactOCRRequestForLog returns a JSON snapshot of the OCR request
// suitable for the audit log. We deliberately do NOT log the raw content
// bytes — they are scans that can contain PII. We log only the mime type
// and byte length so a manager can trace which call the row belongs to.
func redactOCRRequestForLog(content []byte, mime string) json.RawMessage {
	snapshot := struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		MimeType string `json:"mime_type"`
		ByteLen  int    `json:"byte_len"`
		Content  string `json:"content"`
	}{
		Provider: "yandex-vision",
		Model:    "ocr/v1",
		MimeType: mime,
		ByteLen:  len(content),
		Content:  fmt.Sprintf("[%s redacted, %d bytes]", mime, len(content)),
	}
	b, err := json.Marshal(snapshot)
	if err != nil {
		return json.RawMessage(`{"error":"marshal_failed"}`)
	}
	return b
}
