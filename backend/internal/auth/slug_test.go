package auth

import (
	"regexp"
	"testing"
)

func TestNewOrgSlug_LengthAndAlphabet(t *testing.T) {
	for i := 0; i < 100; i++ {
		s, err := NewOrgSlug()
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 7 {
			t.Errorf("slug length %d != 7: %q", len(s), s)
		}
		if !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(s) {
			t.Errorf("slug not base62: %q", s)
		}
	}
}

func TestNewOrgSlug_Unique(t *testing.T) {
	seen := make(map[string]bool, 10000)
	for i := 0; i < 10000; i++ {
		s, _ := NewOrgSlug()
		if seen[s] {
			t.Fatalf("collision after %d iterations: %q", i, s)
		}
		seen[s] = true
	}
}
