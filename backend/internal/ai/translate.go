package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"fujitravel-admin/backend/internal/ai/yandex"
)

const translateSystemPrompt = `You are a Russian → English translator for Japanese visa documents. Translate each string to natural English. For proper names (companies, people, street names), transliterate using the standard Russian → Latin system. For descriptive words (job titles, address parts), translate them fully. Return ONLY a JSON array of translations, same length and order as the input array. No markdown fences, no prose.

Examples:
- "Директор по развитию" → "Director of Development"
- "ООО Ромашка" → "LLC Romashka"
- "Москва, ул. Ленина 5, кв. 12" → "Moscow, Lenin St. 5, Apt. 12"
- "ИП Иванов Петр" → "IE Ivanov Petr"
- "ОУФМС России по г. Москве" → "Federal Migration Service in Moscow"
- "МВД 77810" → "MVD 77810"
- "СССР" → "USSR"
- "январь 2020" → "January 2020"`

// Translator abstracts a YandexGPT-shaped chat call. Implemented by
// *yandex.GPTClient (production) and by the test fakes in
// translate_test.go. Keeping this interface inside the ai package lets
// callers (api/generate.go) depend on a small, mockable surface instead
// of pulling the full yandex package into their dependency graph.
type Translator interface {
	Chat(ctx context.Context, req yandex.ChatRequest) (string, error)
}

// TranslateStrings sends a batch of Russian strings to YandexGPT for
// English translation. Nil or empty input → nil output, no API call.
// The result slice is exactly the same length as the input.
//
// The translator parameter is the only seam that touches Yandex —
// production code passes a *yandexGPTAdapter (which wraps a real
// *yandex.GPTClient and writes an audit-log row per call); tests pass
// a small struct returning a canned response.
func TranslateStrings(ctx context.Context, t Translator, src []string) ([]string, error) {
	if len(src) == 0 {
		return nil, nil
	}
	if t == nil {
		return nil, fmt.Errorf("translate: nil translator")
	}
	ctx = WithFunctionName(ctx, "translate")
	userBody, err := json.Marshal(map[string]any{"strings": src})
	if err != nil {
		return nil, fmt.Errorf("marshal translate input: %w", err)
	}

	raw, err := t.Chat(ctx, yandex.ChatRequest{
		System:      translateSystemPrompt,
		User:        string(userBody),
		Temperature: 0,
		MaxTokens:   2048,
		JSONOutput:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("translate yandex call: %w", err)
	}

	js := extractJSON(raw)
	var out []string
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return nil, fmt.Errorf("translate decode array: %w — raw: %s", err, raw)
	}
	if len(out) != len(src) {
		return nil, fmt.Errorf("translate length mismatch: got %d, want %d — raw: %s", len(out), len(src), raw)
	}
	return out, nil
}
