package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"fujitravel-admin/backend/internal/ai/yandex"
)

// FlightFields is one arrival or departure leg as stored in tourists.flight_data.
type FlightFields struct {
	FlightNumber string `json:"flight_number"`
	Date         string `json:"date"`    // DD.MM.YYYY
	Time         string `json:"time"`    // HH:MM
	Airport      string `json:"airport"` // e.g. "TOKYO NARITA"
}

// TicketFlights is the parser output — both legs in one struct.
type TicketFlights struct {
	Arrival   FlightFields `json:"arrival"`
	Departure FlightFields `json:"departure"`
}

// ticketPageBreak is the marker we insert between OCR'd pages before
// handing the joined text to YandexGPT. The system prompt tells the
// model what the marker means; the test suite asserts both the marker
// text and the prompt's reference to it so the contract stays
// self-describing.
const ticketPageBreak = "\n\n--- PAGE BREAK ---\n\n"

const ticketParseSystemPrompt = `You are a flight-ticket parser for a Japanese visa agency.
Input is OCR'd text from a flight ticket scan. Multiple pages are concatenated
and separated by the marker "--- PAGE BREAK ---". Treat each page as part of
the same itinerary unless the text clearly shows separate trips.

Extract the arrival leg INTO Japan and the return leg FROM Japan. Return ONLY
valid JSON matching the schema below — no prose, no markdown, no code fences.

OUTPUT SCHEMA:
{
  "arrival":   { "flight_number": "...", "date": "DD.MM.YYYY", "time": "HH:MM", "airport": "..." },
  "departure": { "flight_number": "...", "date": "DD.MM.YYYY", "time": "HH:MM", "airport": "..." }
}

RULES:
- arrival: the LAST leg that lands in Japan (for multi-leg itineraries, the final leg; date is Japan local time).
- departure: the FIRST leg leaving Japan (takeoff from Japan; date is Japan local time).
- If the ticket is strictly ONE-WAY (no return), leave all departure.* fields as "".
- Airport: use the OFFICIAL full English name. For Japanese airports use EXACTLY one of these canonical names:
    "Narita International Airport"
    "Haneda Airport"
    "Kansai International Airport"
    "Chubu Centrair International Airport"
    "Fukuoka Airport"
    "New Chitose Airport"
    "Naha Airport"
  For non-Japanese airports use the standard official English name (e.g. "Sheremetyevo International Airport").
- Flight number: uppercase Latin + digits, no spaces, e.g. "SU262", "CZ8101".
- IATA airport codes (e.g. "NRT", "HND") may appear in the OCR; map them to the canonical airport name above.
- All dates DD.MM.YYYY. All times HH:MM (24-hour).
- Never invent data. If a field is unknown or not present in the OCR, use "".`

// canonicalJPAirports maps uppercase keywords found in ticket scans to the
// canonical official English airport name that the form dropdown expects.
// Order matters: more specific keys should win over generic ones (we pick the
// longest matching key).
var canonicalJPAirports = map[string]string{
	"NARITA":              "Narita International Airport",
	"NRT":                 "Narita International Airport",
	"HANEDA":              "Haneda Airport",
	"HND":                 "Haneda Airport",
	"TOKYO INTERNATIONAL": "Haneda Airport",
	"KANSAI":              "Kansai International Airport",
	"KIX":                 "Kansai International Airport",
	"CHUBU":               "Chubu Centrair International Airport",
	"CENTRAIR":            "Chubu Centrair International Airport",
	"NGO":                 "Chubu Centrair International Airport",
	"FUKUOKA":             "Fukuoka Airport",
	"FUK":                 "Fukuoka Airport",
	"NEW CHITOSE":         "New Chitose Airport",
	"CHITOSE":             "New Chitose Airport",
	"SAPPORO":             "New Chitose Airport",
	"CTS":                 "New Chitose Airport",
	"NAHA":                "Naha Airport",
	"OKINAWA":             "Naha Airport",
	"OKA":                 "Naha Airport",
}

// NormalizeJapaneseAirport maps free-form airport strings (e.g. "TOKYO NARITA",
// "NRT", "Narita Intl") to the canonical official name used by the form
// dropdown. Unknown airports are returned unchanged so non-Japanese entries
// (Sheremetyevo etc.) are preserved.
func NormalizeJapaneseAirport(s string) string {
	up := strings.ToUpper(strings.TrimSpace(s))
	if up == "" {
		return ""
	}
	// Longest-match wins so "NEW CHITOSE" is preferred over "CHITOSE".
	var best, bestKey string
	for key, canon := range canonicalJPAirports {
		if strings.Contains(up, key) && len(key) > len(bestKey) {
			best, bestKey = canon, key
		}
	}
	if best != "" {
		return best
	}
	return s
}

// ParseTicketScan extracts flight fields from a ticket scan (PDF / JPEG /
// PNG) using a two-step Yandex pipeline:
//
//  1. Yandex Vision OCR converts the scan to plain text per page.
//  2. YandexGPT receives the joined text and emits structured JSON.
//
// PII (152-ФЗ): both calls stay inside RU-resident Yandex Cloud, so we no
// longer redact passenger names locally before calling AI — the privacy
// guarantee is provided by the residency of the provider rather than by
// the on-prem redactor that the Anthropic path used. Two audit rows are
// produced per call (one yandex-vision, one yandex-gpt).
func ParseTicketScan(ctx context.Context, ocr OCRRecognizer, t Translator, scan []byte, mime string) (TicketFlights, error) {
	if ocr == nil {
		return TicketFlights{}, fmt.Errorf("ticket parse: nil ocr client")
	}
	if t == nil {
		return TicketFlights{}, fmt.Errorf("ticket parse: nil translator")
	}
	ctx = WithFunctionName(ctx, "ticket_parse")

	pages, err := ocr.Recognize(ctx, scan, mime)
	if err != nil {
		return TicketFlights{}, fmt.Errorf("ticket ocr: %w", err)
	}
	fullText := strings.Join(pages, ticketPageBreak)

	raw, err := t.Chat(ctx, yandex.ChatRequest{
		System:      ticketParseSystemPrompt,
		User:        fullText,
		Temperature: 0,
		MaxTokens:   2048,
		JSONOutput:  true,
	})
	if err != nil {
		return TicketFlights{}, fmt.Errorf("ticket gpt: %w", err)
	}

	var out TicketFlights
	if err := json.Unmarshal([]byte(extractJSON(raw)), &out); err != nil {
		return TicketFlights{}, fmt.Errorf("ticket decode: %w — raw: %s", err, raw)
	}
	return out, nil
}
