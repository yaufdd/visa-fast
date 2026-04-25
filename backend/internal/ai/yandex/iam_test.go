package yandex

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- fixture helpers --------------------------------------------------------

// makeFixtureKeyJSON generates a fresh RSA-2048 key, wraps it as PEM, and
// returns a Yandex authorized_key.json-shaped blob plus the parsed RSA key
// (for tests that need to verify signatures locally).
func makeFixtureKeyJSON(t *testing.T) ([]byte, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: der,
	})
	blob := map[string]string{
		"id":                 "fakekeyid12345",
		"service_account_id": "fakesvcacct67890",
		"private_key":        string(pemBytes),
	}
	js, err := json.Marshal(blob)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return js, priv
}

// decodeJWTPayload base64url-decodes the middle segment of a JWT.
func decodeJWTPayload(t *testing.T, jwt string) map[string]any {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("base64 decode payload: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("json.Unmarshal payload: %v", err)
	}
	return out
}

func decodeJWTHeader(t *testing.T, jwt string) map[string]any {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("base64 decode header: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("json.Unmarshal header: %v", err)
	}
	return out
}

// --- tests ------------------------------------------------------------------

func TestBuildSignedJWT_HasExpectedClaims(t *testing.T) {
	keyJSON, _ := makeFixtureKeyJSON(t)
	ts, err := NewTokenSource(keyJSON)
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}

	before := time.Now().Unix()
	jwt, err := ts.buildSignedJWT()
	if err != nil {
		t.Fatalf("buildSignedJWT: %v", err)
	}
	after := time.Now().Unix()

	header := decodeJWTHeader(t, jwt)
	if header["alg"] != "PS256" {
		t.Errorf("header.alg = %v, want PS256", header["alg"])
	}
	if header["typ"] != "JWT" {
		t.Errorf("header.typ = %v, want JWT", header["typ"])
	}
	if header["kid"] != "fakekeyid12345" {
		t.Errorf("header.kid = %v, want fakekeyid12345", header["kid"])
	}

	payload := decodeJWTPayload(t, jwt)
	if payload["iss"] != "fakesvcacct67890" {
		t.Errorf("payload.iss = %v, want fakesvcacct67890", payload["iss"])
	}
	if payload["aud"] != "https://iam.api.cloud.yandex.net/iam/v1/tokens" {
		t.Errorf("payload.aud = %v", payload["aud"])
	}

	iat, ok := payload["iat"].(float64)
	if !ok {
		t.Fatalf("payload.iat missing or wrong type: %v", payload["iat"])
	}
	if int64(iat) < before || int64(iat) > after {
		t.Errorf("payload.iat = %v, want in [%d, %d]", iat, before, after)
	}

	exp, ok := payload["exp"].(float64)
	if !ok {
		t.Fatalf("payload.exp missing or wrong type: %v", payload["exp"])
	}
	expDelta := int64(exp) - int64(iat)
	if expDelta < 3500 || expDelta > 3700 {
		t.Errorf("exp - iat = %d, want ~3600 (1h)", expDelta)
	}
}

