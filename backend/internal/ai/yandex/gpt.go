// Package yandex (gpt.go) implements a thin client over the Yandex
// Foundation Models "completion" endpoint. It performs single-turn,
// non-streaming chat exchanges against models such as YandexGPT Pro
// (default channel: "yandexgpt/rc" — Pro v5 RC). The client is
// deliberately provider-shaped: it returns the raw assistant text and
// does not attempt to parse JSON-mode payloads — callers in
// backend/internal/ai (translate, programme) are responsible for
// decoding.
package yandex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// defaultGPTEndpoint is the Yandex Cloud Foundation Models base URL.
// Mutable so tests can point the client at httptest.Server. Treat as
// read-only outside tests.
var defaultGPTEndpoint = "https://llm.api.cloud.yandex.net"

// gptHTTPClient is the shared HTTP client for Foundation Models calls.
// 60s is generous enough for programme generation on Opus-class workloads
// while still bounding pathological hangs. Mutable so tests can swap it;
// production code must not reassign.
var gptHTTPClient = &http.Client{Timeout: 60 * time.Second}

// defaultGPTModel is the default model URI suffix. "yandexgpt/rc" is the
// Pro v5 release-candidate channel — the variant we have validated for
// translate.go and programme.go output quality.
const defaultGPTModel = "yandexgpt/rc"

// defaultGPTMaxTokens is the fallback for ChatRequest.MaxTokens == 0.
const defaultGPTMaxTokens = 2048

// ChatRequest describes one round-trip to the Foundation Models API.
type ChatRequest struct {
	System      string  // system prompt; omitted from the request when empty
	User        string  // user message
	Temperature float64 // 0..1, default 0
	MaxTokens   int     // default 2048
	JSONOutput  bool    // request "json" responseFormat instead of "text"
	Model       string  // optional override; default: "yandexgpt/rc" (Pro v5)
}

// GPTClient performs single-turn completions against the Yandex
// Foundation Models API. Construct via NewGPTClient (static IAM token,
// mostly for tests) or NewGPTClientFromSource (production wiring with
// auto-refreshing TokenSource). All exported methods are safe for
// concurrent use.
type GPTClient struct {
	tokenSource *TokenSource // optional; takes precedence over staticToken
	staticToken string
	folderID    string
	endpoint    string
}

// NewGPTClient builds a client that uses a fixed bearer token. Intended
// for tests and one-shot scripts. For production wiring, prefer
// NewGPTClientFromSource so tokens are refreshed transparently.
//
// If endpoint is empty, defaultGPTEndpoint is used.
func NewGPTClient(iamToken, folderID, endpoint string) *GPTClient {
	if endpoint == "" {
		endpoint = defaultGPTEndpoint
	}
	return &GPTClient{
		staticToken: iamToken,
		folderID:    folderID,
		endpoint:    endpoint,
	}
}

// NewGPTClientFromSource builds a client that pulls a fresh IAM token
// from ts on every Chat call. ts.Token caches under the hood, so the
// per-call cost is a single mutex acquisition once the token is warm.
//
// If endpoint is empty, defaultGPTEndpoint is used.
func NewGPTClientFromSource(ts *TokenSource, folderID, endpoint string) *GPTClient {
	if endpoint == "" {
		endpoint = defaultGPTEndpoint
	}
	return &GPTClient{
		tokenSource: ts,
		folderID:    folderID,
		endpoint:    endpoint,
	}
}

// gptMessage is one element of the messages array in the request body.
type gptMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// gptCompletionOptions mirrors the Yandex API shape. Note that maxTokens
// is a STRING in their wire format, not a number — this is a documented
// quirk of the Foundation Models API.
type gptCompletionOptions struct {
	Stream           bool    `json:"stream"`
	Temperature      float64 `json:"temperature"`
	MaxTokens        string  `json:"maxTokens"`
	ReasoningOptions any     `json:"reasoningOptions"`
	ResponseFormat   string  `json:"responseFormat"`
}

// gptRequestBody is the full request envelope.
type gptRequestBody struct {
	ModelURI          string               `json:"modelUri"`
	CompletionOptions gptCompletionOptions `json:"completionOptions"`
	Messages          []gptMessage         `json:"messages"`
}

// gptResponseBody is the subset of the response we consume.
type gptResponseBody struct {
	Result struct {
		Alternatives []struct {
			Message struct {
				Role string `json:"role"`
				Text string `json:"text"`
			} `json:"message"`
			Status string `json:"status"`
		} `json:"alternatives"`
		Usage        any    `json:"usage"`
		ModelVersion string `json:"modelVersion"`
	} `json:"result"`
}

// Chat performs one completion round-trip and returns the assistant
// text from the first alternative. Returns a wrapped error on transport
// failure, non-2xx HTTP status (with a truncated body for diagnosis),
// JSON unmarshal failure, or an empty alternatives array.
func (c *GPTClient) Chat(ctx context.Context, req ChatRequest) (string, error) {
	bearer, err := c.bearer(ctx)
	if err != nil {
		return "", fmt.Errorf("yandex: obtain iam token: %w", err)
	}

	model := req.Model
	if model == "" {
		model = defaultGPTModel
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultGPTMaxTokens
	}
	responseFormat := "text"
	if req.JSONOutput {
		responseFormat = "json"
	}

	messages := make([]gptMessage, 0, 2)
	if req.System != "" {
		messages = append(messages, gptMessage{Role: "system", Text: req.System})
	}
	messages = append(messages, gptMessage{Role: "user", Text: req.User})

	body := gptRequestBody{
		ModelURI: fmt.Sprintf("gpt://%s/%s", c.folderID, model),
		CompletionOptions: gptCompletionOptions{
			Stream:           false,
			Temperature:      req.Temperature,
			MaxTokens:        strconv.Itoa(maxTokens),
			ReasoningOptions: nil,
			ResponseFormat:   responseFormat,
		},
		Messages: messages,
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("yandex: marshal gpt request: %w", err)
	}

	url := c.endpoint + "/foundationModels/v1/completion"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("yandex: build gpt request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+bearer)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-folder-id", c.folderID)

	resp, err := gptHTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("yandex: gpt round-trip: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("yandex: read gpt response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("yandex: gpt status %d: %s", resp.StatusCode, truncate(string(respBody), 512))
	}

	var out gptResponseBody
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("yandex: decode gpt response: %w", err)
	}
	if len(out.Result.Alternatives) == 0 {
		return "", errors.New("yandex: empty completion alternatives")
	}
	text := out.Result.Alternatives[0].Message.Text
	if text == "" {
		return "", errors.New("yandex: empty completion alternatives")
	}
	return text, nil
}

// bearer returns the IAM bearer token to use for one Chat call. If the
// client was built with a TokenSource, that wins (production path); else
// the static token from NewGPTClient is used (tests / scripts).
func (c *GPTClient) bearer(ctx context.Context) (string, error) {
	if c.tokenSource != nil {
		return c.tokenSource.Token(ctx)
	}
	return c.staticToken, nil
}
