package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"fujitravel-admin/backend/internal/ai/yandex"
)

// fakeTranslator is a tiny in-process Translator used to verify the
// shape of the request TranslateStrings constructs and the way it
// interprets the response. The test sets `respond` to control the raw
// string the adapter would have read back from Yandex; nil `respond`
// means "echo the input array as a JSON array of pass-through values".
type fakeTranslator struct {
	calls   int
	lastReq yandex.ChatRequest
	respond func(req yandex.ChatRequest) (string, error)
}

func (f *fakeTranslator) Chat(_ context.Context, req yandex.ChatRequest) (string, error) {
	f.calls++
	f.lastReq = req
	if f.respond != nil {
		return f.respond(req)
	}
	// Default: parse the input JSON and echo each string back as
	// "TR:<input>" so happy-path tests get a deterministic translation.
	var in struct {
		Strings []string `json:"strings"`
	}
	if err := json.Unmarshal([]byte(req.User), &in); err != nil {
		return "", err
	}
	out := make([]string, len(in.Strings))
	for i, s := range in.Strings {
		out[i] = "TR:" + s
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

func TestTranslateStrings_HappyPath(t *testing.T) {
	ft := &fakeTranslator{
		respond: func(req yandex.ChatRequest) (string, error) {
			return `["Director of Development","LLC Romashka"]`, nil
		},
	}
	got, err := TranslateStrings(context.Background(), ft,
		[]string{"Директор по развитию", "ООО Ромашка"})
	if err != nil {
		t.Fatal(err)
	}
	if ft.calls != 1 {
		t.Fatalf("expected exactly 1 Chat call, got %d", ft.calls)
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

func TestTranslateStrings_ForwardsJSONOutputAndPromptShape(t *testing.T) {
	ft := &fakeTranslator{}
	src := []string{"Москва, ул. Ленина 5", "ИП Иванов Петр"}
	if _, err := TranslateStrings(context.Background(), ft, src); err != nil {
		t.Fatal(err)
	}
	if !ft.lastReq.JSONOutput {
		t.Errorf("JSONOutput = false, want true (translate must request json mode)")
	}
	if ft.lastReq.Temperature != 0 {
		t.Errorf("Temperature = %v, want 0", ft.lastReq.Temperature)
	}
	if ft.lastReq.MaxTokens != 2048 {
		t.Errorf("MaxTokens = %d, want 2048", ft.lastReq.MaxTokens)
	}
	if ft.lastReq.System != translateSystemPrompt {
		t.Errorf("System prompt mismatch — handlers expect translateSystemPrompt forwarded verbatim")
	}
	// User is a JSON envelope {"strings": [...]} — parse and check.
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

func TestTranslateStrings_EmptyInputNoCall(t *testing.T) {
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			t.Fatalf("Chat must not be called when input is empty")
			return "", nil
		},
	}
	got, err := TranslateStrings(context.Background(), ft, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil result for nil input, got %v", got)
	}
	got, err = TranslateStrings(context.Background(), ft, []string{})
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

func TestTranslateStrings_LengthMismatch(t *testing.T) {
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			// Two-element response for a three-element input.
			return `["A","B"]`, nil
		},
	}
	_, err := TranslateStrings(context.Background(), ft,
		[]string{"раз", "два", "три"})
	if err == nil {
		t.Fatal("expected length-mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "length mismatch") {
		t.Errorf("error = %q, want substring 'length mismatch'", err)
	}
}

func TestTranslateStrings_PassesPlainRussianStrings(t *testing.T) {
	// Sanity: the existing test corpus (previous Anthropic-based test)
	// goes through Yandex unchanged. The translator never sees the
	// strings rewritten / escaped beyond standard JSON marshalling.
	ft := &fakeTranslator{}
	src := []string{
		"Директор по развитию",
		"ООО Ромашка",
		"Москва, ул. Ленина 5, кв. 12",
	}
	if _, err := TranslateStrings(context.Background(), ft, src); err != nil {
		t.Fatal(err)
	}
	for _, s := range src {
		if !strings.Contains(ft.lastReq.User, s) {
			t.Errorf("user payload missing input string %q — got %s", s, ft.lastReq.User)
		}
	}
}

func TestTranslateStrings_NilTranslator(t *testing.T) {
	_, err := TranslateStrings(context.Background(), nil, []string{"hi"})
	if err == nil {
		t.Fatal("expected error for nil translator")
	}
	if !strings.Contains(err.Error(), "nil translator") {
		t.Errorf("error = %q, want substring 'nil translator'", err)
	}
}

func TestTranslateStrings_PropagatesUnderlyingError(t *testing.T) {
	wantErr := errors.New("yandex boom")
	ft := &fakeTranslator{
		respond: func(yandex.ChatRequest) (string, error) {
			return "", wantErr
		},
	}
	_, err := TranslateStrings(context.Background(), ft, []string{"x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error chain missing underlying err: %v", err)
	}
}
