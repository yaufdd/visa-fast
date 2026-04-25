package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"fujitravel-admin/backend/internal/ai/yandex"
)

// fakeOCR and recordingTranslator are defined in ticket_parser_test.go in
// the same package — re-used here so the two suites share one fake shape.

func TestParseVoucherScan_HappyPath(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return []string{"OCR PAGE TEXT — Hilton Tokyo Bay, check-in 25 Apr 2026, check-out 27 Apr 2026"}, nil
		},
	}
	tr := &recordingTranslator{
		respond: func(_ yandex.ChatRequest) (string, error) {
			return `[
				{"name":"Hilton Tokyo Bay","city":"TOKYO","address":"1-8 Maihama, Urayasu, Chiba","phone":"+81 47 355 5000","check_in":"25.04.2026","check_out":"27.04.2026"},
				{"name":"Dusit Thani Kyoto","city":"KYOTO","address":"","phone":"","check_in":"27.04.2026","check_out":"29.04.2026"}
			]`, nil
		},
	}

	out, err := ParseVoucherScan(context.Background(), ocr, tr, []byte("scan-bytes"), "application/pdf")
	if err != nil {
		t.Fatalf("ParseVoucherScan: %v", err)
	}

	if len(out) != 2 {
		t.Fatalf("got %d hotels, want 2", len(out))
	}
	if out[0].Name != "Hilton Tokyo Bay" {
		t.Errorf("hotels[0].Name = %q, want Hilton Tokyo Bay", out[0].Name)
	}
	if out[0].City != "TOKYO" {
		t.Errorf("hotels[0].City = %q, want TOKYO", out[0].City)
	}
	if out[0].CheckIn != "25.04.2026" {
		t.Errorf("hotels[0].CheckIn = %q, want 25.04.2026", out[0].CheckIn)
	}
	if out[1].Name != "Dusit Thani Kyoto" {
		t.Errorf("hotels[1].Name = %q, want Dusit Thani Kyoto", out[1].Name)
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
	if ocr.requests[0].mime != "application/pdf" {
		t.Errorf("ocr mime = %q, want application/pdf", ocr.requests[0].mime)
	}
}

func TestParseVoucherScan_OCRError(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return nil, errors.New("vision-503")
		},
	}
	tr := &recordingTranslator{}

	_, err := ParseVoucherScan(context.Background(), ocr, tr, []byte("x"), "image/jpeg")
	if err == nil {
		t.Fatal("expected error on OCR failure, got nil")
	}
	if !strings.Contains(err.Error(), "voucher ocr") {
		t.Errorf("error = %q, want substring 'voucher ocr'", err)
	}
	if !strings.Contains(err.Error(), "vision-503") {
		t.Errorf("error = %q, want substring 'vision-503'", err)
	}
	// GPT must NOT be called when OCR fails — the pipeline is short-circuit.
	if tr.calls != 0 {
		t.Errorf("tr.calls on OCR failure = %d, want 0", tr.calls)
	}
}

func TestParseVoucherScan_GPTError(t *testing.T) {
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

	_, err := ParseVoucherScan(context.Background(), ocr, tr, []byte("x"), "image/jpeg")
	if err == nil {
		t.Fatal("expected error on GPT failure, got nil")
	}
	if !strings.Contains(err.Error(), "voucher gpt") {
		t.Errorf("error = %q, want substring 'voucher gpt'", err)
	}
	if !strings.Contains(err.Error(), "gpt-429") {
		t.Errorf("error = %q, want substring 'gpt-429'", err)
	}
}

func TestParseVoucherScan_InvalidJSON(t *testing.T) {
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

	_, err := ParseVoucherScan(context.Background(), ocr, tr, []byte("x"), "image/jpeg")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %q, want substring 'decode'", err)
	}
}

func TestParseVoucherScan_PromptIncludesPageBreak(t *testing.T) {
	ocr := &fakeOCR{
		respond: func(_ []byte, _ string) ([]string, error) {
			return []string{"PAGE-1-TEXT", "PAGE-2-TEXT", "PAGE-3-TEXT"}, nil
		},
	}
	tr := &recordingTranslator{
		respond: func(_ yandex.ChatRequest) (string, error) {
			return `[]`, nil
		},
	}

	if _, err := ParseVoucherScan(context.Background(), ocr, tr, []byte("x"), "application/pdf"); err != nil {
		t.Fatalf("ParseVoucherScan: %v", err)
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
	// keep so the voucher parser keeps working as more variants land.
	// Substring asserts are deliberately loose — we want to catch deletions,
	// not nitpick wording.
	if !strings.Contains(req.System, "DD.MM.YYYY") {
		t.Errorf("System prompt missing DD.MM.YYYY date format rule")
	}
	if !strings.Contains(req.System, "CITY CAPS") {
		t.Errorf("System prompt missing CITY CAPS city-format rule")
	}
	if !strings.Contains(req.System, "+81") {
		t.Errorf("System prompt missing Japanese phone-format hint")
	}
	if !strings.Contains(req.System, "JSON array") {
		t.Errorf("System prompt missing JSON array output instruction")
	}
	if !req.JSONOutput {
		t.Errorf("JSONOutput = false, want true — extractor must use json responseFormat")
	}
	if req.Temperature != 0 {
		t.Errorf("Temperature = %v, want 0 — extractor must be deterministic", req.Temperature)
	}
}

func TestParseVoucherScan_NilClients(t *testing.T) {
	// Defensive: the handler-side wiring should always pass real clients,
	// but a nil interface should fail fast with a clear error rather than
	// a nil-pointer panic.
	_, err := ParseVoucherScan(context.Background(), nil, &recordingTranslator{}, []byte("x"), "image/jpeg")
	if err == nil || !strings.Contains(err.Error(), "ocr") {
		t.Errorf("nil-ocr error = %v, want substring 'ocr'", err)
	}

	_, err = ParseVoucherScan(context.Background(), &fakeOCR{}, nil, []byte("x"), "image/jpeg")
	if err == nil || !strings.Contains(err.Error(), "translator") {
		t.Errorf("nil-translator error = %v, want substring 'translator'", err)
	}
}
