package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
)

const (
	claudeModel  = "claude-opus-4-6"
	anthropicAPI = "https://api.anthropic.com/v1/messages"
)

// Pass1Result holds structured data extracted from travel documents.
// All string values are in ENGLISH / Latin script unless noted.
type Pass1Result struct {
	// Name from ticket (SURNAME FIRSTNAME, uppercase Latin).
	// If a ticket is present, it takes priority over the passport latin name.
	NameLat string `json:"name_lat"`
	// Name in Cyrillic as written in the Russian internal passport.
	NameCyr string `json:"name_cyr"`

	// Foreign passport fields.
	PassportNumber  string `json:"passport_number"`
	BirthDate       string `json:"birth_date"`    // DD.MM.YYYY
	Nationality     string `json:"nationality"`   // full English name UPPERCASE, e.g. "RUSSIA"
	PlaceOfBirth    string `json:"place_of_birth"` // city, country
	IssueDate       string `json:"issue_date"`    // DD.MM.YYYY
	ExpiryDate      string `json:"expiry_date"`   // DD.MM.YYYY
	FormerNat       string `json:"former_nationality"` // "USSR" or "NO"
	Gender          string `json:"gender"`         // "M" or "F"
	PassportType    string `json:"passport_type"`  // "Ordinary" | "Diplomatic" | "Official" | "Other"
	IssuedBy        string `json:"issued_by"`

	// Russian internal passport fields (for the доверенность).
	InternalSeries  string `json:"internal_series"`
	InternalNumber  string `json:"internal_number"`
	InternalIssued  string `json:"internal_issued"`   // DD.MM.YYYY
	InternalIssuedBy string `json:"internal_issued_by"`
	RegAddress      string `json:"reg_address"`

	// Flight information — last leg INTO Japan (or only leg if direct).
	FlightNumber     string `json:"flight_number"`
	ArrivalTime      string `json:"arrival_time"`    // HH:MM local Japan time
	ArrivalAirport   string `json:"arrival_airport"` // e.g. "OSAKA KANSAI"
	ArrivalDate      string `json:"arrival_date"`    // DD.MM.YYYY — Japan arrival date
	DepartureTime    string `json:"departure_time"`  // HH:MM
	DepartureAirport string `json:"departure_airport"` // e.g. "MOSCOW SHEREMETYEVO"
	DepartureDate    string `json:"departure_date"`  // DD.MM.YYYY — departure from Russia

	// Hotels from vouchers (if any voucher files are provided).
	HotelsFromVouchers []VoucherHotel `json:"hotels_from_vouchers"`
}

// VoucherHotel is a hotel extracted from a voucher document.
type VoucherHotel struct {
	Name    string `json:"name"`
	CheckIn string `json:"checkin"`  // DD.MM.YYYY
	CheckOut string `json:"checkout"` // DD.MM.YYYY
}

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	System      string             `json:"system"`
	Messages    []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string            `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type   string          `json:"type"`
	Text   string          `json:"text,omitempty"`
	Source *contentSource  `json:"source,omitempty"`
}

type contentSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	FileID    string `json:"file_id,omitempty"` // Anthropic Files API reference
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// FileInput represents a single file to send to Claude.
// If AnthropicFileID is set, it is used directly (Files API reference).
// Otherwise, Name + Data are base64-encoded inline.
type FileInput struct {
	AnthropicFileID string // preferred: Anthropic Files API file_id
	Name            string // filename (used to determine MIME type for inline)
	Data            []byte // raw bytes (used when AnthropicFileID is empty)
}

// ParseDocuments sends the provided files to Claude Pass 1 and returns
// extracted tourist data as structured JSON. Returns one result per tourist
// found in the documents.
//
// Each FileInput can either carry an Anthropic file_id (no size limit) or
// raw bytes (inline base64). Mixed inputs are supported.
func ParseDocuments(ctx context.Context, apiKey string, inputs []FileInput, notes ...string) ([]Pass1Result, error) {
	extraNote := ""
	if len(notes) > 0 && notes[0] != "" {
		extraNote = "\n\n=== MANAGER NOTES ===\n" + notes[0] + "\nUse these notes to resolve ambiguities in the documents."
	}

	system := `You are a travel document parser for a Japanese visa agency. Documents for one or more tourists may be provided together. Extract structured data for EVERY tourist found. Return ONLY a valid JSON array — one object per tourist. No markdown fences, no explanation, nothing outside the JSON array.

