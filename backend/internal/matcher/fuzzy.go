package matcher

import (
	"strings"
	"unicode"
)

// Match holds a fuzzy match result.
type Match struct {
	Row   map[string]string
	Score float64 // 0.0 – 1.0
}

// Normalize strips punctuation, collapses whitespace and lowercases s.
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		} else if unicode.IsSpace(r) {
			b.WriteRune(' ')
		}
	}
	// Collapse multiple spaces.
	parts := strings.Fields(b.String())
	return strings.Join(parts, " ")
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	la, lb := len(ra), len(rb)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(del, ins, sub)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

// Similarity returns a score in [0, 1] between two strings using normalised
// Levenshtein distance.
func Similarity(a, b string) float64 {
	na := Normalize(a)
	nb := Normalize(b)
	if na == nb {
		return 1.0
	}
	dist := levenshtein(na, nb)
	maxLen := len([]rune(na))
	if l := len([]rune(nb)); l > maxLen {
		maxLen = l
	}
	if maxLen == 0 {
		return 1.0
	}
	return 1.0 - float64(dist)/float64(maxLen)
}

// TopN returns the top n rows from candidates ranked by fuzzy similarity of
// the value at nameColumn against query.
func TopN(query, nameColumn string, rows []map[string]string, n int) []Match {
	type scored struct {
		row   map[string]string
		score float64
	}

	var all []scored
	for _, row := range rows {
		candidate := row[nameColumn]
		s := Similarity(query, candidate)
		all = append(all, scored{row, s})
	}

	// Sort descending by score (simple insertion sort — datasets are small).
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].score > all[j-1].score; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}

	if n > len(all) {
		n = len(all)
	}
	result := make([]Match, n)
	for i := 0; i < n; i++ {
		result[i] = Match{Row: all[i].row, Score: all[i].score}
	}
	return result
}
