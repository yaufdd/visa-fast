package yandex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// decodeOCRRequest reads and JSON-decodes the request body sent to the
// fake Vision OCR server.
func decodeOCRRequest(t *testing.T, r *http.Request) ocrRequestBody {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var body ocrRequestBody
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return body
}

// writeOCRResponse encodes a single happy-path response with the given
// fullText.
func writeOCRResponse(t *testing.T, w http.ResponseWriter, fullText string) {
	t.Helper()
	resp := map[string]any{
		"result": map[string]any{
			"textAnnotation": map[string]any{
				"fullText": fullText,
				"blocks":   []any{},
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

// fixturePDF reads a committed test PDF from testdata/.
func fixturePDF(t *testing.T, name string) []byte {
	t.Helper()
	bts, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return bts
}

func TestOCR_SinglePageImage(t *testing.T) {
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
		if r.URL.Path != "/ocr/v1/recognizeText" {
			t.Errorf("path = %q, want /ocr/v1/recognizeText", r.URL.Path)
		}

		body := decodeOCRRequest(t, r)
		if body.MimeType != "image/jpeg" {
			t.Errorf("mimeType = %q, want image/jpeg", body.MimeType)
		}
		if body.Model != "page" {
			t.Errorf("model = %q, want page", body.Model)
		}
		if len(body.LanguageCodes) != 1 || body.LanguageCodes[0] != "*" {
			t.Errorf("languageCodes = %v, want [\"*\"]", body.LanguageCodes)
		}
		if body.Content == "" {
			t.Errorf("content = empty, want base64-encoded image")
		}
		// Decode the content header to be sure it's valid base64.
		dec, err := base64.StdEncoding.DecodeString(body.Content)
		if err != nil {
			t.Errorf("content not valid base64: %v", err)
		}
		if string(dec) != "fake-jpeg-bytes" {
			t.Errorf("decoded content = %q, want fake-jpeg-bytes", string(dec))
		}

		writeOCRResponse(t, w, "hello world")
	}))
	defer srv.Close()

	c := NewOCRClient("test-token", "test-folder", srv.URL)
	pages, err := c.Recognize(context.Background(), []byte("fake-jpeg-bytes"), "image/jpeg")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("len(pages) = %d, want 1", len(pages))
	}
	if pages[0] != "hello world" {
		t.Errorf("pages[0] = %q, want hello world", pages[0])
	}
}

func TestOCR_MultiPagePDF(t *testing.T) {
	pdfBytes := fixturePDF(t, "two-page.pdf")

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		body := decodeOCRRequest(t, r)
		if body.MimeType != "application/pdf" {
			t.Errorf("call %d mimeType = %q, want application/pdf", n, body.MimeType)
		}
		// The split PDF for each page should also be valid base64.
		if _, err := base64.StdEncoding.DecodeString(body.Content); err != nil {
			t.Errorf("call %d content not valid base64: %v", n, err)
		}
		writeOCRResponse(t, w, fmt.Sprintf("page%d", n))
	}))
	defer srv.Close()

	c := NewOCRClient("test-token", "test-folder", srv.URL)
	pages, err := c.Recognize(context.Background(), pdfBytes, "application/pdf")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server hits = %d, want exactly 2", got)
	}
	want := []string{"page1", "page2"}
	if len(pages) != len(want) {
		t.Fatalf("len(pages) = %d, want %d", len(pages), len(want))
	}
	for i := range want {
		if pages[i] != want[i] {
			t.Errorf("pages[%d] = %q, want %q", i, pages[i], want[i])
		}
	}
}

func TestOCR_SinglePagePDF(t *testing.T) {
	pdfBytes := fixturePDF(t, "one-page.pdf")

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body := decodeOCRRequest(t, r)
		if body.MimeType != "application/pdf" {
			t.Errorf("mimeType = %q, want application/pdf", body.MimeType)
		}
		writeOCRResponse(t, w, "single-page-text")
	}))
	defer srv.Close()

	c := NewOCRClient("test-token", "test-folder", srv.URL)
	pages, err := c.Recognize(context.Background(), pdfBytes, "application/pdf")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server hits = %d, want exactly 1", got)
	}
	if len(pages) != 1 {
		t.Fatalf("len(pages) = %d, want 1", len(pages))
	}
	if pages[0] != "single-page-text" {
		t.Errorf("pages[0] = %q, want single-page-text", pages[0])
	}
}

func TestOCR_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "internal server boom")
	}))
	defer srv.Close()

	c := NewOCRClient("test-token", "test-folder", srv.URL)
	_, err := c.Recognize(context.Background(), []byte("x"), "image/png")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "500") {
		t.Errorf("error = %q, want substring 500", msg)
	}
	if !strings.Contains(msg, "internal server boom") {
		t.Errorf("error = %q, want substring of body", msg)
	}
}

func TestOCR_EmptyAnnotation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result":{}}`)
	}))
	defer srv.Close()

	c := NewOCRClient("test-token", "test-folder", srv.URL)
	pages, err := c.Recognize(context.Background(), []byte("x"), "image/jpeg")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(pages) != 1 {
		t.Fatalf("len(pages) = %d, want 1", len(pages))
	}
	if pages[0] != "" {
		t.Errorf("pages[0] = %q, want empty string", pages[0])
	}
}

