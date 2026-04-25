package ai

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

// NewGenerationID returns a fresh UUID-v4 string. Use at the start of every
// /generate or /finalize run to correlate all nested provider calls into one
// audit trail.
func NewGenerationID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// CallLog is the shape of one audit-log entry for a single AI provider call.
// Written via Logger.Log inside the per-provider HTTP seam (yandexGPTAdapter,
// yandexOCRAdapter) — every outbound AI request is observed so nothing can
// leak out unrecorded.
type CallLog struct {
	OrgID        string          // org_id of the calling tenant; "" if request is not org-scoped
	GroupID      string          // group_id this generation belongs to; "" if not applicable
	SubgroupID   string          // subgroup_id if the call targets one subgroup
	GenerationID string          // UUID grouping every call inside one /generate or /finalize run
	FunctionName string          // "translate" | "programme" | "ticket_parser" | "voucher_parser"
	// Provider names the AI vendor that served this call. Required on every
	// row. Allowed values mirror the DB CHECK constraint
	// (migration 000019): "anthropic" (legacy rows), "yandex-gpt",
	// "yandex-vision". Validation is delegated to the DB so future
	// providers can be added by migration alone — the Go code records the
	// value verbatim.
	Provider     string
	Model        string          // model id (provider-specific, e.g. yandexgpt/latest)
	RequestJSON  json.RawMessage // request marshalled, with image bytes redacted
	ResponseText string          // raw text reply from the provider (empty on error)
	Status       string          // "success" | "error"
	ErrorMsg     string          // non-empty on failure
	InputTokens  int             // future: parse from provider response
	OutputTokens int
	StartedAt    time.Time
	FinishedAt   time.Time
	DurationMs   int
}

// Logger accepts and persists CallLog entries. Implementations must be
// best-effort — they must NEVER return a non-nil error in a way that would
// abort the AI call. The log is observational, not critical path.
type Logger interface {
	Log(ctx context.Context, entry CallLog) error
}

// NopLogger silently discards log entries. Used as the default when no
// Logger is installed in ctx (e.g. tests, one-off utilities).
type NopLogger struct{}

func (NopLogger) Log(_ context.Context, _ CallLog) error { return nil }

type ctxKey int

const (
	ctxKeyLogger ctxKey = iota
	ctxKeyGenerationID
	ctxKeyOrgID
	ctxKeyGroupID
	ctxKeySubgroupID
	ctxKeyFunctionName
)

// WithLogger returns a ctx whose AI provider seams will write audit rows
// via `l`.
func WithLogger(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, ctxKeyLogger, l)
}

// LoggerFromContext returns the installed Logger, or a NopLogger when none.
func LoggerFromContext(ctx context.Context) Logger {
	if l, ok := ctx.Value(ctxKeyLogger).(Logger); ok && l != nil {
		return l
	}
	return NopLogger{}
}

// WithGenerationID tags all nested provider rows with the same generation_id.
// Callers should set one UUID per /generate or /finalize run so a manager can
// inspect "everything that left the server for that generation" in one query.
func WithGenerationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyGenerationID, id)
}
func GenerationIDFromContext(ctx context.Context) string { return ctxString(ctx, ctxKeyGenerationID) }

// WithOrgID attaches the tenant org for scoping. Does NOT replace the
// middleware.OrgID helper — this is a parallel value used only for audit rows.
func WithOrgID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyOrgID, id)
}
func OrgIDFromContext(ctx context.Context) string { return ctxString(ctx, ctxKeyOrgID) }

// WithGroupID / WithSubgroupID — optional context for UI filtering.
func WithGroupID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyGroupID, id)
}
func GroupIDFromContext(ctx context.Context) string { return ctxString(ctx, ctxKeyGroupID) }

func WithSubgroupID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeySubgroupID, id)
}
func SubgroupIDFromContext(ctx context.Context) string { return ctxString(ctx, ctxKeySubgroupID) }

// WithFunctionName tags subsequent provider rows with a stable name for the
// calling high-level function ("translate", "programme", etc.). Each
// high-level function is expected to set this at its own entry so the audit
// row carries the call-site label.
func WithFunctionName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, ctxKeyFunctionName, name)
}
func FunctionNameFromContext(ctx context.Context) string {
	return ctxString(ctx, ctxKeyFunctionName)
}

func ctxString(ctx context.Context, key ctxKey) string {
	if v, ok := ctx.Value(key).(string); ok {
		return v
	}
	return ""
}
