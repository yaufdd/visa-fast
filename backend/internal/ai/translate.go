package ai

import (
	"context"
	"encoding/json"
	"fmt"
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

// TranslateStrings sends a batch of Russian strings to Claude Haiku for
// English translation. Nil or empty input → nil output, no API call.
// The result slice is exactly the same length as the input.
func TranslateStrings(ctx context.Context, apiKey string, src []string) ([]string, error) {
	if len(src) == 0 {
		return nil, nil
	}
	userBody, err := json.Marshal(map[string]any{"strings": src})
	if err != nil {
		return nil, fmt.Errorf("marshal translate input: %w", err)
	}

	req := anthropicRequest{
		Model:       ModelHaikuTranslate,
		MaxTokens:   2048,
		Temperature: 0,
		System:      translateSystemPrompt,
		Messages: []anthropicMessage{{
			Role: "user",
			Content: []anthropicContent{
				{Type: "text", Text: string(userBody)},
			},
		}},
	}

	raw, err := callClaude(ctx, apiKey, req)
	if err != nil {
		return nil, fmt.Errorf("translate claude call: %w", err)
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
