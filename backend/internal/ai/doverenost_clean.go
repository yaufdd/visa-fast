package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"fujitravel-admin/backend/internal/ai/yandex"
)

// doverenostCleanSystemPrompt encodes the same canonicalization rules
// previously implemented in backend/internal/format/russian_address.go.
// The rules apply to free-text Russian fields used inside доверенность:
// home/registration addresses and the internal-passport issuing
// authority. The model receives a JSON envelope {"strings":[...]} and
// must reply with a JSON array of cleaned strings in the same order
// and length.
const doverenostCleanSystemPrompt = `You normalise Russian free-text fields used inside a Russian "доверенность" (power of attorney): residential addresses, registration addresses, and internal-passport issuing authority strings ("кем выдан"). Return ONLY a JSON array of cleaned strings, exactly the same length and order as the input. No markdown fences, no prose, no extra keys.

LANGUAGE — input is Russian, output stays Russian. Never translate to English. Never transliterate.

RULES (apply in order):

1. Short Russian abbreviations — lowercase, trailing dot, single space after:
   г, ул, д, к, кв, корп, стр, пер, пр, пр-т, б-р, ш, пл, пос, с, дер, р-н, обл, респ, мкр, наб, тер.
   Examples:
     "Д.4 КВ.23"   → "д. 4, кв. 23"
     "г москва"    → "г. Москва"
     "ул ленина"   → "ул. Ленина"
     "пр-т мира"   → "пр-т. Мира"

2. State acronyms — UPPERCASE, NO trailing dot:
   МВД, УФМС, ОУФМС, ГУ, ФМС, ОВД, УВД, ГУВД, ОМВД, ГИБДД, МФЦ, ЗАГС, ЖЭК, СНГ, СССР, РФ, ФЗ, ТП.
   Examples:
     "оуфмс россии по г москве"  → "ОУФМС России по г. Москве"
     "гу мвд россии по г.москве" → "ГУ МВД России по г. Москве"

3. Function words inside compound names — lowercased (no comma before/after):
   по, в, и, на, от, до, при.

4. Proper nouns (cities, streets, surnames) — Title Case in Russian rules:
   capitalise the first letter of each hyphen-separated part, lowercase the rest.
     "санкт-петербург" → "Санкт-Петербург"
     "сиракава-го"     → "Сиракава-Го"

5. Numbers and number-like identifiers preserved exactly — "770-001", "12а", "12-а" etc. Letter suffixes after digits become lowercase: "12А" → "12а".

6. Comma + single space between address segments. A comma is inserted before a short abbreviation (rule 1) when the previous real token was a proper noun or a number. Never insert a comma before/after function words (rule 3) or state acronyms (rule 2).

7. Empty input string → empty output string in the same slot. Never drop an element. Never reorder. Never merge.

EXAMPLES (input → output):

Input:  ["москва ул митинская д12 кв49","ОУФМС России по г Москве","г. Санкт-Петербург, ул. Пушкина, д. 3"]
Output: ["г. Москва, ул. Митинская, д. 12, кв. 49","ОУФМС России по г. Москве","г. Санкт-Петербург, ул. Пушкина, д. 3"]

Input:  ["УФМС РОССИИ ПО Г. МОСКВЕ В РАЙОНЕ КРЫЛАТСКОЕ"]
Output: ["УФМС России по г. Москве в Районе Крылатское"]

Input:  [""]
Output: [""]`

// CleanDoverenostFields takes a batch of Russian free-text fields
// (home/registration addresses + issuing-authority strings) and returns
// canonically formatted versions in the same order and length.
//
// The pipeline calls this once per /generate run with all
// home_address_ru / reg_address_ru / internal_issued_by_ru values from
// every tourist in the group, deduplicated by the caller (matching the
// pattern used by collectFreeText / TranslateStrings). YandexGPT
// reformats each string per the rules embedded in the system prompt.
//
// PII NOTE: addresses and issuing-authority fields qualify as PII and
// previously stayed on the server (formatted by the now-removed
// backend/internal/format package). Routing them through YandexGPT is
// acceptable per 152-ФЗ ONLY because YandexGPT processes data inside
// the Russian Federation — there is no cross-border transfer. The
// audit log records each call via yandexGPTAdapter as
// Provider="yandex-gpt" so we retain a full trace of what was sent.
//
// Empty / nil input → nil output, no API call. The result slice is
// exactly the same length as the input.
func CleanDoverenostFields(ctx context.Context, t Translator, src []string) ([]string, error) {
	if len(src) == 0 {
		return nil, nil
	}
	if t == nil {
		return nil, fmt.Errorf("doverenost clean: nil translator")
	}
	ctx = WithFunctionName(ctx, "doverenost_clean")

	userBody, err := json.Marshal(map[string]any{"strings": src})
	if err != nil {
		return nil, fmt.Errorf("marshal doverenost input: %w", err)
	}

	raw, err := t.Chat(ctx, yandex.ChatRequest{
		System:      doverenostCleanSystemPrompt,
		User:        string(userBody),
		Temperature: 0,
		MaxTokens:   2048,
		JSONOutput:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("doverenost clean yandex call: %w", err)
	}

	js := extractJSON(raw)
	var out []string
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return nil, fmt.Errorf("doverenost clean decode array: %w — raw: %s", err, raw)
	}
	if len(out) != len(src) {
		return nil, fmt.Errorf("doverenost clean length mismatch: got %d, want %d — raw: %s", len(out), len(src), raw)
	}
	return out, nil
}
