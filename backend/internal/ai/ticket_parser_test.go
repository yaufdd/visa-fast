package ai

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"fujitravel-admin/backend/internal/ai/yandex"
)

// fakeOCR is a recording substitute for the OCR seam used by
// ParseTicketScan. Implements the OCRRecognizer interface so tests can
// drive the parser without standing up an HTTP test server.
type fakeOCR struct {
	mu       sync.Mutex
	calls    int
	requests []ocrCall
	respond  func(content []byte, mime string) ([]string, error)
}

type ocrCall struct {
	content []byte
	mime    string
}

func (f *fakeOCR) Recognize(_ context.Context, content []byte, mime string) ([]string, error) {
	f.mu.Lock()
	f.calls++
	f.requests = append(f.requests, ocrCall{content: content, mime: mime})
	respond := f.respond
	f.mu.Unlock()
	if respond != nil {
		return respond(content, mime)
	}
	return []string{""}, nil
}

// recordingTranslator is the GPT-side seam: implements the Translator interface
// so tests can drive ParseTicketScan without a real Yandex client. Re-uses
// the recording shape from yandex_adapter_test.go's fakeYandexClient but
// is named separately to avoid coupling the two suites.
type recordingTranslator struct {
	mu       sync.Mutex
	calls    int
	requests []yandex.ChatRequest
	respond  func(req yandex.ChatRequest) (string, error)
}

func (f *recordingTranslator) Chat(_ context.Context, req yandex.ChatRequest) (string, error) {
	f.mu.Lock()
	f.calls++
	f.requests = append(f.requests, req)
	respond := f.respond
	f.mu.Unlock()
	if respond != nil {
		return respond(req)
	}
	return "{}", nil
}

func TestParseTicketScan_HappyPath(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return []string{"OCR PAGE TEXT — SU262 SVO→NRT 2025-04-22"}, nil
		},
	}
	tr := &recordingTranslator{
		respond: func(_ yandex.ChatRequest) (string, error) {
			return `{
				"arrival":   {"flight_number":"SU262","date":"22.04.2025","time":"09:30","airport":"Narita International Airport"},
				"departure": {"flight_number":"SU263","date":"29.04.2025","time":"12:45","airport":"Narita International Airport"}
			}`, nil
		},
	}

	out, err := ParseTicketScan(context.Background(), ocr, tr, []byte("scan-bytes"), "image/jpeg")
	if err != nil {
		t.Fatalf("ParseTicketScan: %v", err)
	}

	if out.Arrival.FlightNumber != "SU262" {
		t.Errorf("Arrival.FlightNumber = %q, want SU262", out.Arrival.FlightNumber)
	}
	if out.Arrival.Date != "22.04.2025" {
		t.Errorf("Arrival.Date = %q, want 22.04.2025", out.Arrival.Date)
	}
	if out.Arrival.Airport != "Narita International Airport" {
		t.Errorf("Arrival.Airport = %q, want Narita International Airport", out.Arrival.Airport)
	}
	if out.Departure.FlightNumber != "SU263" {
		t.Errorf("Departure.FlightNumber = %q, want SU263", out.Departure.FlightNumber)
	}

	if ocr.calls != 1 {
		t.Errorf("ocr.calls = %d, want 1", ocr.calls)
	}
	if tr.calls != 1 {
		t.Errorf("tr.calls = %d, want 1", tr.calls)
	}
	// OCR receives the raw scan bytes + mime verbatim — Yandex residency means
	// no local redaction step; the parser must not mutate the upload payload.
	if string(ocr.requests[0].content) != "scan-bytes" {
		t.Errorf("ocr content = %q, want scan-bytes", ocr.requests[0].content)
	}
	if ocr.requests[0].mime != "image/jpeg" {
		t.Errorf("ocr mime = %q, want image/jpeg", ocr.requests[0].mime)
	}
}

func TestParseTicketScan_OCRError(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return nil, errors.New("vision-503")
		},
	}
	tr := &recordingTranslator{}

	_, err := ParseTicketScan(context.Background(), ocr, tr, []byte("x"), "image/jpeg")
	if err == nil {
		t.Fatal("expected error on OCR failure, got nil")
	}
	if !strings.Contains(err.Error(), "ticket ocr") {
		t.Errorf("error = %q, want substring 'ticket ocr'", err)
	}
	if !strings.Contains(err.Error(), "vision-503") {
		t.Errorf("error = %q, want substring 'vision-503'", err)
	}
	// GPT must NOT be called when OCR fails — the pipeline is short-circuit.
	if tr.calls != 0 {
		t.Errorf("tr.calls on OCR failure = %d, want 0", tr.calls)
	}
}

