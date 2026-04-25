package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"fujitravel-admin/backend/internal/ai/yandex"
)

func TestGenerateProgramme_HappyPath(t *testing.T) {
	sampleOutput := `[
		{"date":"2026-25-04","activity":"Arrival","contact":"+7","accommodation":"H1"},
		{"date":"2026-26-04","activity":"Sensoji\n\nAsakusa","contact":"Same as above","accommodation":"Same as above"}
	]`
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			return sampleOutput, nil
		},
	}

	input := ProgrammeInput{
		ArrivalDate:   "25.04.2026",
		DepartureDate: "26.04.2026",
		ArrivalFlight: FlightBrief{Number: "SU 262", Time: "12:45", Airport: "TOKYO NARITA"},
		Hotels:        []HotelBrief{{Name: "H1", City: "TOKYO", CheckIn: "25.04.2026", CheckOut: "27.04.2026"}},
		ContactPhone:  "+7",
	}
	days, err := GenerateProgramme(context.Background(), ft, input)
	if err != nil {
		t.Fatal(err)
	}
	if ft.calls != 1 {
		t.Fatalf("expected exactly 1 Chat call, got %d", ft.calls)
	}
	if len(days) != 2 {
		t.Fatalf("got %d days, want 2", len(days))
	}
	if days[0].Date != "2026-25-04" {
		t.Errorf("unexpected date: %s", days[0].Date)
	}
}

func TestGenerateProgramme_ForwardsJSONOutputAndPromptShape(t *testing.T) {
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			return `[]`, nil
		},
	}
	input := ProgrammeInput{
		ArrivalDate:  "25.04.2026",
		Hotels:       []HotelBrief{{Name: "H1", City: "TOKYO", CheckIn: "25.04.2026", CheckOut: "26.04.2026"}},
		ContactPhone: "+7-555-555-5555",
		ManagerNotes: "трансферный день без экскурсий",
	}
	if _, err := GenerateProgramme(context.Background(), ft, input); err != nil {
		t.Fatal(err)
	}

	if !ft.lastReq.JSONOutput {
		t.Errorf("JSONOutput = false, want true (programme must request json mode)")
	}
	if ft.lastReq.Temperature != 0 {
		t.Errorf("Temperature = %v, want 0", ft.lastReq.Temperature)
	}
	if ft.lastReq.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", ft.lastReq.MaxTokens)
	}
	if ft.lastReq.System != programmeSystemPrompt {
		t.Errorf("System prompt mismatch — handlers expect programmeSystemPrompt forwarded verbatim")
	}

	// Anti-hallucination guard must be embedded in the prompt.
	if !strings.Contains(programmeSystemPrompt, "STRICT FACTUALITY:") {
		t.Errorf("programmeSystemPrompt missing 'STRICT FACTUALITY:' header")
	}
	for _, want := range []string{
		"verifiable on Google Maps",
		`NEVER use hedging phrases like "if time allows"`,
		"Do NOT invent fictional districts",
	} {
		if !strings.Contains(programmeSystemPrompt, want) {
			t.Errorf("programmeSystemPrompt missing anti-hallucination snippet %q", want)
		}
	}

	// User message must contain the JSON-encoded ProgrammeInput so the
	// model sees the trip data — verify a couple of distinctive fields
	// round-trip through json.Marshal.
	for _, want := range []string{
		`"arrival_date": "25.04.2026"`,
		`"contact_phone": "+7-555-555-5555"`,
		`"name": "H1"`,
		"трансферный день без экскурсий",
	} {
		if !strings.Contains(ft.lastReq.User, want) {
			t.Errorf("user payload missing %q — got: %s", want, ft.lastReq.User)
		}
	}
}

func TestGenerateProgramme_NilTranslator(t *testing.T) {
	_, err := GenerateProgramme(context.Background(), nil, ProgrammeInput{ArrivalDate: "25.04.2026"})
	if err == nil {
		t.Fatal("expected error for nil translator")
	}
	if !strings.Contains(err.Error(), "nil translator") {
		t.Errorf("error = %q, want substring 'nil translator'", err)
	}
}

func TestGenerateProgramme_MalformedResponse(t *testing.T) {
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			// Not JSON — extractJSON returns the raw string, json.Unmarshal fails.
			return "not json at all", nil
		},
	}
	_, err := GenerateProgramme(context.Background(), ft, ProgrammeInput{ArrivalDate: "25.04.2026"})
	if err == nil {
		t.Fatal("expected decode error for malformed response")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %q, want substring 'decode'", err)
	}
}

func TestGenerateProgramme_EmptyArrayDecodes(t *testing.T) {
	// An empty JSON array is structurally valid — we surface zero rows
	// without an error so the caller can decide whether to treat it as
	// an upstream miss. Length-zero is recoverable; malformed is not.
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			return `[]`, nil
		},
	}
	out, err := GenerateProgramme(context.Background(), ft, ProgrammeInput{ArrivalDate: "25.04.2026"})
	if err != nil {
		t.Fatalf("expected no error for empty array, got %v", err)
	}
	if len(out) != 0 {
		t.Errorf("got %d rows, want 0", len(out))
	}
}

func TestGenerateProgramme_PropagatesUnderlyingError(t *testing.T) {
	wantErr := errors.New("yandex programme boom")
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			return "", wantErr
		},
	}
	_, err := GenerateProgramme(context.Background(), ft, ProgrammeInput{ArrivalDate: "25.04.2026"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error chain missing underlying err: %v", err)
	}
}

func TestGenerateProgramme_PIIContract_NoTouristFieldsInPayload(t *testing.T) {
	// Defensive check: ProgrammeInput must not have any tourist-name or
	// passport-related fields. If someone adds one, this test fails as a
	// loud reminder to verify against CLAUDE.md "AI Privacy" rules.
	b, err := json.Marshal(ProgrammeInput{})
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"name_cyr", "name_lat", "maiden_name",
		"passport_number", "passport_series",
		"date_of_birth", "birth_date",
		"home_address", "reg_address",
		"tourist_phone", "tourists",
	}
	got := strings.ToLower(string(b))
	for _, f := range forbidden {
		if strings.Contains(got, f) {
			t.Errorf("ProgrammeInput JSON contains forbidden PII field %q — verify against CLAUDE.md AI Privacy section", f)
		}
	}
}
