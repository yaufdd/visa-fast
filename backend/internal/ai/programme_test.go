package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateProgramme_HappyPath(t *testing.T) {
	sampleOutput := `[
		{"date":"2026-25-04","activity":"Arrival","contact":"+7","accommodation":"H1"},
		{"date":"2026-26-04","activity":"Sensoji\n\nAsakusa","contact":"Same as above","accommodation":"Same as above"}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != ModelOpusProgramme {
			t.Errorf("wrong model: %v", req["model"])
		}
		resp := map[string]any{
			"content": []map[string]string{{"type": "text", "text": sampleOutput}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	orig := AnthropicAPIOverride
	AnthropicAPIOverride = srv.URL
	defer func() { AnthropicAPIOverride = orig }()

	input := ProgrammeInput{
		ArrivalDate:   "25.04.2026",
		DepartureDate: "26.04.2026",
		ArrivalFlight: FlightBrief{Number: "SU 262", Time: "12:45", Airport: "TOKYO NARITA"},
		Hotels:        []HotelBrief{{Name: "H1", City: "TOKYO", CheckIn: "25.04.2026", CheckOut: "27.04.2026"}},
		ContactPhone:  "+7",
	}
	days, err := GenerateProgramme(context.Background(), "test", input)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 2 {
		t.Fatalf("got %d days, want 2", len(days))
	}
	if days[0].Date != "2026-25-04" {
		t.Errorf("unexpected date: %s", days[0].Date)
	}
}
