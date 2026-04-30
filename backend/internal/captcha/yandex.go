// Package captcha verifies Yandex SmartCaptcha tokens issued by the
// frontend widget. The package is intentionally tiny and dependency-free
// (stdlib + log/slog) so it can be reused from any handler without
// coupling to the rest of the codebase.
//
// The verifier has a "soft" mode: when constructed with an empty secret
// (the local-dev case where YANDEX_CAPTCHA_SECRET is unset), Verify
// returns nil for any input, including the empty string. Production
// deployments set the secret and the verifier becomes a hard gate on
// the public form.
package captcha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// validateURL is the Yandex SmartCaptcha server-side validation endpoint.
// Documented at https://yandex.cloud/en/docs/smartcaptcha/concepts/validation.
const validateURL = "https://smartcaptcha.cloud.yandex.ru/validate"

// Sentinel errors. Callers can use errors.Is to distinguish between the
// three ways a verification can fail. All three are exported so handlers
// in other packages can pick the right HTTP status / user-facing copy.
var (
	// ErrTokenMissing is returned when the verifier is enabled but the
	// caller passed an empty token (typical when the SmartCaptcha widget
	// hasn't fired or the frontend didn't include the form field).
	ErrTokenMissing = errors.New("captcha token missing")

	// ErrRejected is returned when Yandex responded but its `status`
	// field is not "ok". The remote diagnostic message is logged at
	// Warn level — we do not surface it to end users.
	ErrRejected = errors.New("captcha rejected")

	// ErrTransport wraps any HTTP / network failure when contacting the
	// Yandex validation endpoint. Wrapped (not direct ==) so callers
	// can errors.Is against it but still see the underlying cause via
	// errors.Unwrap if they want to log it.
	ErrTransport = errors.New("captcha transport failure")
)

// Verifier validates SmartCaptcha tokens against the Yandex endpoint.
// The zero value is not usable — construct via New.
type Verifier struct {
	secret string
	client *http.Client
}

// New constructs a Verifier. Empty secret means verification is disabled
// — Verify returns nil immediately for any token. Use this behaviour
// for local dev where YANDEX_CAPTCHA_SECRET is unset.
func New(secret string) *Verifier {
	return &Verifier{
		secret: secret,
		client: &http.Client{
			// 5s gives Yandex plenty of headroom (typical p99 is well
			// under a second) while still keeping a stuck request from
			// holding a tourist's submit button hostage.
			Timeout: 5 * time.Second,
		},
	}
}

// Enabled reports whether verification will actually contact Yandex.
// Used by handlers that want to log the active configuration on startup.
func (v *Verifier) Enabled() bool {
	return v != nil && v.secret != ""
}

// Verify POSTs the token to the Yandex SmartCaptcha validation endpoint
// with the configured secret. The optional ip is best-effort context
// for Yandex; pass an empty string when unavailable.
//
// Returns nil on Yandex `status: "ok"`, or one of the sentinel errors
// otherwise:
//   - ErrTokenMissing when the verifier is enabled but token is empty
//   - ErrRejected     when Yandex responded with status != "ok"
//   - ErrTransport    (wrapped) on HTTP / network / decode failures
//
// When the verifier is disabled (empty secret) Verify is a no-op and
// returns nil regardless of the token value — this is the local-dev
// convenience path.
func (v *Verifier) Verify(ctx context.Context, token, ip string) error {
	if !v.Enabled() {
		return nil
	}
	if strings.TrimSpace(token) == "" {
		return ErrTokenMissing
	}

	form := url.Values{}
	form.Set("secret", v.secret)
	form.Set("token", token)
	if ip != "" {
		form.Set("ip", ip)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, validateURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		// In practice this only fires on a malformed URL / nil ctx — we
		// still wrap as transport so the caller has a single error class
		// to handle for "couldn't talk to Yandex".
		return fmt.Errorf("%w: build request: %v", ErrTransport, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTransport, err)
	}
	defer resp.Body.Close()

	// Cap the response body — the documented payload is a few hundred
	// bytes; reading more would only help an attacker burn memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if err != nil {
		return fmt.Errorf("%w: read response: %v", ErrTransport, err)
	}
	if resp.StatusCode/100 != 2 {
		// Non-2xx is almost always a transient Yandex hiccup; treat as
		// transport so future code can split this off into a 503.
		slog.Warn("smartcaptcha non-2xx response",
			"status", resp.StatusCode, "body_len", len(body))
		return fmt.Errorf("%w: http %d", ErrTransport, resp.StatusCode)
	}

	var parsed struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Host    string `json:"host"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("%w: decode response: %v", ErrTransport, err)
	}

	if parsed.Status != "ok" {
		// Log the remote diagnostic (host can help if a misconfigured
		// site key is sending traffic to the wrong tenant). We
		// deliberately do NOT echo the message to the end user — the
		// caller decides on the user-visible copy.
		slog.Warn("smartcaptcha rejected token",
			"status", parsed.Status,
			"message", parsed.Message,
			"host", parsed.Host)
		return ErrRejected
	}
	return nil
}
