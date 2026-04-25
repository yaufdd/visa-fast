package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"fujitravel-admin/backend/internal/ai/yandex"
)

// PassportType discriminates the two flavours of Russian passport that the
// public form may upload. Internal = general-civil ("внутренний"); foreign =
// travel passport ("заграничный"). Each type uses a dedicated system prompt
// because the field set, MRZ format, and address presence all differ.
type PassportType string

const (
	PassportInternal PassportType = "internal"
	PassportForeign  PassportType = "foreign"
)

// PassportFields is the structured output from a passport scan. Fields are
// populated as the OCR text and GPT extractor allow; missing data stays as
// the zero value (empty string) — never fabricated. Type is set
// deterministically by ParsePassportScan after decode so the model cannot
// override the caller's intent.
type PassportFields struct {
	Type           PassportType `json:"type"`
	Series         string       `json:"series"`           // "4523"
	Number         string       `json:"number"`           // "172344" or "1234567"
	LastName       string       `json:"last_name"`        // "БАМБА"
	FirstName      string       `json:"first_name"`       // "ЭРИК"
	Patronymic     string       `json:"patronymic"`       // optional
	Gender         string       `json:"gender"`           // "МУЖ" | "ЖЕН"
	BirthDate      string       `json:"birth_date"`       // YYYY-MM-DD
	PlaceOfBirth   string       `json:"place_of_birth"`
	IssueDate      string       `json:"issue_date"`       // YYYY-MM-DD
	ExpiryDate     string       `json:"expiry_date"`      // foreign passports only
	IssuingAuthor  string       `json:"issuing_authority"`
	DepartmentCode string       `json:"department_code"`  // "770-091"
	RegAddress     string       `json:"reg_address"`      // internal passport page 2
	NameLatin      string       `json:"name_latin"`       // foreign passport MRZ
}

// passportPageBreak mirrors the convention used by ticket_parser.go and
// voucher_parser.go — the OCR returns one string per page, we join with
// this marker, and the system prompt explicitly tells the model what the
// marker means so the contract stays self-describing.
const passportPageBreak = "\n\n--- PAGE BREAK ---\n\n"

const passportInternalSystemPrompt = `You parse OCR'd text from a Russian internal civil passport. The OCR text may be split across pages by ` + "`" + `--- PAGE BREAK ---` + "`" + ` markers (page 1 = main spread, page 2 = registration). Extract these fields and return ONLY a JSON object matching the schema. No prose.

Fields to extract:
- series: 4 digits (e.g. "4523")
- number: 6 digits (e.g. "172344"), often shown as "45 23 172344"
- last_name, first_name, patronymic (Russian, UPPERCASE preserved)
- gender: "МУЖ" or "ЖЕН"
- birth_date: YYYY-MM-DD (parse from "ДД.ММ.ГГГГ")
- place_of_birth: the city/region as written
- issue_date: YYYY-MM-DD (cross-validate against MRZ if OCR mis-read)
- issuing_authority: full text of the organization that issued (e.g. "ГУ МВД РОССИИ ПО Г.МОСКВЕ")
- department_code: format XXX-XXX (e.g. "770-091")
- reg_address: full registration address from page 2 (e.g. "Г. МОСКВА, УЛ. МИТИНСКАЯ, Д. 12, КВ. 49")
- expiry_date: ALWAYS empty for internal passports
- name_latin: ALWAYS empty for internal passports

CROSS-VALIDATION:
- The MRZ at the bottom of page 1 contains structured data. If your text-extracted issue_date or birth_date conflicts with the MRZ-encoded version, prefer the MRZ.
- If OCR clearly mis-read a digit ("96.10.2022" but MRZ says "221006"), trust the MRZ to fix it.

If a field is genuinely missing from the OCR text, output empty string. NEVER fabricate.`

const passportForeignSystemPrompt = `You parse OCR'd text from a Russian foreign (travel) passport. Output ONLY JSON matching the schema. No prose.

Fields:
- series: empty string (foreign passports use a single 9-digit number, no series)
- number: 9 characters as printed (e.g. "751234567"). NOT just digits — preserve any letter prefix.
- last_name, first_name, patronymic (Cyrillic, as printed)
- name_latin: the Latin transliteration of "LASTNAME LASTNAME FIRSTNAME" or just "LASTNAME FIRSTNAME" as printed on the data page (NOT the MRZ — the human-readable version)
- gender: "МУЖ" or "ЖЕН"
- birth_date: YYYY-MM-DD
- place_of_birth
- issue_date: YYYY-MM-DD
- expiry_date: YYYY-MM-DD (foreign passports always have one — usually 5 or 10 years from issue)
- issuing_authority: typically "ФМС XXXXX" or "MID XXXXX"
- department_code: e.g. "77810"
- reg_address: empty string (foreign passports do not contain registration)

CROSS-VALIDATION: prefer MRZ values when conflicting with text-extracted fields, especially for dates and the document number.

If a field is genuinely missing, output empty string. NEVER fabricate.`

// ParsePassportScan extracts structured fields from a scanned passport (PDF /
// JPEG / PNG) using the same two-step Yandex pipeline as ParseTicketScan and
// ParseVoucherScan:
//
//  1. Yandex Vision OCR converts the scan to plain text per page (joined
//     with the documented PAGE BREAK marker so the extractor knows where
//     one page ends and the next begins).
//  2. YandexGPT receives the joined text + a passport-type-specific system
//     prompt and emits structured JSON.
//
// pType selects between PassportInternal (general-civil, 2-page main spread
// + registration) and PassportForeign (travel passport with MRZ). The Type
// field on the returned struct is always set from pType after decode so the
// model cannot override the caller's intent.
//
// PII (152-ФЗ): both calls stay inside RU-resident Yandex Cloud, so no
// local redaction is needed — the privacy guarantee is provided by the
// residency of the provider. Two audit rows are produced per call (one
// yandex-vision, one yandex-gpt) via the existing adapters.
func ParsePassportScan(ctx context.Context, ocr OCRRecognizer, t Translator, scan []byte, mime string, pType PassportType) (PassportFields, error) {
	if ocr == nil {
		return PassportFields{}, fmt.Errorf("passport parse: nil ocr client")
	}
	if t == nil {
		return PassportFields{}, fmt.Errorf("passport parse: nil translator")
	}
	ctx = WithFunctionName(ctx, "passport_parse")

	var systemPrompt string
	switch pType {
	case PassportInternal:
		systemPrompt = passportInternalSystemPrompt
	case PassportForeign:
		systemPrompt = passportForeignSystemPrompt
	default:
		return PassportFields{}, fmt.Errorf("passport: unknown type %q", pType)
	}

	pages, err := ocr.Recognize(ctx, scan, mime)
	if err != nil {
		return PassportFields{}, fmt.Errorf("passport ocr: %w", err)
	}
	fullText := strings.Join(pages, passportPageBreak)

	raw, err := t.Chat(ctx, yandex.ChatRequest{
		System:      systemPrompt,
		User:        fullText,
		Temperature: 0,
		MaxTokens:   2048,
		JSONOutput:  true,
	})
	if err != nil {
		return PassportFields{}, fmt.Errorf("passport gpt: %w", err)
	}

	var out PassportFields
	if err := json.Unmarshal([]byte(extractJSON(raw)), &out); err != nil {
		return PassportFields{}, fmt.Errorf("passport decode: %w — raw: %s", err, raw)
	}
	// Authoritatively set Type from the caller's intent — whatever the model
	// returned (or didn't return) is overridden so downstream consumers can
	// trust this field unconditionally.
	out.Type = pType
	return out, nil
}