=== OUTPUT SCHEMA ===

[
  {
    "name_lat": "SURNAME FIRSTNAME",
    "name_cyr": "Фамилия Имя",
    "passport_number": "...",
    "birth_date": "DD.MM.YYYY",
    "nationality": "...",
    "place_of_birth": "...",
    "issue_date": "DD.MM.YYYY",
    "expiry_date": "DD.MM.YYYY",
    "former_nationality": "...",
    "gender": "M",
    "passport_type": "Ordinary",
    "issued_by": "...",
    "internal_series": "...",
    "internal_number": "...",
    "internal_issued": "DD.MM.YYYY",
    "internal_issued_by": "...",
    "reg_address": "...",
    "flight_number": "...",
    "arrival_time": "HH:MM",
    "arrival_airport": "...",
    "arrival_date": "DD.MM.YYYY",
    "departure_time": "HH:MM",
    "departure_airport": "...",
    "departure_date": "DD.MM.YYYY",
    "hotels_from_vouchers": []
  }
]

If only one tourist is found, still return a single-element array.
Flight and hotel voucher data is shared — apply the same flight/hotel data to all tourists in the group.

=== FIELD RULES ===

name_lat (CRITICAL — read this carefully):
- If a FLIGHT TICKET is present, take the passenger name from the ticket. It is usually printed as SURNAME/FIRSTNAME or SURNAME/FIRSTNAME SUFFIX.
  - Replace "/" with a space: "IVANOV/IVAN" → "IVANOV IVAN"
  - Strip honorific suffixes: MR, MRS, MS, DR (case-insensitive)
  - Result must be UPPERCASE Latin: "IVANOV IVAN"
- If NO ticket is present, take the name from the foreign passport latin field.
- Always UPPERCASE.

name_cyr:
- Take from the Russian internal (domestic) passport if provided.
- Format: "Фамилия Имя" (NO patronymic/otchestvo).
- If only the foreign passport is provided and it has no Cyrillic, leave as "".

passport_number / issue_date / expiry_date / issued_by / place_of_birth / birth_date / gender / passport_type:
- Take from the FOREIGN (international) passport.
- gender: "M" for male, "F" for female.
- passport_type: "Ordinary" for most Russian passports; "Diplomatic" / "Official" / "Other" only if explicitly stated.
- All dates: DD.MM.YYYY.
- place_of_birth: city and country in ENGLISH. If printed in Cyrillic on the passport, transliterate or translate to English (e.g. "МОСКВА, СССР" → "MOSCOW, USSR", "АЛМАТЫ, КАЗАХСТАН" → "ALMATY, KAZAKHSTAN").

nationality:
- Full English country name in ALL CAPS.
- Examples: "RUSSIA" (never "RUS"), "KAZAKHSTAN", "UKRAINE".
- Read from the passport nationality field.

former_nationality (three-step logic — follow in order):
  STEP 1: Does the document EXPLICITLY state a former nationality as "USSR" or "SOVIET UNION"? → "USSR"
  STEP 2: Is former_nationality NOT stated, BUT place_of_birth contains "USSR" or "SOVIET UNION"? → "USSR"
  STEP 3: Is former_nationality NOT stated AND place_of_birth does NOT mention USSR? → "NO"

issued_by:
- Take from the FOREIGN passport. Translate to English/Latin. "МВД 77533" → "MVD 77533", "МВД 77810, Москва" → "MVD 77810, Moscow".

internal_series / internal_number / internal_issued / internal_issued_by / reg_address:
- Take from the Russian INTERNAL (domestic) passport (серия, номер, кем выдан, дата выдачи, адрес регистрации).
- internal_series: 4-digit series (e.g. "4521").
- internal_number: 6-digit number (e.g. "120035").
- internal_issued_by and reg_address may remain in Russian Cyrillic (used in доверенность, not the Japanese form).
- If no internal passport is provided, all internal_* fields = "".

flight_number / arrival_time / arrival_airport / arrival_date:
- For multi-leg itineraries (e.g. Moscow → Shanghai → Osaka): use the LAST leg that arrives IN JAPAN.
- arrival_airport: city + airport name in CAPS, e.g. "OSAKA KANSAI", "TOKYO NARITA".
- arrival_date: the calendar date of Japan arrival (may differ from Russia departure date if overnight flight).

