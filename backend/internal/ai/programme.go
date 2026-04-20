package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// FlightBrief is the minimal flight info needed for the programme.
type FlightBrief struct {
	Number  string `json:"flight,omitempty"`
	Time    string `json:"time,omitempty"`
	Airport string `json:"airport,omitempty"`
}

// HotelBrief is the minimal hotel info needed for the programme.
type HotelBrief struct {
	Name     string `json:"name"`
	City     string `json:"city"`
	Address  string `json:"address,omitempty"`
	Phone    string `json:"phone,omitempty"`
	CheckIn  string `json:"check_in"`
	CheckOut string `json:"check_out"`
}

// ProgrammeInput is the full payload sent to Claude Opus for the
// itinerary. No tourist PII — only dates, hotels, one contact phone.
type ProgrammeInput struct {
	ArrivalDate     string       `json:"arrival_date"`
	DepartureDate   string       `json:"departure_date,omitempty"`
	ArrivalFlight   FlightBrief  `json:"arrival_flight"`
	DepartureFlight FlightBrief  `json:"departure_flight,omitempty"`
	Hotels          []HotelBrief `json:"hotels"`
	ContactPhone    string       `json:"contact_phone"`
}

// ProgrammeDay is one row of the programme table.
type ProgrammeDay struct {
	Date          string `json:"date"`
	Activity      string `json:"activity"`
	Contact       string `json:"contact"`
	Accommodation string `json:"accommodation"`
}

const programmeSystemPrompt = `You are a Japanese travel programme builder for FujiTravel (a Moscow-based visa agency). Given trip data, produce the day-by-day programme as a JSON array. Return ONLY the JSON array — no markdown, no prose.

DATE FORMAT (non-standard — do NOT "fix" it):
  YYYY-DD-MM (examples: "2026-25-04" = 25 April 2026, "2026-01-05" = 1 May 2026)

Cover every calendar day from arrival_date to departure_date inclusive. If departure_date is empty (one-way ticket), cover every day up to and including the last hotel check_out date.

CELL SEPARATOR — CRITICAL:
Use "\n\n" (double newline) between every logical section. NEVER single "\n" between separate items.
  Wrong:  "Arrival\nCheck in\nRest in Hotel"
  Right:  "Arrival\n\nCheck in\n\nRest in Hotel"

ACTIVITY per day type:

Arrival day (arrival_date):
  "Arrival\n\n{HH:MM}\n{AIRPORT IN CAPS}\n{FLIGHT NUMBER}\n\nCheck in\n\nRest in Hotel"

Regular sightseeing day:
  "{Place1}\n\n{Place2}\n\n{Place3}"
  Rules: 3–4 places max; geographically close to hotel city; no duplicate sights anywhere; no sightseeing on arrival/transfer/departure days.

Transfer day (check-out one hotel, check-in another):
  "Check out\n\nTransfer to {City}\n\nCheck in"
  May add 1–2 sights ONLY if clearly on the transfer route.

Departure day (departure_date, if present):
  "Check out\n\nDeparture : {HH:MM}\n\n{AIRPORT IN CAPS}\n\n{FLIGHT NUMBER}"

One-way last day (no departure flight):
  "Free day in {City}"

CONTACT column:
  Row 1 (arrival day): contact_phone exactly as given.
  All other rows: "Same as above"

ACCOMMODATION column:
  First row of each hotel stay: "{Hotel Name}\n\n{Address}\n\n{Phone}"
  Subsequent rows of SAME hotel: "Same as above"
  Transfer day: show the NEW hotel (being checked INTO).

HOTEL DATE LOGIC:
  A hotel checked in on X and out on Y covers nights X, X+1 ... Y-1.
  On date Y that is check-out of A AND check-in of B → show hotel B.

OUTPUT SCHEMA (array only, no wrapper):
[ { "date": "YYYY-DD-MM", "activity": "...", "contact": "...", "accommodation": "..." } ]`

// GenerateProgramme asks Claude Opus to produce the programme day rows.
func GenerateProgramme(ctx context.Context, apiKey string, in ProgrammeInput) ([]ProgrammeDay, error) {
	body, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal programme input: %w", err)
	}

	userMsg := "Trip data:\n\n```json\n" + string(body) + "\n```\n\nProduce the programme array."

	// Note: claude-opus-4-7 deprecates the `temperature` parameter; the
	// model uses its default sampling. `omitempty` on the struct field
	// would also drop `0`, but omitting the line entirely makes the
	// intent explicit.
	req := anthropicRequest{
		Model:     ModelOpusProgramme,
		MaxTokens: 4096,
		System:    programmeSystemPrompt,
		Messages: []anthropicMessage{{
			Role: "user",
			Content: []anthropicContent{
				{Type: "text", Text: userMsg},
			},
		}},
	}

	raw, err := callClaude(ctx, apiKey, req)
	if err != nil {
		return nil, fmt.Errorf("programme claude call: %w", err)
	}

	js := extractJSON(raw)
	var out []ProgrammeDay
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return nil, fmt.Errorf("programme decode: %w — raw: %s", err, raw)
	}
	return out, nil
}
