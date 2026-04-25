// Package yandex contains the Yandex Cloud IAM token source used by the
// Russian-AI fallback path (GPT-on-YandexGPT, OCR-on-Vision). A single
// TokenSource exchanges a freshly-signed JWT (PS256, derived from a Yandex
// service-account authorized_key.json) for a 12-hour IAM bearer token at
// https://iam.api.cloud.yandex.net/iam/v1/tokens, caches it under a mutex,
// and proactively refreshes via an optional background goroutine.
//
// This file is the foundation for tasks 1.A3 (GPT client) and 1.A4 (OCR
// client). It does not import anything from the parent ai package — the
// goal is a tiny, dependency-light unit that can be unit-tested in
// isolation.
package yandex

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// iamEndpoint is the Yandex Cloud IAM exchange URL. Mutable so tests can
// point it at httptest.Server. Treat as read-only outside tests.
var iamEndpoint = "https://iam.api.cloud.yandex.net/iam/v1/tokens"

// httpClient is the shared HTTP client for IAM exchange. Mutable so tests
// can swap it; production code must not reassign.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// refreshThreshold is how close to expiry we consider a token stale.
const refreshThreshold = 60 * time.Second

// refreshInterval is the period of the background refresh goroutine.
// Yandex IAM tokens last 12h; we refresh roughly every 11h to give us a
// generous safety margin.
const refreshInterval = 11 * time.Hour

// authorizedKey is the on-disk shape of `yc iam key create --output ...`.
type authorizedKey struct {
	ID               string `json:"id"`
	ServiceAccountID string `json:"service_account_id"`
	PrivateKey       string `json:"private_key"`
}

// TokenSource issues IAM bearer tokens for a single Yandex service account.
// All exported methods are safe for concurrent use.
type TokenSource struct {
	keyID            string
	serviceAccountID string
	privateKey       *rsa.PrivateKey

	mu       sync.Mutex // guards token, exp, started
	token    string
	exp      time.Time
	started  bool
	stopOnce sync.Once
}

// NewTokenSource parses the contents of a Yandex authorized_key.json file
// and returns a TokenSource ready to issue IAM tokens. Returns an error
// if the JSON is malformed, the required fields are missing, or the
// embedded PEM private key is unparseable.
func NewTokenSource(keyJSON []byte) (*TokenSource, error) {
	var k authorizedKey
	if err := json.Unmarshal(keyJSON, &k); err != nil {
		return nil, fmt.Errorf("yandex: parse authorized_key.json: %w", err)
	}
	if k.ID == "" || k.ServiceAccountID == "" || k.PrivateKey == "" {
		return nil, errors.New("yandex: authorized_key.json missing id/service_account_id/private_key")
	}

	block, _ := pem.Decode([]byte(k.PrivateKey))
	if block == nil {
		return nil, errors.New("yandex: private_key is not valid PEM")
	}
	priv, err := parseRSAPrivateKey(block)
	if err != nil {
		return nil, fmt.Errorf("yandex: parse private key: %w", err)
	}

	return &TokenSource{
		keyID:            k.ID,
		serviceAccountID: k.ServiceAccountID,
		privateKey:       priv,
	}, nil
}

// parseRSAPrivateKey accepts both PKCS#1 ("RSA PRIVATE KEY") and PKCS#8
// ("PRIVATE KEY") PEM blocks — Yandex emits PKCS#8 today but older keys
// in the wild use PKCS#1.
func parseRSAPrivateKey(block *pem.Block) (*rsa.PrivateKey, error) {
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	anyKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rsaKey, ok := anyKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA (got %T)", anyKey)
	}
	return rsaKey, nil
}

// Token returns a non-empty IAM bearer token, refreshing if the cached
// token is missing or within refreshThreshold of expiry. Single-flight:
// concurrent callers share one HTTP exchange.
func (ts *TokenSource) Token(ctx context.Context) (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.token != "" && time.Until(ts.exp) > refreshThreshold {
		return ts.token, nil
	}
	if err := ts.refreshLocked(ctx); err != nil {
		return "", err
	}
	return ts.token, nil
}

// refreshLocked performs one JWT-for-IAM-token exchange. Caller must hold
// ts.mu.
func (ts *TokenSource) refreshLocked(ctx context.Context) error {
	signed, err := ts.buildSignedJWT()
	if err != nil {
		return fmt.Errorf("yandex: build jwt: %w", err)
	}

	body, err := json.Marshal(map[string]string{"jwt": signed})
	if err != nil {
		return fmt.Errorf("yandex: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, iamEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("yandex: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("yandex: iam exchange: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("yandex: read iam response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("yandex: iam exchange status %d: %s", resp.StatusCode, string(respBody))
	}

	var out struct {
		IAMToken  string `json:"iamToken"`
		ExpiresAt string `json:"expiresAt"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return fmt.Errorf("yandex: decode iam response: %w", err)
	}
	if out.IAMToken == "" {
		return errors.New("yandex: iam response missing iamToken")
	}

	exp, err := time.Parse(time.RFC3339Nano, out.ExpiresAt)
	if err != nil {
		// Fall back to RFC3339 (no fractional seconds) for tolerance.
		if exp, err = time.Parse(time.RFC3339, out.ExpiresAt); err != nil {
			return fmt.Errorf("yandex: parse expiresAt %q: %w", out.ExpiresAt, err)
		}
	}

	ts.token = out.IAMToken
	ts.exp = exp
	return nil
}

// buildSignedJWT constructs and signs a fresh PS256 JWT for the IAM
// exchange endpoint. Lifetime: 1 hour.
func (ts *TokenSource) buildSignedJWT() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": ts.serviceAccountID,
		"aud": "https://iam.api.cloud.yandex.net/iam/v1/tokens",
		"iat": now.Unix(),
		"exp": now.Add(1 * time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodPS256, claims)
	tok.Header["kid"] = ts.keyID
	// "typ" and "alg" are set by jwt.NewWithClaims; we only need to add kid.
	return tok.SignedString(ts.privateKey)
}

// Start spawns a background goroutine that refreshes the IAM token every
// refreshInterval (~11h) until ctx is cancelled. On refresh failure the
// previous cached token is kept and a slog.Error is emitted (the SA key
// is never logged). Idempotent — subsequent calls are no-ops.
func (ts *TokenSource) Start(ctx context.Context) {
	ts.mu.Lock()
	if ts.started {
		ts.mu.Unlock()
		return
	}
	ts.started = true
	ts.mu.Unlock()

	go ts.refreshLoop(ctx)
}

func (ts *TokenSource) refreshLoop(ctx context.Context) {
	t := time.NewTimer(refreshInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ts.mu.Lock()
			err := ts.refreshLocked(ctx)
			ts.mu.Unlock()
			if err != nil {
				slog.Error("yandex iam refresh failed", "err", err)
			} else {
				slog.Info("yandex iam token refreshed")
			}
			t.Reset(refreshInterval)
		}
	}
}
