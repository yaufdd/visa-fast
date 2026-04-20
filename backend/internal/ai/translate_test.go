package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslateStrings_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		// model + temperature check (sanity)
		if req["model"] != ModelHaikuTranslate {
			t.Errorf("expected model %s, got %v", ModelHaikuTranslate, req["model"])
		}
		resp := map[string]any{
			"content": []map[string]string{
				{"type": "text", "text": `["Director of Development","LLC Romashka"]`},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	orig := AnthropicAPIOverride
	AnthropicAPIOverride = srv.URL
	defer func() { AnthropicAPIOverride = orig }()

	got, err := TranslateStrings(context.Background(), "test-key",
		[]string{"Директор по развитию", "ООО Ромашка"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0] != "Director of Development" {
		t.Errorf("got[0] = %q", got[0])
	}
	if !strings.Contains(got[1], "Romashka") {
		t.Errorf("got[1] = %q", got[1])
	}
}

func TestTranslateStrings_Empty(t *testing.T) {
	got, err := TranslateStrings(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil result for empty input, got %v", got)
	}
}