func TestOCR_FromTokenSource(t *testing.T) {
	keyJSON, _ := makeFixtureKeyJSON(t)

	var iamCalls int32
	const issuedToken = "iam-token-for-ocr"
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

	var ocrCalls int32
	ocrSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&ocrCalls, 1)
		want := "Bearer " + issuedToken
		if got := r.Header.Get("Authorization"); got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		writeOCRResponse(t, w, "from-source-ocr")
	}))
	defer ocrSrv.Close()

	ts, err := NewTokenSource(keyJSON)
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}

	c := NewOCRClientFromSource(ts, "test-folder", ocrSrv.URL)
	pages, err := c.Recognize(context.Background(), []byte("img"), "image/png")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(pages) != 1 || pages[0] != "from-source-ocr" {
		t.Errorf("pages = %v, want [from-source-ocr]", pages)
	}
	if n := atomic.LoadInt32(&iamCalls); n != 1 {
		t.Errorf("iam calls = %d, want exactly 1", n)
	}
	if n := atomic.LoadInt32(&ocrCalls); n != 1 {
		t.Errorf("ocr calls = %d, want exactly 1", n)
	}
}

// TestOCR_DefaultEndpointWhenEmpty exercises the constructor branch where
// endpoint == "" falls back to defaultOCREndpoint. We flip the package var
// to point at a httptest server so no real network call goes out.
func TestOCR_DefaultEndpointWhenEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOCRResponse(t, w, "default-ocr-ok")
	}))
	defer srv.Close()

	prev := defaultOCREndpoint
	defaultOCREndpoint = srv.URL
	defer func() { defaultOCREndpoint = prev }()

	c := NewOCRClient("test-token", "test-folder", "")
	pages, err := c.Recognize(context.Background(), []byte("x"), "image/jpeg")
	if err != nil {
		t.Fatalf("Recognize: %v", err)
	}
	if len(pages) != 1 || pages[0] != "default-ocr-ok" {
		t.Errorf("pages = %v, want [default-ocr-ok]", pages)
	}
}

// dirSnapshot returns a stable, comparable string of the entries in dir,
// or "<missing>" if dir does not exist. It is used to detect whether
// pdfcpu silently writes config files to a directory between two
// observations.
func dirSnapshot(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "<missing>"
	}
	if err != nil {
		t.Fatalf("readdir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// TestNoPDFCPUConfigSideEffects guards the api.DisableConfigDir() call in
// pdfcpu_init.go. Without that init, pdfcpu would lazily create
// ~/Library/Application Support/pdfcpu/{config.yml,fonts/,certs/} on the
// first multi-page PDF call — which breaks read-only container
// filesystems and pollutes CI sandboxes.
//
// The test is idempotent: if the pdfcpu directory was already created by
// a prior tool run on the developer's machine, the snapshot will compare
// equal because we only check that the OCR call did not ADD entries.
func TestNoPDFCPUConfigSideEffects(t *testing.T) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("UserConfigDir unavailable")
	}
	pdfcpuDir := filepath.Join(cfgDir, "pdfcpu")

	before := dirSnapshot(t, pdfcpuDir)

	// Drive a multi-page OCR using the committed two-page fixture; the
	// httptest server makes this hermetic. Result is discarded — we only
	// care about disk side-effects of the pdfcpu split.
	pdfBytes := fixturePDF(t, "two-page.pdf")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeOCRResponse(t, w, "side-effect-test")
	}))
	defer srv.Close()

	c := NewOCRClient("test-token", "test-folder", srv.URL)
	if _, err := c.Recognize(context.Background(), pdfBytes, "application/pdf"); err != nil {
		t.Fatalf("Recognize: %v", err)
	}

	after := dirSnapshot(t, pdfcpuDir)
	if before != after {
		t.Fatalf("pdfcpu dir mutated despite DisableConfigDir(): before=%q after=%q", before, after)
	}
}

// TestOCR_MultiPagePDF_ErrorIncludesPageIndex verifies that when a
// per-page OCR call fails, the wrapped error names the offending page.
// We arrange for page 1 to succeed and page 2 to return 503; the
// returned error must contain both "page 2" and the upstream status
// code so a manager triaging logs can identify the bad page.
//
// It also guards against the double-prefix regression
// ("yandex ocr: yandex ocr: status 503").
func TestOCR_MultiPagePDF_ErrorIncludesPageIndex(t *testing.T) {
	pdfBytes := fixturePDF(t, "two-page.pdf")

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			writeOCRResponse(t, w, "page1-ok")
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "upstream busy")
	}))
	defer srv.Close()

	c := NewOCRClient("test-token", "test-folder", srv.URL)
	_, err := c.Recognize(context.Background(), pdfBytes, "application/pdf")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "page 2") {
		t.Errorf("error = %q, want substring \"page 2\"", msg)
	}
	if !strings.Contains(msg, "503") {
		t.Errorf("error = %q, want substring \"503\"", msg)
	}
	if strings.Contains(msg, "yandex ocr: yandex ocr:") {
		t.Errorf("error = %q, double-prefixed (regression)", msg)
	}
}
