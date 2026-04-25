// Package yandex (ocr.go) implements a thin client over the Yandex Cloud
// Vision OCR sync endpoint (POST /ocr/v1/recognizeText). The client is
// page-oriented: Recognize returns one string per page of the input. For
// images that always means a one-element slice; for PDFs it means the
// document is split locally (via pdfcpu) into single-page PDFs and each
// page is sent in its own HTTP call, because the sync endpoint currently
// accepts only one page at a time.
//
// Like gpt.go, this client is deliberately provider-shaped: it returns
// raw fullText strings and does not interpret block / line / vertex
// metadata — callers in backend/internal/ai/* (ticket_parser,
// voucher_parser, the upcoming passport_parser) are responsible for
// parsing.
package yandex

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// defaultOCREndpoint is the Yandex Cloud Vision OCR base URL. Mutable so
// tests can point the client at httptest.Server. Treat as read-only
// outside tests.
var defaultOCREndpoint = "https://ocr.api.cloud.yandex.net"

// ocrHTTPClient is the shared HTTP client for Vision OCR calls. PDFs with
// many pages take noticeably longer than the Foundation Models endpoint,
// so we allow a 60s ceiling per request. Mutable so tests can swap it;
// production code must not reassign.
var ocrHTTPClient = &http.Client{Timeout: 60 * time.Second}

// OCRClient performs synchronous text recognition against the Yandex
// Vision API. Construct via NewOCRClient (static IAM token, mostly for
// tests) or NewOCRClientFromSource (production wiring with auto-
// refreshing TokenSource). All exported methods are safe for concurrent
// use.
type OCRClient struct {
	tokenSource *TokenSource // optional; takes precedence over staticToken
	staticToken string
	folderID    string
	endpoint    string
}

// NewOCRClient builds a client that uses a fixed bearer token. Intended
// for tests and one-shot scripts. For production wiring, prefer
// NewOCRClientFromSource so tokens are refreshed transparently.
//
// If endpoint is empty, defaultOCREndpoint is used.
func NewOCRClient(iamToken, folderID, endpoint string) *OCRClient {
	if endpoint == "" {
		endpoint = defaultOCREndpoint
	}
	return &OCRClient{
		staticToken: iamToken,
		folderID:    folderID,
		endpoint:    endpoint,
	}
}

// NewOCRClientFromSource builds a client that pulls a fresh IAM token
// from ts on every Recognize call. ts.Token caches under the hood, so
// the per-call cost is a single mutex acquisition once the token is
// warm.
//
// If endpoint is empty, defaultOCREndpoint is used.
func NewOCRClientFromSource(ts *TokenSource, folderID, endpoint string) *OCRClient {
	if endpoint == "" {
		endpoint = defaultOCREndpoint
	}
	return &OCRClient{
		tokenSource: ts,
		folderID:    folderID,
		endpoint:    endpoint,
	}
}

// ocrRequestBody mirrors the wire shape of POST /ocr/v1/recognizeText.
type ocrRequestBody struct {
	MimeType      string   `json:"mimeType"`
	LanguageCodes []string `json:"languageCodes"`
	Model         string   `json:"model"`
	Content       string   `json:"content"` // base64-encoded
}

// ocrResponseBody is the subset of the response we consume.
type ocrResponseBody struct {
	Result struct {
		TextAnnotation struct {
			FullText string `json:"fullText"`
		} `json:"textAnnotation"`
	} `json:"result"`
}

