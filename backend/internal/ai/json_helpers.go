package ai

import "strings"

// extractJSON strips prose around the outermost JSON value. It picks
// whichever of `{` or `[` appears first in the string and slices to the
// matching last `}` / `]`. This correctly handles both bare objects and
// arrays of objects (where naively searching for `{...}` first would
// slice from the first inner `{` to the last inner `}`).
//
// Used by every Yandex parser/translator that asks the LLM for JSON —
// real-world responses sometimes include a leading "Here is the JSON:"
// or trailing prose despite system-prompt instructions.
func extractJSON(s string) string {
	firstObj := strings.IndexByte(s, '{')
	firstArr := strings.IndexByte(s, '[')
	var start int
	var closeCh byte
	switch {
	case firstObj >= 0 && (firstArr < 0 || firstObj < firstArr):
		start, closeCh = firstObj, '}'
	case firstArr >= 0:
		start, closeCh = firstArr, ']'
	default:
		return s
	}
	end := strings.LastIndexByte(s, closeCh)
	if end > start {
		return s[start : end+1]
	}
	return s
}