func TestTokenSource_RefreshAndCache(t *testing.T) {
	keyJSON, _ := makeFixtureKeyJSON(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		body, _ := io.ReadAll(r.Body)
		var in struct {
			JWT string `json:"jwt"`
		}
		if err := json.Unmarshal(body, &in); err != nil {
			http.Error(w, "bad body", 400)
			return
		}
		if in.JWT == "" {
			http.Error(w, "missing jwt", 400)
			return
		}
		n := atomic.LoadInt32(&calls)
		resp := map[string]string{
			"iamToken":  fmt.Sprintf("token-%d", n),
			"expiresAt": time.Now().Add(1 * time.Hour).Format(time.RFC3339Nano),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	prev := iamEndpoint
	iamEndpoint = srv.URL
	defer func() { iamEndpoint = prev }()

	ts, err := NewTokenSource(keyJSON)
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}

	ctx := context.Background()
	tok1, err := ts.Token(ctx)
	if err != nil {
		t.Fatalf("Token() #1: %v", err)
	}
	tok2, err := ts.Token(ctx)
	if err != nil {
		t.Fatalf("Token() #2: %v", err)
	}
	if tok1 != tok2 {
		t.Errorf("expected cached token, got %q vs %q", tok1, tok2)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected exactly 1 HTTP call, got %d", got)
	}
}

func TestTokenSource_RefreshOnExpiry(t *testing.T) {
	keyJSON, _ := makeFixtureKeyJSON(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		// First call returns a soon-to-expire token; subsequent calls return long-lived ones.
		var expires time.Time
		if n == 1 {
			expires = time.Now().Add(30 * time.Second)
		} else {
			expires = time.Now().Add(1 * time.Hour)
		}
		resp := map[string]string{
			"iamToken":  fmt.Sprintf("token-%d", n),
			"expiresAt": expires.Format(time.RFC3339Nano),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	prev := iamEndpoint
	iamEndpoint = srv.URL
	defer func() { iamEndpoint = prev }()

	ts, err := NewTokenSource(keyJSON)
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}

	ctx := context.Background()
	tok1, err := ts.Token(ctx)
	if err != nil {
		t.Fatalf("Token() #1: %v", err)
	}
	if tok1 != "token-1" {
		t.Errorf("first token = %q, want token-1", tok1)
	}
	// Token-1 expires in 30s which is <= 60s threshold → next Token() refreshes.
	tok2, err := ts.Token(ctx)
	if err != nil {
		t.Fatalf("Token() #2: %v", err)
	}
	if tok2 != "token-2" {
		t.Errorf("second token = %q, want token-2 (expected refresh)", tok2)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 HTTP calls after expiry-driven refresh, got %d", got)
	}
}

func TestTokenSource_ConcurrentToken(t *testing.T) {
	keyJSON, _ := makeFixtureKeyJSON(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		// Slight delay so concurrent callers must actually race for the lock.
		time.Sleep(20 * time.Millisecond)
		resp := map[string]string{
			"iamToken":  "shared-token",
			"expiresAt": time.Now().Add(1 * time.Hour).Format(time.RFC3339Nano),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	prev := iamEndpoint
	iamEndpoint = srv.URL
	defer func() { iamEndpoint = prev }()

	ts, err := NewTokenSource(keyJSON)
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}

	const N = 50
	var wg sync.WaitGroup
	tokens := make([]string, N)
	errs := make([]error, N)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			tok, err := ts.Token(context.Background())
			tokens[i] = tok
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
		if tokens[i] != "shared-token" {
			t.Errorf("goroutine %d: token = %q", i, tokens[i])
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected exactly 1 HTTP call across %d concurrent callers, got %d", N, got)
	}
}

// setRefreshIntervalForTest swaps refreshInterval for the duration of the
// test and restores the previous value on cleanup. Test-only — production
// code must never call this.
func setRefreshIntervalForTest(t *testing.T, d time.Duration) {
	t.Helper()
	prev := refreshInterval
	refreshInterval = d
	t.Cleanup(func() { refreshInterval = prev })
}

// setBackoffForTest swaps the backoff knobs and restores them on cleanup.
func setBackoffForTest(t *testing.T, initial, max time.Duration) {
	t.Helper()
	prevI, prevM := refreshBackoffInitial, refreshBackoffMax
	refreshBackoffInitial = initial
	refreshBackoffMax = max
	t.Cleanup(func() {
		refreshBackoffInitial = prevI
		refreshBackoffMax = prevM
	})
}

// setIAMEndpointForTest swaps iamEndpoint and restores it on cleanup.
func setIAMEndpointForTest(t *testing.T, url string) {
	t.Helper()
	prev := iamEndpoint
	iamEndpoint = url
	t.Cleanup(func() { iamEndpoint = prev })
}

// TestRefreshLoop_BackoffOnFailure verifies that a transient IAM failure
// does not wedge the loop into the next ~11h slot. The fake server returns
// 503 on its first request and a healthy token thereafter; with timer +
// backoff both shrunk to ~50 ms, two requests should arrive within ~200 ms.
func TestRefreshLoop_BackoffOnFailure(t *testing.T) {
	keyJSON, _ := makeFixtureKeyJSON(t)

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		resp := map[string]string{
			"iamToken":  fmt.Sprintf("token-%d", n),
			"expiresAt": time.Now().Add(1 * time.Hour).Format(time.RFC3339Nano),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	setIAMEndpointForTest(t, srv.URL)
	setRefreshIntervalForTest(t, 50*time.Millisecond)
	setBackoffForTest(t, 50*time.Millisecond, 200*time.Millisecond)

	ts, err := NewTokenSource(keyJSON)
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts.Start(ctx)

	// Wait for at least 2 requests (initial 503 + at least one retry).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&calls) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	got := atomic.LoadInt32(&calls)
	if got < 2 {
		t.Fatalf("expected loop to retry after 503, got only %d call(s)", got)
	}

	// Cancel and give the goroutine a beat to exit cleanly so the race
	// detector has nothing in flight when t.Cleanup restores package vars.
	cancel()
	time.Sleep(100 * time.Millisecond)
}

func TestNewTokenSource_RejectsMalformedJSON(t *testing.T) {
	if _, err := NewTokenSource([]byte("{not json")); err == nil {
		t.Errorf("expected error for malformed JSON, got nil")
	}
}

func TestNewTokenSource_RejectsBadPEM(t *testing.T) {
	blob := map[string]string{
		"id":                 "k",
		"service_account_id": "s",
		"private_key":        "not a pem",
	}
	js, _ := json.Marshal(blob)
	if _, err := NewTokenSource(js); err == nil {
		t.Errorf("expected error for bad PEM, got nil")
	}
}
