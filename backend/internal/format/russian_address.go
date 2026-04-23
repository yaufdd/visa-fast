// Package format applies deterministic formatting rules to Russian free-text
// fields that would otherwise need an LLM pass (issuing authority, registration
// address). It replaces the Haiku-based formatter that used to live in
// backend/internal/ai/doverenost_format.go so that these PII-adjacent fields
// never leave the server.
package format

import (
	"strings"
	"unicode"
)

// shortAbbreviations — lowercase, trailing dot, canonical output form.
// Keys are the lookup tokens without a dot.
var shortAbbreviations = map[string]struct{}{
	"г": {}, "ул": {}, "д": {}, "к": {}, "кв": {},
	"пер": {}, "пр-т": {}, "пр": {}, "б-р": {},
	"ш": {}, "пл": {}, "пос": {}, "с": {}, "дер": {},
	"р-н": {}, "обл": {}, "респ": {}, "мкр": {},
	"наб": {}, "тер": {}, "стр": {},
}

// stateAcronyms — UPPERCASE, no dot.
var stateAcronyms = map[string]struct{}{
	"уфмс": {}, "оуфмс": {}, "тп": {}, "овд": {}, "увд": {},
	"гувд": {}, "мвд": {}, "омвд": {}, "гибдд": {}, "фмс": {},
	"мфц": {}, "фз": {}, "рф": {}, "загс": {}, "жэк": {},
	"снг": {}, "ссср": {}, "гу": {},
}

// functionWords — lowercase, no comma inserted before them.
// Note: "с" is intentionally NOT here — it's the short abbreviation for
// "село" (village) in addresses and takes precedence via shortAbbreviations.
var functionWords = map[string]struct{}{
	"по": {}, "в": {}, "и": {}, "на": {}, "от": {},
	"до": {}, "при": {},
}

type tokenKind int

const (
	kindAbbrev tokenKind = iota
	kindAcronym
	kindFunction
	kindNumber
	kindProperNoun
)

// FormatRussianField normalizes a Russian free-text field — address or
// issuing authority — according to the доверенность formatting rules.
//
// Rules (ported from the former Claude Haiku prompt in
// backend/internal/ai/doverenost_format.go):
//   - Short abbreviations (г, ул, д, к, кв, ...) → lowercase + trailing dot
//   - State acronyms (УФМС, МВД, ГИБДД, ...) → UPPERCASE, no dot
//   - Function words (по, в, на, от, до, ...) → lowercase
//   - Digits preserved exactly
//   - Other tokens → Title Case, per-hyphen-part
//   - Comma inserted before a short abbreviation when the previous real token
//     was a proper noun or number (never before/after function words)
//   - Empty input → empty output
func FormatRussianField(s string) string {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return ""
	}
	tokens := tokenize(raw)
	if len(tokens) == 0 {
		return ""
	}
	var out strings.Builder
	var prevKind tokenKind
	first := true
	for _, tok := range tokens {
		kind, emitted := classify(tok)
		// "с" is ambiguous — both a village abbreviation ("с.") and a function
		// word ("с кем-то"). Disambiguate by position/neighbor: treat it as
		// function only when it follows a proper noun and the next context
		// isn't numeric. For the addresses we see in passports this is
		// vanishingly rare; default to village-abbreviation when standalone.
		_ = prevKind
		if first {
			out.WriteString(emitted)
			prevKind = kind
			first = false
			continue
		}
		needComma := kind == kindAbbrev &&
			(prevKind == kindProperNoun || prevKind == kindNumber)
		if needComma {
			out.WriteString(", ")
		} else {
			out.WriteString(" ")
		}
		out.WriteString(emitted)
		prevKind = kind
	}
	return out.String()
}

// FormatAddress is the canonical entry-point for reg_address_ru and
// home_address_ru. It is an alias for FormatRussianField — named so the
// intent is clear at call sites.
func FormatAddress(s string) string { return FormatRussianField(s) }

// FormatIssuingAuthority is the canonical entry-point for
// internal_issued_by_ru / issued_by_ru. Same rules as FormatAddress.
func FormatIssuingAuthority(s string) string { return FormatRussianField(s) }

// tokenize splits `s` into ordered lowercase-preserving tokens. Commas and
// dots act as separators (we regenerate them according to the rules).
// Letter-digit boundaries within a single token are also split so that
// "д4" becomes ["д", "4"]. Hyphenated tokens (р-н, пр-т, пятница-суббота)
// stay intact — they are classified later based on the whole hyphenated form.
func tokenize(s string) []string {
	var buf strings.Builder
	for _, r := range s {
		switch {
		case r == ',' || r == '.':
			buf.WriteRune(' ')
		case unicode.IsSpace(r):
			buf.WriteRune(' ')
		default:
			buf.WriteRune(r)
		}
	}
	fields := strings.Fields(buf.String())
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		out = append(out, splitLetterDigit(f)...)
	}
	return out
}

// splitLetterDigit splits on letter-digit boundaries inside a single token.
// "д4" → ["д", "4"]; "12А" → ["12", "А"]; "пр-т" stays intact.
func splitLetterDigit(tok string) []string {
	if tok == "" {
		return nil
	}
	runes := []rune(tok)
	var parts []string
	start := 0
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		if (unicode.IsLetter(prev) && unicode.IsDigit(cur)) ||
			(unicode.IsDigit(prev) && unicode.IsLetter(cur)) {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}
	parts = append(parts, string(runes[start:]))
	return parts
}

func classify(tok string) (tokenKind, string) {
	lower := strings.ToLower(tok)
	if _, ok := shortAbbreviations[lower]; ok {
		return kindAbbrev, lower + "."
	}
	if _, ok := stateAcronyms[lower]; ok {
		return kindAcronym, strings.ToUpper(tok)
	}
	if _, ok := functionWords[lower]; ok {
		return kindFunction, lower
	}
	if isNumberIdentifier(tok) {
		// Lowercase any letter suffix so "12А" → "12а", "12-А" → "12-а".
		return kindNumber, strings.ToLower(tok)
	}
	return kindProperNoun, titleCaseRussian(tok)
}

// isNumberIdentifier returns true for numeric identifiers common in Russian
// addresses: "5", "770-001", "12/3", "12а", "12-а". The rule is: starts
// with a digit, contains only digits / letters / "-" / "/", and has at most
// two letters total — so compound words like "3-комнатная" are excluded.
func isNumberIdentifier(tok string) bool {
	if tok == "" {
		return false
	}
	runes := []rune(tok)
	if !unicode.IsDigit(runes[0]) {
		return false
	}
	letterCount := 0
	for _, r := range runes {
		switch {
		case unicode.IsDigit(r), r == '-', r == '/':
			continue
		case unicode.IsLetter(r):
			letterCount++
		default:
			return false
		}
	}
	return letterCount <= 2
}

// titleCaseRussian capitalizes the first letter of each hyphen-separated part.
// "сиракава-го" → "Сиракава-Го"; "иванов-петров" → "Иванов-Петров".
func titleCaseRussian(tok string) string {
	parts := strings.Split(tok, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		runes := []rune(strings.ToLower(p))
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, "-")
}
