package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"fujitravel-admin/backend/internal/ai/yandex"
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

// voucherPageBreak is the marker we insert between OCR'd pages before
// handing the joined text to YandexGPT. Mirrors the ticket parser
// convention; the system prompt explicitly tells the model what the
// marker means so the contract stays self-describing.
const voucherPageBreak = "\n\n--- PAGE BREAK ---\n\n"

const voucherParseSystemPrompt = `You are a hotel-voucher parser for a Japanese visa agency.
Input is OCR'd text from a hotel-voucher scan. Multiple pages are concatenated
and separated by the marker "--- PAGE BREAK ---". A single voucher may span
multiple pages, and a single scan may include several distinct hotel stays.
Both Russian and English source vouchers are common — handle either.

Extract every hotel stay you find. Return ONLY a JSON array — no prose, no
markdown, no code fences.

OUTPUT SCHEMA:
[ { "name": "...", "city": "CITY CAPS", "address": "...", "phone": "+81 ...", "check_in": "DD.MM.YYYY", "check_out": "DD.MM.YYYY" } ]

RULES:
- name: official hotel name in English, as printed on the voucher. If the
  voucher has only a Russian name, transliterate to the standard English
  brand name when it is well-known (e.g. "Дусит Тани Киото" → "Dusit Thani
  Kyoto"); otherwise keep the original spelling.
- city: Japanese city, English CAPS (e.g. "TOKYO", "KYOTO", "OSAKA").
- address: full street address in English. First check the OCR text; if the
  voucher has no address, you may use general knowledge of well-known
  Japanese hotels to fill it in. If you genuinely do not know, use "".
- phone: international format (+81 ...). Same fallback as address — voucher
  text first, general knowledge second, "" if unknown.
- check_in / check_out: DD.MM.YYYY. The OCR text may print dates in any
  format ("25 Apr 2026", "2026-04-25", "25.04.2026", "25/04/2026", Russian
  "25 апреля 2026"); convert all of them to DD.MM.YYYY. If a date is
  unreadable use "".
- Never invent hotel names or dates. Only addresses and phones may be
  filled from general knowledge.
- Output an empty array [] if no hotel stays are extractable.`

// ParseVoucherScan extracts hotel stays from a voucher scan (PDF / JPEG /
// PNG) using a two-step Yandex pipeline:
//
//  1. Yandex Vision OCR converts the scan to plain text per page.
//  2. YandexGPT receives the joined text and emits a structured JSON array.
//
// PII (152-ФЗ): both calls stay inside RU-resident Yandex Cloud, so we do
// not redact guest names locally before calling AI — the privacy guarantee
// is provided by the residency of the provider. Two audit rows are
// produced per call (one yandex-vision, one yandex-gpt).
func ParseVoucherScan(ctx context.Context, ocr OCRRecognizer, t Translator, scan []byte, mime string) ([]VoucherHotel, error) {
	if ocr == nil {
		return nil, fmt.Errorf("voucher parse: nil ocr client")
	}
	if t == nil {
		return nil, fmt.Errorf("voucher parse: nil translator")
	}
	ctx = WithFunctionName(ctx, "voucher_parse")

	pages, err := ocr.Recognize(ctx, scan, mime)
	if err != nil {
		return nil, fmt.Errorf("voucher ocr: %w", err)
	}
	fullText := strings.Join(pages, voucherPageBreak)

	raw, err := t.Chat(ctx, yandex.ChatRequest{
		System:      voucherParseSystemPrompt,
		User:        fullText,
		Temperature: 0,
		MaxTokens:   2048,
		JSONOutput:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("voucher gpt: %w", err)
	}

	var out []VoucherHotel
	if err := json.Unmarshal([]byte(extractJSON(raw)), &out); err != nil {
		return nil, fmt.Errorf("voucher decode: %w — raw: %s", err, raw)
	}
	return out, nil
}
