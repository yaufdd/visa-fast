package ai

import (
	"strings"
	"time"
)

// ussrDissolution is 25 December 1991 — last day the USSR existed.
var ussrDissolution = time.Date(1991, 12, 25, 23, 59, 59, 0, time.UTC)

// ComputeFormerNationality applies the rules:
//  1. explicit "СССР" / "Soviet" in formerRu     → "USSR"
//  2. "СССР" / "Soviet" / "USSR" in placeOfBirth → "USSR"
//  3. birth date on or before 25.12.1991         → "USSR"
//  4. otherwise                                   → "NO"
func ComputeFormerNationality(formerRu, placeOfBirthRu, birthDate string) string {
	if containsUSSR(formerRu) {
		return "USSR"
	}
	if containsUSSR(placeOfBirthRu) {
		return "USSR"
	}
	if t, err := time.Parse("02.01.2006", strings.TrimSpace(birthDate)); err == nil {
		if !t.After(ussrDissolution) {
			return "USSR"
		}
	}
	return "NO"
}

func containsUSSR(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "ссср") ||
		strings.Contains(lower, "soviet") ||
		strings.Contains(lower, "ussr")
}
