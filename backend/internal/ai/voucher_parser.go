package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// VoucherHotel is one hotel extracted from a voucher.
type VoucherHotel struct {
	Name     string `json:"name"`
	City     string `json:"city"`
	Address  string `json:"address"`
	Phone    string `json:"phone"`
	CheckIn  string `json:"check_in"`  // DD.MM.YYYY
	CheckOut string `json:"check_out"` // DD.MM.YYYY
}

const voucherSystemPrompt = `You are a hotel voucher parser for a Japanese visa agency. Given voucher scans, extract every hotel stay found. Return ONLY a JSON array — no markdown, no prose.

SCHEMA:
[ { "name": "...", "city": "CITY CAPS", "address": "...", "phone": "+81 ...", "check_in": "DD.MM.YYYY", "check_out": "DD.MM.YYYY" } ]

RULES:
- name: official hotel name in English (as on the voucher).
- city: Japanese city, English CAPS (e.g. "TOKYO", "KYOTO", "OSAKA").
- address: full street address in English. First check the voucher; if the voucher has no address, use your general knowledge of well-known Japanese hotels (e.g. "Dusit Thani Kyoto" → official address). If you genuinely do not know, use "".
- phone: international format (+81 ...). Same fallback as address.
- Dates DD.MM.YYYY.
- Do not invent names or dates. Only addresses/phones may be filled from general knowledge.`

// ParseVouchers returns the hotels found across all voucher files.
func ParseVouchers(ctx context.Context, apiKey string, files []FileInput) ([]VoucherHotel, error) {
	contents, err := buildFileContents(files)
	if err != nil {
		return nil, err
	}
	contents = append(contents, anthropicContent{
		Type: "text",
		Text: "Extract every hotel stay from the voucher(s) above per the schema.",
	})

	req := anthropicRequest{
		Model:       ModelOpusParser,
		MaxTokens:   2048,
		Temperature: 0,
		System:      voucherSystemPrompt,
		Messages:    []anthropicMessage{{Role: "user", Content: contents}},
	}
	raw, err := callClaude(ctx, apiKey, req)
	if err != nil {
		return nil, fmt.Errorf("voucher parse call: %w", err)
	}
	var out []VoucherHotel
	if err := json.Unmarshal([]byte(extractJSON(raw)), &out); err != nil {
		return nil, fmt.Errorf("voucher parse decode: %w — raw: %s", err, raw)
	}
	return out, nil
}