// Recognize returns one fullText string per page of the input. Single-
// page documents and images always return a one-element slice. Multi-
// page PDFs are split locally with pdfcpu and each page is sent in its
// own request, in document order.
//
// mime must be one of "application/pdf", "image/jpeg", or "image/png".
// (The Yandex API accepts more, but these are the only ones the
// FujiTravel pipeline actually feeds in today; allowing unbounded
// passthrough would require trusting upstream MIME detection too far.)
func (c *OCRClient) Recognize(ctx context.Context, content []byte, mime string) ([]string, error) {
	if mime != "application/pdf" && mime != "image/jpeg" && mime != "image/png" {
		return nil, fmt.Errorf("yandex ocr: unsupported mime %q", mime)
	}

	if mime != "application/pdf" {
		text, err := c.recognizeOnce(ctx, content, mime)
		if err != nil {
			return nil, err
		}
		return []string{text}, nil
	}

	// PDF path: count pages first; only split if multi-page so that
	// 1-page PDFs incur no extra parsing/serialization work.
	pageCount, err := api.PageCount(bytes.NewReader(content), model.NewDefaultConfiguration())
	if err != nil {
		return nil, fmt.Errorf("yandex ocr: read pdf page count: %w", err)
	}
	if pageCount <= 0 {
		return nil, fmt.Errorf("yandex ocr: pdf has no pages")
	}
	if pageCount == 1 {
		text, err := c.recognizeOnce(ctx, content, mime)
		if err != nil {
			return nil, err
		}
		return []string{text}, nil
	}

	results := make([]string, 0, pageCount)
	for i := 1; i <= pageCount; i++ {
		pageBytes, err := extractSinglePage(content, i)
		if err != nil {
			return nil, fmt.Errorf("yandex ocr: split pdf page %d: %w", i, err)
		}
		text, err := c.recognizeOnce(ctx, pageBytes, mime)
		if err != nil {
			return nil, err
		}
		results = append(results, text)
	}
	return results, nil
}

// extractSinglePage uses pdfcpu's Trim to produce a single-page PDF
// containing only pageNr (1-indexed) of the source. Trim takes selected
// pages and writes a PDF stream of just those pages — exactly what the
// Vision sync endpoint expects.
func extractSinglePage(pdfBytes []byte, pageNr int) ([]byte, error) {
	var buf bytes.Buffer
	conf := model.NewDefaultConfiguration()
	if err := api.Trim(bytes.NewReader(pdfBytes), &buf, []string{strconv.Itoa(pageNr)}, conf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// recognizeOnce performs one HTTP call against the sync OCR endpoint
// and returns the recognized fullText (possibly empty) or a wrapped
// error. Empty / missing textAnnotation is logged at warn but not
// treated as a failure: a multi-page PDF where one page is blank should
// still produce results for the other pages.
func (c *OCRClient) recognizeOnce(ctx context.Context, content []byte, mime string) (string, error) {
	bearer, err := c.bearer(ctx)
	if err != nil {
		return "", fmt.Errorf("yandex ocr: obtain iam token: %w", err)
	}

	body := ocrRequestBody{
		MimeType:      mime,
		LanguageCodes: []string{"*"},
		Model:         "page",
		Content:       base64.StdEncoding.EncodeToString(content),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("yandex ocr: marshal request: %w", err)
	}

	url := c.endpoint + "/ocr/v1/recognizeText"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("yandex ocr: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+bearer)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-folder-id", c.folderID)

	resp, err := ocrHTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("yandex ocr: round-trip: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("yandex ocr: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("yandex ocr: status %d: %s", resp.StatusCode, truncate(string(respBody), 512))
	}

	var out ocrResponseBody
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("yandex ocr: decode response: %w", err)
	}
	text := out.Result.TextAnnotation.FullText
	if text == "" {
		// Don't fail the page — a blank scan or a layout-only page can
		// legitimately produce no text. Caller decides whether the
		// downstream consumer (translate / parser) cares.
		slog.Warn("yandex ocr: empty fullText", "mime", mime, "bytes", len(content))
	}
	return text, nil
}

// bearer returns the IAM bearer token to use for one Recognize call. If
// the client was built with a TokenSource, that wins (production path);
// else the static token from NewOCRClient is used (tests / scripts).
func (c *OCRClient) bearer(ctx context.Context) (string, error) {
	if c.tokenSource != nil {
		return c.tokenSource.Token(ctx)
	}
	return c.staticToken, nil
}
