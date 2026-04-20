package auth

import (
	"crypto/rand"
	"fmt"
)

const (
	slugAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	slugLength   = 7
)

// NewOrgSlug returns a 7-character random base62 string.
// 62^7 = ~3.5 trillion combinations — collisions are negligible, but
// callers should retry on unique-violation from the DB.
func NewOrgSlug() (string, error) {
	buf := make([]byte, slugLength)
	rnd := make([]byte, slugLength)
	if _, err := rand.Read(rnd); err != nil {
		return "", fmt.Errorf("slug rand: %w", err)
	}
	for i, b := range rnd {
		buf[i] = slugAlphabet[int(b)%len(slugAlphabet)]
	}
	return string(buf), nil
}
