package ai

import (
	"strings"
	"unicode"
)

// canonicalCities maps normalized (lowercased, punctuation-stripped) forms of
// every recognised English/Russian city variant to its canonical Russian name.
// The voucher parser typically emits English CAPS (e.g. "TOKYO") — we map it
// to "Токио" so auto-created hotels land with the same spelling the UI
// dropdown expects.
var canonicalCities = map[string]string{
	// Tokyo
	"tokyo": "Токио", "tōkyō": "Токио", "tokio": "Токио", "токио": "Токио",
	// Kyoto
	"kyoto": "Киото", "kyōto": "Киото", "kioto": "Киото", "киото": "Киото",
	// Osaka
	"osaka": "Осака", "ōsaka": "Осака", "осака": "Осака",
	// Hakone
	"hakone": "Хаконэ", "хаконэ": "Хаконэ", "хаконе": "Хаконэ",
	// Izu
	"izu": "Идзу", "идзу": "Идзу", "izuhanto": "Идзу",
	// Okinawa / Naha
	"okinawa": "Окинава", "окинава": "Окинава",
	"naha": "Наха", "наха": "Наха",
	// Nara
	"nara": "Нара", "нара": "Нара",
	// Kanazawa
	"kanazawa": "Канадзава", "канадзава": "Канадзава", "каназава": "Канадзава",
	// Nagoya
	"nagoya": "Нагоя", "нагоя": "Нагоя",
	// Hiroshima
	"hiroshima": "Хиросима", "хиросима": "Хиросима",
	// Nikko
	"nikko": "Никко", "nikkō": "Никко", "никко": "Никко",
	// Yokohama
	"yokohama": "Иокогама", "иокогама": "Иокогама", "йокогама": "Иокогама",
	// Kamakura
	"kamakura": "Камакура", "камакура": "Камакура",
	// Sapporo
	"sapporo": "Саппоро", "саппоро": "Саппоро", "сапоро": "Саппоро",
	// Fukuoka
	"fukuoka": "Фукуока", "фукуока": "Фукуока",
	// Takayama
	"takayama": "Такаяма", "такаяма": "Такаяма",
	// Matsumoto
	"matsumoto": "Мацумото", "мацумото": "Мацумото",
	// Kobe
	"kobe": "Кобе", "kōbe": "Кобе", "кобе": "Кобе",
	// Miyajima
	"miyajima": "Миядзима", "миядзима": "Миядзима",
	// Shirakawa-go
	"shirakawago": "Сиракава-го", "shirakawa": "Сиракава-го",
	"сиракаваго": "Сиракава-го", "сиракавого": "Сиракава-го", "сиракава": "Сиракава-го",
	// Mt Fuji
	"mtfuji": "Гора Фудзи", "mountfuji": "Гора Фудзи", "fuji": "Гора Фудзи",
	"горафудзи": "Гора Фудзи", "фудзи": "Гора Фудзи",
	// Nagano
	"nagano": "Нагано", "нагано": "Нагано",
	// Sendai
	"sendai": "Сэндай", "сэндай": "Сэндай", "сендай": "Сэндай",
}

func normalizeCityKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NormalizeCity maps any known city variant to its canonical Russian spelling.
// Unknown strings are returned trimmed with Title Case applied so a voucher
// entry like "sendai-city" still displays legibly.
func NormalizeCity(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if hit, ok := canonicalCities[normalizeCityKey(s)]; ok {
		return hit
	}
	// Title-case fallback — preserves the rest of the string as-is so a
	// voucher entry like "sendai-city" becomes "Sendai-city".
	runes := []rune(s)
	if len(runes) > 0 {
		runes[0] = unicode.ToUpper(runes[0])
	}
	return string(runes)
}
