package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"fujitravel-admin/backend/internal/ai/yandex"
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

// ProgrammeInput is the full payload sent to YandexGPT for the
// itinerary. No tourist PII — only dates, hotels, one contact phone.
// The single contact_phone is a guide phone (already on the no-PII
// side per CLAUDE.md "AI Privacy" section), not a tourist phone.
type ProgrammeInput struct {
	ArrivalDate     string       `json:"arrival_date"`
	DepartureDate   string       `json:"departure_date,omitempty"`
	ArrivalFlight   FlightBrief  `json:"arrival_flight"`
	DepartureFlight FlightBrief  `json:"departure_flight,omitempty"`
	Hotels          []HotelBrief `json:"hotels"`
	ContactPhone    string       `json:"contact_phone"`
	// ManagerNotes are free-form Russian hints from the travel-agency
	// manager (e.g. "3 марта — чайная церемония", "trying not to walk much").
	// The AI must honour them over the default activity rules wherever they
	// don't conflict with the strict schema constraints below.
	ManagerNotes string `json:"manager_notes,omitempty"`
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
  "Check out\n\nTransfer to {City}\n\n{Place1}\n\n{Place2}\n\nCheck in"
  Rules:
    - You MUST add 1–2 sightseeing places — pick sights in the destination
      city (or along the transfer route) that make logistical sense given
      luggage/travel time. Never leave the day as just
      "Check out + Transfer + Check in".
    - The sights must not duplicate any sightseeing shown on other days of
      the same itinerary.
    - The ONLY exception is when manager_notes explicitly instruct
      otherwise (e.g. "трансферный день без экскурсий" / "no sightseeing
      on transfer days"). Only in that case emit the short form
      "Check out\n\nTransfer to {City}\n\nCheck in".

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

MANAGER NOTES (manager_notes field on the input):
  Free-form Russian hints written by the travel-agency manager. When present
  they describe concrete wishes (specific sights, themes, exceptions) for
  particular days or the whole trip. Treat them as high-priority
  instructions: incorporate them into the relevant day's activity cell.
  Honour them OVER the default activity rules wherever the two conflict,
  EXCEPT these hard constraints that are never negotiable:
    - CELL SEPARATOR ("\n\n") and DATE FORMAT (YYYY-DD-MM).
    - ACCOMMODATION and CONTACT column rules.
    - No duplicate sights anywhere in the itinerary.
  If manager_notes are empty or absent, generate the programme from the
  default rules alone.

OUTPUT SCHEMA (array only, no wrapper):
[ { "date": "YYYY-DD-MM", "activity": "...", "contact": "...", "accommodation": "..." } ]

STRICT FACTUALITY:
- Every place name MUST be a real, well-known landmark verifiable on Google Maps.
- If even slightly unsure, choose a more famous alternative.
- NEVER use hedging phrases like "if time allows" — those are red flags that you are guessing. Output places with full confidence or do not output them at all.
- Do NOT invent fictional districts, neighborhoods, or attractions. If a city has fewer than 3 famous landmarks for a given day, prefer to repeat a generic activity over fabricating a place.`

// GenerateProgramme asks YandexGPT to produce the programme day rows.
//
// PII contract: ProgrammeInput carries only dates, flights, hotels, the
// guide contact phone, and free-form manager notes — none of which are
// classified as PII per CLAUDE.md "AI Privacy" section. Tourist names,
// passport data, dates of birth, home/registration addresses and tourist
// phones must NOT be added to ProgrammeInput; the audit log records the
// full system + user payload verbatim, so leakage here would breach the
// 152-ФЗ contract documented on yandexGPTAdapter.
func GenerateProgramme(ctx context.Context, t Translator, in ProgrammeInput) ([]ProgrammeDay, error) {
	if t == nil {
		return nil, fmt.Errorf("programme: nil translator")
	}
	ctx = WithFunctionName(ctx, "programme")
	body, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal programme input: %w", err)
	}

	userMsg := "Trip data:\n\n```json\n" + string(body) + "\n```\n\nProduce the programme array."

	raw, err := t.Chat(ctx, yandex.ChatRequest{
		System:      programmeSystemPrompt,
		User:        userMsg,
		MaxTokens:   4096,
		Temperature: 0,
		JSONOutput:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("programme yandex call: %w", err)
	}

	js := extractJSON(raw)
	var out []ProgrammeDay
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return nil, fmt.Errorf("programme decode: %w — raw: %s", err, raw)
	}
	return out, nil
}
