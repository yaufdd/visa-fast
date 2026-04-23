package ai

import (
	"strings"
	"unicode"
)

// TitleCaseRuName capitalizes the first letter of each whitespace- or
// hyphen-separated token and lowercases the rest. Used for person names
// typed in caps ("ИВАНОВ-ПЕТРОВ ПЕТР ИВАНОВИЧ" → "Иванов-Петров Петр
// Иванович") or lowercase ("иванов петр" → "Иванов Петр"). Does NOT touch
// particles inside words — we want proper-noun capitalization, not English
// title-case rules.
func TitleCaseRuName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	out := make([]rune, 0, len(s))
	startOfToken := true
	for _, r := range s {
		if unicode.IsSpace(r) || r == '-' || r == '\u2010' || r == '\u2011' {
			out = append(out, r)
			startOfToken = true
			continue
		}
		if startOfToken {
			out = append(out, unicode.ToUpper(r))
			startOfToken = false
		} else {
			out = append(out, unicode.ToLower(r))
		}
	}
	return string(out)
}
