package ai

import (
	"context"
	"sync"
	"testing"
)

// captureLogger collects entries written via Logger.Log for assertions.
// Used by the per-provider adapter tests (yandex_adapter_test.go,
// yandex_ocr_adapter_test.go) — kept here so the helper lives next to the
// Logger type it implements.
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
