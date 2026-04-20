package auth

import (
	"regexp"
	"testing"
)

func TestNewSessionToken_LengthAndAlphabet(t *testing.T) {
	tok, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 43 {
		t.Errorf("token too short: %d (expected >= 43)", len(tok))
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(tok) {
		t.Errorf("token contains non-urlsafe-base64 chars: %q", tok)
	}
}

func TestNewSessionToken_Unique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		tok, err := NewSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		if seen[tok] {
			t.Fatalf("collision after %d iterations", i)
		}
		seen[tok] = true
	}
}
