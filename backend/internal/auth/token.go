package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const sessionTokenBytes = 32

// NewSessionToken returns a URL-safe base64 string of 32 random bytes
// (43 characters without padding). Use for session cookies.
func NewSessionToken() (string, error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session token rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
