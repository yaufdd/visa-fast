package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"fujitravel-admin/backend/internal/ai/yandex"
)

func TestCleanDoverenostFields_HappyPath(t *testing.T) {
	ft := &fakeTranslator{
		respond: func(req yandex.ChatRequest) (string, error) {
			// Echo a canned cleaned-up array matching the inputs.
			return `["г. Москва, ул. Митинская, д. 12, кв. 49","ОУФМС России по г. Москве"]`, nil
		},
	}
	got, err := CleanDoverenostFields(context.Background(), ft,
		[]string{"москва ул митинская д12 кв49", "ОУФМС России по г Москве"})
	if err != nil {
		t.Fatal(err)
	}
	if ft.calls != 1 {
		t.Fatalf("expected exactly 1 Chat call, got %d", ft.calls)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0] != "г. Москва, ул. Митинская, д. 12, кв. 49" {
		t.Errorf("got[0] = %q", got[0])
	}
	if got[1] != "ОУФМС России по г. Москве" {
		t.Errorf("got[1] = %q", got[1])
	}
}

func TestCleanDoverenostFields_EmptyInputNoCall(t *testing.T) {
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			t.Fatalf("Chat must not be called when input is empty")
			return "", nil
		},
	}
	got, err := CleanDoverenostFields(context.Background(), ft, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil result for nil input, got %v", got)
	}
	got, err = CleanDoverenostFields(context.Background(), ft, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil result for empty input, got %v", got)
	}
	if ft.calls != 0 {
		t.Errorf("expected 0 Chat calls for empty input, got %d", ft.calls)
	}
}

func TestCleanDoverenostFields_LengthMismatch(t *testing.T) {
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			// Two-element response for a three-element input.
			return `["A","B"]`, nil
		},
	}
	_, err := CleanDoverenostFields(context.Background(), ft,
		[]string{"раз", "два", "три"})
	if err == nil {
		t.Fatal("expected length-mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "length mismatch") {
		t.Errorf("error = %q, want substring 'length mismatch'", err)
	}
}

func TestCleanDoverenostFields_PromptIncludesRules(t *testing.T) {
	ft := &fakeTranslator{}
	if _, err := CleanDoverenostFields(context.Background(), ft,
		[]string{"г москва"}); err != nil {
		t.Fatal(err)
	}
	// The system prompt should mention each canonical abbreviation /
	// acronym set so a manager reading the audit log can verify the
	// rules in flight. We assert a representative subset rather than
	// the whole prompt to allow safe wording tweaks later.
	for _, marker := range []string{"д.", "кв.", "ул.", "г.", "р-н",
		"УФМС", "МВД", "Title Case"} {
		if !strings.Contains(ft.lastReq.System, marker) {
			t.Errorf("system prompt missing rule marker %q", marker)
		}
	}
}

func TestCleanDoverenostFields_ForwardsJSONOutputAndPromptShape(t *testing.T) {
	ft := &fakeTranslator{
		respond: func(req yandex.ChatRequest) (string, error) {
			// Echo the input as a JSON array so the length-check passes.
			var in struct {
				Strings []string `json:"strings"`
			}
			if err := json.Unmarshal([]byte(req.User), &in); err != nil {
				return "", err
			}
			b, _ := json.Marshal(in.Strings)
			return string(b), nil
		},
	}
	src := []string{"москва ул ленина 5", "оуфмс по г москве"}
	if _, err := CleanDoverenostFields(context.Background(), ft, src); err != nil {
		t.Fatal(err)
	}
	if !ft.lastReq.JSONOutput {
		t.Errorf("JSONOutput = false, want true (clean must request json mode)")
	}
	if ft.lastReq.Temperature != 0 {
		t.Errorf("Temperature = %v, want 0", ft.lastReq.Temperature)
	}
	if ft.lastReq.MaxTokens != 2048 {
		t.Errorf("MaxTokens = %d, want 2048", ft.lastReq.MaxTokens)
	}
	if ft.lastReq.System != doverenostCleanSystemPrompt {
		t.Errorf("System prompt mismatch — handlers expect doverenostCleanSystemPrompt forwarded verbatim")
	}
	// User payload is a {"strings": [...]} envelope — parse and check.
	var got struct {
		Strings []string `json:"strings"`
	}
	if err := json.Unmarshal([]byte(ft.lastReq.User), &got); err != nil {
		t.Fatalf("user payload not JSON: %v — raw: %s", err, ft.lastReq.User)
	}
	if len(got.Strings) != len(src) {
		t.Fatalf("user strings len = %d, want %d", len(got.Strings), len(src))
	}
	for i := range src {
		if got.Strings[i] != src[i] {
			t.Errorf("user.strings[%d] = %q, want %q", i, got.Strings[i], src[i])
		}
	}
}

func TestCleanDoverenostFields_NilTranslator(t *testing.T) {
	_, err := CleanDoverenostFields(context.Background(), nil,
		[]string{"г москва"})
	if err == nil {
		t.Fatal("expected error for nil translator")
	}
	if !strings.Contains(err.Error(), "nil translator") {
		t.Errorf("error = %q, want substring 'nil translator'", err)
	}
}

func TestCleanDoverenostFields_PropagatesUnderlyingError(t *testing.T) {
	wantErr := errors.New("yandex boom")
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			return "", wantErr
		},
	}
	_, err := CleanDoverenostFields(context.Background(), ft,
		[]string{"г москва"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error chain missing underlying err: %v", err)
	}
}

func TestCleanDoverenostFields_PreservesEmptyElement(t *testing.T) {
	// Sanity: contract says the function never drops or reorders
	// elements. A "" must round-trip to "" (or whatever the model
	// returns at that index) — we just verify the length is preserved.
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			return `["г. Москва","",""]`, nil
		},
	}
	got, err := CleanDoverenostFields(context.Background(), ft,
		[]string{"г москва", "", ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got len %d, want 3", len(got))
	}
	if got[0] != "г. Москва" || got[1] != "" || got[2] != "" {
		t.Errorf("got %v", got)
	}
}
