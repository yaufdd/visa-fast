package ai

import (
	"strings"
)

// ComputeFormerNationality maps the form's former-nationality answer to
// the visa anketa's expected token.
//
// The function only inspects formerRu — the value the form sent. The
// older heuristics (USSR mention in place_of_birth, birth date on or
// before 25.12.1991) moved to the form layer, where they are presented
// as a reversible auto-fill the tourist or manager can override. See
// frontend/src/components/forms/steps/PersonalStep.jsx (and the legacy
// frontend/src/components/SubmissionForm.jsx) for the suggestion logic.
//
// Rules:
//  1. empty / "Нет" / "No"        → "NO"  (explicit "no" wins)
//  2. anything containing СССР / Soviet / USSR → "USSR"
//  3. anything else (e.g. a custom country name typed via the
//     "Другое" branch) → "NO" — the visa form's T34 field only
//     renders USSR / NO today; passing a country verbatim is a
//     separate feature.
//
// placeOfBirthRu and birthDate are kept in the signature so existing
// call sites (assembler.go) continue to compile unchanged. They are
// intentionally unused.
func ComputeFormerNationality(formerRu, _placeOfBirthRu, _birthDate string) string {
	s := strings.TrimSpace(formerRu)
	lower := strings.ToLower(s)
	if lower == "" || lower == "нет" || lower == "no" {
		return "NO"
	}
	if containsUSSR(s) {
		return "USSR"
	}
	return "NO"
}

func containsUSSR(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "ссср") ||
		strings.Contains(lower, "soviet") ||
		strings.Contains(lower, "ussr")
}