departure_time / departure_airport / departure_date:
- The outbound flight LEAVING RUSSIA (first leg of the outbound journey).
- departure_airport: city + airport name in CAPS, e.g. "MOSCOW SHEREMETYEVO".

hotels_from_vouchers:
- Array of objects, one per hotel found in any voucher document.
- Each object: { "name": "Hotel Name", "checkin": "DD.MM.YYYY", "checkout": "DD.MM.YYYY" }
- If no vouchers provided: [].

=== MISSING DATA ===
- If a field cannot be found in any of the provided documents, use "" (empty string).
- Never invent data. Never guess. Never fill from general knowledge.
- Exception: former_nationality must always be "USSR" or "NO" (never "").` + extraNote

	var contents []anthropicContent

	for _, inp := range inputs {
		if inp.AnthropicFileID != "" {
			// Use Anthropic Files API reference — no size limit.
			ext := strings.ToLower(filepath.Ext(inp.Name))
			blockType := "document"
			if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
				blockType = "image"
			}
			contents = append(contents, anthropicContent{
				Type: blockType,
				Source: &contentSource{
					Type:   "file",
					FileID: inp.AnthropicFileID,
				},
			})
			continue
		}

		// Fallback: inline base64.
		ext := strings.ToLower(filepath.Ext(inp.Name))
		switch ext {
		case ".jpg", ".jpeg":
			contents = append(contents, anthropicContent{
				Type: "image",
				Source: &contentSource{
					Type:      "base64",
					MediaType: "image/jpeg",
					Data:      base64.StdEncoding.EncodeToString(inp.Data),
				},
			})
		case ".png":
			contents = append(contents, anthropicContent{
				Type: "image",
				Source: &contentSource{
					Type:      "base64",
					MediaType: "image/png",
					Data:      base64.StdEncoding.EncodeToString(inp.Data),
				},
			})
		case ".pdf":
			contents = append(contents, anthropicContent{
				Type: "document",
				Source: &contentSource{
					Type:      "base64",
					MediaType: "application/pdf",
					Data:      base64.StdEncoding.EncodeToString(inp.Data),
				},
			})
		default:
			contents = append(contents, anthropicContent{
				Type: "text",
				Text: fmt.Sprintf("File: %s\n\n%s", inp.Name, string(inp.Data)),
			})
		}
	}

	contents = append(contents, anthropicContent{
		Type: "text",
		Text: "Extract data for ALL tourists found in the documents above. Return a JSON array with one object per tourist.",
	})

	reqBody := anthropicRequest{
		Model:       claudeModel,
		MaxTokens:   8192,
		Temperature: 0, // deterministic extraction
		System:      system,
		Messages: []anthropicMessage{
			{Role: "user", Content: contents},
		},
	}

	raw, err := callClaude(ctx, apiKey, reqBody)
	if err != nil {
		return nil, err
	}

	// Response is a JSON array. Strip any prose before/after.
	s := strings.TrimSpace(raw)
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start == -1 || end == -1 || end < start {
		// Fallback: maybe model returned a single object — wrap it.
		jsonObj := extractJSON(s)
		var single Pass1Result
		if err := json.Unmarshal([]byte(jsonObj), &single); err != nil {
			return nil, fmt.Errorf("unmarshal pass1 response: %w — raw: %s", err, raw)
		}
		return []Pass1Result{single}, nil
	}
	jsonArr := s[start : end+1]
	var results []Pass1Result
	if err := json.Unmarshal([]byte(jsonArr), &results); err != nil {
		return nil, fmt.Errorf("unmarshal pass1 array: %w — raw: %s", err, raw)
	}
	return results, nil
}

// extractJSON finds the first '{' ... last '}' in s, stripping any surrounding
// prose that Claude sometimes emits before or after the JSON object.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return s
	}
	return s[start : end+1]
}

// callClaude performs the HTTP request to the Anthropic API and returns the
// first text content block from the response.
func callClaude(ctx context.Context, apiKey string, reqBody anthropicRequest) (string, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal claude request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPI, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("build claude request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "files-api-2025-04-14")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("claude http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read claude response: %w", err)
	}

	var ar anthropicResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return "", fmt.Errorf("unmarshal claude response: %w — body: %s", err, body)
	}
	if ar.Error != nil {
		return "", fmt.Errorf("claude API error: %s", ar.Error.Message)
	}
	if len(ar.Content) == 0 {
		return "", fmt.Errorf("claude returned empty content")
	}
	return ar.Content[0].Text, nil
}