func TestParseTicketScan_GPTError(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return []string{"page text"}, nil
		},
	}
	tr := &recordingTranslator{
		respond: func(_ yandex.ChatRequest) (string, error) {
			return "", errors.New("gpt-429")
		},
	}

	_, err := ParseTicketScan(context.Background(), ocr, tr, []byte("x"), "image/jpeg")
	if err == nil {
		t.Fatal("expected error on GPT failure, got nil")
	}
	if !strings.Contains(err.Error(), "ticket gpt") {
		t.Errorf("error = %q, want substring 'ticket gpt'", err)
	}
	if !strings.Contains(err.Error(), "gpt-429") {
		t.Errorf("error = %q, want substring 'gpt-429'", err)
	}
}

func TestParseTicketScan_InvalidJSON(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return []string{"page text"}, nil
		},
	}
	tr := &recordingTranslator{
		respond: func(_ yandex.ChatRequest) (string, error) {
			return "this is not json at all", nil
		},
	}

	_, err := ParseTicketScan(context.Background(), ocr, tr, []byte("x"), "image/jpeg")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %q, want substring 'decode'", err)
	}
}

func TestParseTicketScan_PromptIncludesPageBreak(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return []string{"PAGE-1-TEXT", "PAGE-2-TEXT", "PAGE-3-TEXT"}, nil
		},
	}
	tr := &recordingTranslator{
		respond: func(_ yandex.ChatRequest) (string, error) {
			return `{"arrival":{"flight_number":"","date":"","time":"","airport":""},"departure":{"flight_number":"","date":"","time":"","airport":""}}`, nil
		},
	}

	if _, err := ParseTicketScan(context.Background(), ocr, tr, []byte("x"), "application/pdf"); err != nil {
		t.Fatalf("ParseTicketScan: %v", err)
	}

	if tr.calls != 1 {
		t.Fatalf("tr.calls = %d, want 1", tr.calls)
	}
	req := tr.requests[0]

	// Multi-page OCR output is joined with the documented page-break marker
	// so the GPT extractor knows where one page ends and the next begins.
	if !strings.Contains(req.User, "PAGE BREAK") {
		t.Errorf("User payload missing PAGE BREAK marker: %q", req.User)
	}
	if !strings.Contains(req.User, "PAGE-1-TEXT") || !strings.Contains(req.User, "PAGE-3-TEXT") {
		t.Errorf("User payload missing page text: %q", req.User)
	}
	// And the system prompt explicitly tells the model what the marker means
	// — the contract is self-describing on both sides.
	if !strings.Contains(req.System, "PAGE BREAK") {
		t.Errorf("System prompt missing PAGE BREAK reference")
	}

	// Spot-check a handful of prompt-rules that the production prompt MUST
	// keep so the scan parser keeps working as more variants land. Substring
	// asserts are deliberately loose — we want to catch deletions, not
	// nitpick wording.
	if !strings.Contains(req.System, "DD.MM.YYYY") {
		t.Errorf("System prompt missing DD.MM.YYYY date format rule")
	}
	if !strings.Contains(req.System, "IATA") {
		t.Errorf("System prompt missing IATA airport-code rule")
	}
	if !strings.Contains(req.System, "Narita International Airport") {
		t.Errorf("System prompt missing canonical Japanese airport list")
	}
	if !strings.Contains(req.System, "Latin") {
		t.Errorf("System prompt missing Latin flight-number rule")
	}
	if !req.JSONOutput {
		t.Errorf("JSONOutput = false, want true — extractor must use json responseFormat")
	}
	if req.Temperature != 0 {
		t.Errorf("Temperature = %v, want 0 — extractor must be deterministic", req.Temperature)
	}
}

func TestParseTicketScan_NilClients(t *testing.T) {
	// Defensive: the handler-side wiring should always pass real clients,
	// but a nil interface should fail fast with a clear error rather than
	// a nil-pointer panic.
	_, err := ParseTicketScan(context.Background(), nil, &recordingTranslator{}, []byte("x"), "image/jpeg")
	if err == nil || !strings.Contains(err.Error(), "ocr") {
		t.Errorf("nil-ocr error = %v, want substring 'ocr'", err)
	}

	_, err = ParseTicketScan(context.Background(), &fakeOCR{}, nil, []byte("x"), "image/jpeg")
	if err == nil || !strings.Contains(err.Error(), "translator") {
		t.Errorf("nil-translator error = %v, want substring 'translator'", err)
	}
}
