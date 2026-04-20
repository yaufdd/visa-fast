// Package translit implements Russian → Latin transliteration per
// ICAO Doc 9303 (the Russian MVD standard used on passports).
package translit

import (
	"strings"
	"unicode"
)

// icaoMap maps one Cyrillic rune to its Latin equivalent. Multi-letter
// digraphs (zh, kh, ts, ch, sh, shch, iu, ia) are produced naturally by
// this mapping per ICAO Doc 9303.
var icaoMap = map[rune]string{
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D",
	'Е': "E", 'Ё': "E", 'Ж': "ZH", 'З': "Z", 'И': "I",
	'Й': "I", 'К': "K", 'Л': "L", 'М': "M", 'Н': "N",
	'О': "O", 'П': "P", 'Р': "R", 'С': "S", 'Т': "T",
	'У': "U", 'Ф': "F", 'Х': "KH", 'Ц': "TC", 'Ч': "CH",
	'Ш': "SH", 'Щ': "SHCH", 'Ъ': "IE", 'Ы': "Y", 'Ь': "",
	'Э': "E", 'Ю': "IU", 'Я': "IA",
}

// RuToLatICAO transliterates Russian Cyrillic to uppercase Latin using
// the ICAO Doc 9303 rules. Non-Cyrillic characters (Latin, digits,
// punctuation, whitespace) are preserved as-is but uppercased.
// Lowercase Cyrillic is handled by uppercasing first, so "Иван" and
// "иван" produce the same output.
func RuToLatICAO(s string) string {
	if s == "" {
		return ""
	}
	upper := strings.ToUpper(s)
	var b strings.Builder
	b.Grow(len(upper) * 2)
	for _, r := range upper {
		if v, ok := icaoMap[r]; ok {
			b.WriteString(v)
			continue
		}
		if unicode.IsSpace(r) || unicode.IsDigit(r) || unicode.IsPunct(r) ||
			(r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
