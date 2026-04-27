# Russian Services Migration + Multi-Step Form Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all Anthropic Claude API calls with Yandex services (YandexGPT + Yandex Vision OCR), redesign the public submission form into a multi-step wizard with file uploads (passport / ticket / voucher), and surface uploaded documents to managers in the admin panel.

**Architecture:** The work splits into 4 independent phases. Phase 1 (AI migration) is foundational and unblocks all OCR-based features in later phases. Phase 2 (file uploads + parse hooks) extends the existing public submission flow with multipart upload endpoints and DB-tracked file rows. Phase 3 (wizard form) is a pure frontend refactor that splits the current flat 747-line form into ~7 logical steps with a sidebar progress indicator. Phase 4 (admin view) adds read-only access to those files in `GroupDetailPage` / `SubmissionDetailPage`.

**Tech Stack:**
- Backend: Go (chi router, pgx), PostgreSQL 16, golang-migrate
- AI: Yandex Cloud — YandexGPT 5 Pro (text), Yandex Vision OCR (PDF/image)
- Frontend: React + Vite (no new component libraries)
- Auth to Yandex: IAM token from service account (JWT exchange every ~11h)

---

## Decisions Locked In (from prior conversation)

1. **Yandex over Anthropic for everything.** Translate, programme generation, and all scan parsing (ticket/voucher/passport) move to Yandex. Anthropic SDK + `ANTHROPIC_API_KEY` removed at end of Phase 1.
2. **PII redaction layer dropped.** Since Yandex is in RF, the local Tesseract redactor (`docgen/redact_scan.py` + `backend/internal/privacy/`) is no longer required for 152-ФЗ compliance. Kept as defence-in-depth is a non-goal — explicitly removed to simplify the pipeline.
3. **Address/authority formatting CAN now go through YandexGPT** (replaces local Go formatters in `backend/internal/format/`). Decision deferred to Phase 1, Task B3 — measured against keeping the existing Go ports.
4. **Passport scan parser uses Yandex** (the 2026-04-22-passport-scan-parser plan that proposed local Tesseract is **superseded** by this plan).
5. **Form blocks** to be: Personal → Internal Passport → Foreign Passport → Addresses → Occupation → Travel/Documents → Review.

## Open Questions to Confirm Before Starting

These are flagged inline in the relevant tasks. Confirm with user before that task starts:

- **Q1 (Phase 1, Task B3):** Drop the local Go formatters in `backend/internal/format/` and route доверенность fields through YandexGPT, or keep them local because deterministic output is more reliable than AI?
- **Q2 (Phase 2, Task F2):** ИП auto-fill — confirmed mapping:
  - `occupation_en` = `"IP [name_lat]"` (from user)
  - `employer_phone` = tourist's own `phone` (from user)
  - `occupation_ru`, `employer_ru`, `employer_address_ru` — what should be auto-set? Proposed defaults in Task F2.
- **Q3 (Phase 3, Task W1):** Auth tokens — should the YandexGPT IAM-token rotation be a goroutine in `cmd/server/main.go` (one shared token for the whole process), or per-request lazy fetch with cache?

---

# Phase 1 — Yandex AI Foundation + Migration

**Goal:** Replace every Anthropic call with Yandex equivalents. End state: code does not import `anthropic` paths, ENV no longer requires `ANTHROPIC_API_KEY`, audit log records Yandex calls.

**File Structure:**

New:
- `backend/internal/ai/yandex/client.go` — IAM auth + signed HTTP request executor
- `backend/internal/ai/yandex/iam.go` — service-account JWT → IAM token rotation goroutine
- `backend/internal/ai/yandex/gpt.go` — generic YandexGPT chat call (system + user prompt → text)
- `backend/internal/ai/yandex/ocr.go` — Yandex Vision OCR with multi-page PDF split
- `backend/internal/ai/yandex/client_test.go`, `gpt_test.go`, `ocr_test.go` — table-driven tests with mock HTTP server

Modified:
- `backend/internal/ai/translate.go` — switch to YandexGPT
- `backend/internal/ai/programme.go` — switch to YandexGPT, add anti-hallucination guard
- `backend/internal/ai/ticket_parser.go` — Yandex OCR → text → YandexGPT JSON extract
- `backend/internal/ai/voucher_parser.go` — same pattern as ticket_parser
- `backend/internal/ai/client.go` — repurpose `callClaude` seam into a provider-neutral interface (or remove if unused after migration)
- `backend/internal/ai/logger.go` — add `provider` field to log row
- `backend/internal/api/uploads.go:250-340` — drop redaction call, route scan to new Yandex parsers
- `cmd/server/main.go` — start IAM token rotator goroutine; replace `ANTHROPIC_API_KEY` env reads
- `Dockerfile.backend` — drop the Tesseract/OpenCV pip install layer (no longer needed)

New migration:
- `backend/migrations/000019_ai_call_logs_provider.up.sql` / `.down.sql`
  ```sql
  ALTER TABLE ai_call_logs ADD COLUMN provider TEXT NOT NULL DEFAULT 'anthropic';
  ALTER TABLE ai_call_logs ALTER COLUMN provider DROP DEFAULT;
  ```

Deleted (final cleanup):
- `backend/internal/privacy/redact.go` (or whole package)
- `docgen/redact_scan.py`
- Anthropic-specific helpers in `backend/internal/ai/client.go` after migration

New passport parser:
- `backend/internal/ai/passport_parser.go` — Yandex OCR + YandexGPT extraction → `PassportFields` struct
- `backend/internal/ai/passport_parser_test.go`

---

### Task 1.A1: Add YANDEX_* env vars and config wiring

**Files:**
- Modify: `cmd/server/main.go`
- Modify: `CLAUDE.md` (Environment Variables section)

- [ ] **Step 1: Add Yandex env-var reads to main.go**

```go
// At main.go startup, alongside other env reads:
yandexFolderID := os.Getenv("YANDEX_FOLDER_ID")
yandexSAKeyJSON := os.Getenv("YANDEX_SA_KEY_JSON") // contents of authorized_key.json
if yandexFolderID == "" || yandexSAKeyJSON == "" {
    log.Fatal("YANDEX_FOLDER_ID and YANDEX_SA_KEY_JSON are required")
}
```

- [ ] **Step 2: Update CLAUDE.md env section**

Replace the `ANTHROPIC_API_KEY=...` line with:
```
YANDEX_FOLDER_ID=b1gv...
YANDEX_SA_KEY_JSON={"id":"...","service_account_id":"...","private_key":"..."}
```

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go CLAUDE.md
git commit -m "feat(ai): wire Yandex env vars (no callers yet)"
```

---

### Task 1.A2: Implement IAM token rotation

**Files:**
- Create: `backend/internal/ai/yandex/iam.go`
- Create: `backend/internal/ai/yandex/iam_test.go`

- [ ] **Step 1: Write failing test for JWT generation**

```go
// iam_test.go
func TestSignedJWT_HasExpectedClaims(t *testing.T) {
    keyJSON := testFixtureKey() // returns sample authorized_key.json bytes
    jwt, err := buildSignedJWT(keyJSON, time.Now())
    if err != nil { t.Fatal(err) }
    parts := strings.Split(jwt, ".")
    if len(parts) != 3 { t.Fatalf("expected 3 JWT parts, got %d", len(parts)) }
    // Decode payload, check iss/aud/exp present
}
```

- [ ] **Step 2: Run test, confirm it fails**

```bash
cd backend && go test ./internal/ai/yandex/ -run TestSignedJWT
```

Expected: `undefined: buildSignedJWT`

- [ ] **Step 3: Implement JWT signer + IAM exchange**

`backend/internal/ai/yandex/iam.go`:
```go
package yandex

import (
    "context"
    "crypto/rsa"
    "encoding/json"
    "fmt"
    "net/http"
    "sync"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

type saKey struct {
    ID               string `json:"id"`
    ServiceAccountID string `json:"service_account_id"`
    PrivateKey       string `json:"private_key"`
}

type TokenSource struct {
    keyJSON []byte
    mu      sync.RWMutex
    token   string
    exp     time.Time
}

func NewTokenSource(keyJSON []byte) *TokenSource {
    return &TokenSource{keyJSON: keyJSON}
}

func (ts *TokenSource) Token(ctx context.Context) (string, error) {
    ts.mu.RLock()
    if ts.token != "" && time.Until(ts.exp) > 60*time.Second {
        t := ts.token
        ts.mu.RUnlock()
        return t, nil
    }
    ts.mu.RUnlock()

    return ts.refresh(ctx)
}

func (ts *TokenSource) refresh(ctx context.Context) (string, error) {
    // build JWT, POST to https://iam.api.cloud.yandex.net/iam/v1/tokens,
    // with body {"jwt": "..."}, parse {"iamToken":"...", "expiresAt":"..."}
    // store under lock, return token
}

func buildSignedJWT(keyJSON []byte, now time.Time) (string, error) {
    var k saKey
    if err := json.Unmarshal(keyJSON, &k); err != nil {
        return "", fmt.Errorf("parse SA key: %w", err)
    }
    privKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(k.PrivateKey))
    if err != nil { return "", fmt.Errorf("parse RSA: %w", err) }
    claims := jwt.MapClaims{
        "iss": k.ServiceAccountID,
        "aud": "https://iam.api.cloud.yandex.net/iam/v1/tokens",
        "iat": now.Unix(),
        "exp": now.Add(time.Hour).Unix(),
    }
    tok := jwt.NewWithClaims(jwt.SigningMethodPS256, claims)
    tok.Header["kid"] = k.ID
    return tok.SignedString(privKey)
}

var _ = rsa.PublicKey{} // keeps import if unused above
```

Add to `backend/go.mod`:
```bash
cd backend && go get github.com/golang-jwt/jwt/v5
```

- [ ] **Step 4: Run test to verify pass**

```bash
cd backend && go test ./internal/ai/yandex/ -run TestSignedJWT -v
```

- [ ] **Step 5: Add integration test for refresh against mock server**

```go
func TestTokenSource_RefreshAndCache(t *testing.T) {
    var calls int
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        calls++
        json.NewEncoder(w).Encode(map[string]any{
            "iamToken": fmt.Sprintf("token-%d", calls),
            "expiresAt": time.Now().Add(time.Hour).Format(time.RFC3339Nano),
        })
    }))
    defer srv.Close()
    // patch endpoint via package var, call Token() twice within 1 minute,
    // assert second call returns cached token (calls == 1)
}
```

Make endpoint URL a package-level var so tests can override.

- [ ] **Step 6: Run all yandex tests**

```bash
cd backend && go test ./internal/ai/yandex/ -v
```

- [ ] **Step 7: Commit**

```bash
git add backend/internal/ai/yandex/iam.go backend/internal/ai/yandex/iam_test.go backend/go.mod backend/go.sum
git commit -m "feat(ai/yandex): IAM token source with JWT exchange + caching"
```

---

### Task 1.A3: Implement YandexGPT chat client

**Files:**
- Create: `backend/internal/ai/yandex/gpt.go`
- Create: `backend/internal/ai/yandex/gpt_test.go`

- [ ] **Step 1: Write failing test for GPT call**

```go
// gpt_test.go
func TestChat_HappyPath(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Authorization") != "Bearer test-token" {
            t.Fatalf("missing/wrong auth: %q", r.Header.Get("Authorization"))
        }
        json.NewEncoder(w).Encode(map[string]any{
            "result": map[string]any{
                "alternatives": []map[string]any{
                    {"message": map[string]any{"role": "assistant", "text": "hello"}},
                },
            },
        })
    }))
    defer srv.Close()

    client := NewGPTClient("test-token", "test-folder", srv.URL)
    out, err := client.Chat(context.Background(), ChatRequest{
        Model: "yandexgpt", System: "be brief", User: "ping", Temperature: 0,
    })
    if err != nil { t.Fatal(err) }
    if out != "hello" { t.Fatalf("got %q want %q", out, "hello") }
}
```

- [ ] **Step 2: Run test, confirm fail**

- [ ] **Step 3: Implement GPT client**

```go
// gpt.go
package yandex

type ChatRequest struct {
    Model       string
    System      string
    User        string
    Temperature float64
    MaxTokens   int
    JSONOutput  bool
}

type GPTClient struct {
    iamToken   string // for tests; production uses TokenSource
    tokenSrc   *TokenSource
    folderID   string
    endpoint   string
}

func NewGPTClient(iamToken, folderID, endpoint string) *GPTClient { ... }
func NewGPTClientFromSource(ts *TokenSource, folderID string) *GPTClient { ... }

func (c *GPTClient) Chat(ctx context.Context, req ChatRequest) (string, error) {
    // POST {endpoint}/foundationModels/v1/completion
    // body:
    //   modelUri: gpt://{folder}/yandexgpt/latest
    //   completionOptions: {temperature, maxTokens, responseFormat: "text"|"json"}
    //   messages: [{role:"system",text:...},{role:"user",text:...}]
    // Authorization: Bearer {iamToken}
    // Parse result.alternatives[0].message.text, return.
}
```

- [ ] **Step 4: Run test, verify pass**

- [ ] **Step 5: Add JSON-mode test**

```go
func TestChat_JSONMode(t *testing.T) {
    // Server asserts request body has completionOptions.responseFormat == "json"
    // Returns valid JSON string in alternatives[0].message.text
}
```

- [ ] **Step 6: Commit**

```bash
git add backend/internal/ai/yandex/gpt.go backend/internal/ai/yandex/gpt_test.go
git commit -m "feat(ai/yandex): YandexGPT chat client"
```

---

### Task 1.A4: Implement Yandex Vision OCR client (multi-page PDF aware)

**Files:**
- Create: `backend/internal/ai/yandex/ocr.go`
- Create: `backend/internal/ai/yandex/ocr_test.go`

- [ ] **Step 1: Write failing test for single-page image OCR**

```go
func TestOCR_SinglePageImage(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        var body map[string]any
        json.NewDecoder(r.Body).Decode(&body)
        if body["mimeType"] != "image/jpeg" {
            t.Fatalf("mime: %v", body["mimeType"])
        }
        json.NewEncoder(w).Encode(map[string]any{
            "result": map[string]any{
                "textAnnotation": map[string]any{
                    "fullText": "hello world",
                    "blocks":   []any{},
                },
            },
        })
    }))
    defer srv.Close()
    c := NewOCRClient("tok", "folder", srv.URL)
    pages, err := c.Recognize(context.Background(), []byte{0xFF, 0xD8, 0xFF}, "image/jpeg")
    if err != nil { t.Fatal(err) }
    if len(pages) != 1 || pages[0] != "hello world" { t.Fatalf("got %#v", pages) }
}
```

- [ ] **Step 2: Run test, confirm fail**

- [ ] **Step 3: Implement OCR client (single-page path)**

```go
// ocr.go
package yandex

type OCRClient struct { /* token, folderID, endpoint */ }

// Recognize returns one string per page. Single-page documents return
// a one-element slice; multi-page PDFs return one entry per page in
// document order.
func (c *OCRClient) Recognize(ctx context.Context, content []byte, mime string) ([]string, error) {
    if mime == "application/pdf" {
        return c.recognizePDF(ctx, content)
    }
    return c.recognizeOnePage(ctx, content, mime)
}

func (c *OCRClient) recognizeOnePage(ctx context.Context, content []byte, mime string) ([]string, error) {
    // POST https://ocr.api.cloud.yandex.net/ocr/v1/recognizeText
    // body: {mimeType, languageCodes:["*"], model:"page", content: base64}
    // returns [fullText]
}
```

- [ ] **Step 4: Run test, verify pass**

- [ ] **Step 5: Write failing test for multi-page PDF**

```go
func TestOCR_MultiPagePDF(t *testing.T) {
    var calls int
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        calls++
        json.NewEncoder(w).Encode(map[string]any{
            "result": map[string]any{
                "textAnnotation": map[string]any{"fullText": fmt.Sprintf("page%d", calls)},
            },
        })
    }))
    defer srv.Close()
    pdfBytes := loadFixture(t, "testdata/two-page.pdf")
    c := NewOCRClient("tok", "folder", srv.URL)
    pages, _ := c.Recognize(context.Background(), pdfBytes, "application/pdf")
    if len(pages) != 2 { t.Fatalf("expected 2 pages, got %d", len(pages)) }
}
```

Add a tiny 2-page PDF fixture under `backend/internal/ai/yandex/testdata/two-page.pdf`.

- [ ] **Step 6: Implement multi-page split**

Use `github.com/pdfcpu/pdfcpu/pkg/api` (pure Go, already in indirect deps via fillpdf? — verify; otherwise add dep) to split PDF into per-page byte slices, then call `recognizeOnePage` for each.

```go
func (c *OCRClient) recognizePDF(ctx context.Context, pdf []byte) ([]string, error) {
    pages, err := splitPDFInMemory(pdf)
    if err != nil { return nil, fmt.Errorf("split pdf: %w", err) }
    out := make([]string, len(pages))
    for i, page := range pages {
        text, err := c.recognizeOnePage(ctx, page, "application/pdf")
        if err != nil { return nil, fmt.Errorf("page %d: %w", i+1, err) }
        // Recognize returns []string; flatten:
        out[i] = strings.Join(text, "\n")
    }
    return out, nil
}
```

- [ ] **Step 7: Run all OCR tests, verify pass**

- [ ] **Step 8: Commit**

```bash
git add backend/internal/ai/yandex/ocr.go backend/internal/ai/yandex/ocr_test.go backend/internal/ai/yandex/testdata/
git commit -m "feat(ai/yandex): Vision OCR client with multi-page PDF support"
```

---

### Task 1.A5: Update audit log to record provider

**Files:**
- Create: `backend/migrations/000019_ai_call_logs_provider.up.sql`
- Create: `backend/migrations/000019_ai_call_logs_provider.down.sql`
- Modify: `backend/internal/ai/logger.go`
- Modify: `backend/internal/ai/logger_test.go`

- [ ] **Step 1: Write migration**

```sql
-- up
ALTER TABLE ai_call_logs ADD COLUMN provider TEXT NOT NULL DEFAULT 'anthropic';
ALTER TABLE ai_call_logs ALTER COLUMN provider DROP DEFAULT;

-- down
ALTER TABLE ai_call_logs DROP COLUMN provider;
```

- [ ] **Step 2: Update Logger.Log signature**

In `logger.go` add `Provider string` field to the row struct and to the INSERT statement. Existing Anthropic call sites pass `"anthropic"`; new yandex code paths pass `"yandex-gpt"` or `"yandex-vision"`.

- [ ] **Step 3: Run migration locally**

```bash
make db-migrate-up
```

- [ ] **Step 4: Run logger tests**

```bash
cd backend && go test ./internal/ai/ -run Logger -v
```

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/000019*.sql backend/internal/ai/logger.go backend/internal/ai/logger_test.go
git commit -m "feat(ai): audit log records provider (anthropic|yandex-gpt|yandex-vision)"
```

---

### Task 1.B1: Migrate translate.go to YandexGPT

**Files:**
- Modify: `backend/internal/ai/translate.go`
- Modify: `backend/internal/ai/translate_test.go`
- Modify: `backend/internal/api/generate.go:336-345` (call site — likely no change beyond client injection)

- [ ] **Step 1: Update test to mock YandexGPT instead of Anthropic**

The existing test mocks `callClaude` via httptest. Switch to mocking `yandex.GPTClient` — inject a fake client into `TranslateStrings` (refactor signature to accept a client, not raw apiKey).

- [ ] **Step 2: Refactor TranslateStrings signature**

```go
type Translator interface {
    Chat(ctx context.Context, req yandex.ChatRequest) (string, error)
}

func TranslateStrings(ctx context.Context, t Translator, src []string) ([]string, error) {
    if len(src) == 0 { return nil, nil }
    body, _ := json.Marshal(map[string]any{"strings": src})
    raw, err := t.Chat(ctx, yandex.ChatRequest{
        System:      translateSystemPrompt,
        User:        string(body),
        Temperature: 0,
        MaxTokens:   2048,
        JSONOutput:  true,
    })
    if err != nil { return nil, err }
    // parse JSON array (re-use extractJSON)
}
```

- [ ] **Step 3: Update call site in generate.go**

```go
// in runPipeline, build the GPT client once:
gpt := yandex.NewGPTClientFromSource(yandexTokenSrc, yandexFolderID)
// pass it down:
t, err := ai.TranslateStrings(gctx, gpt, uniques)
```

`yandexTokenSrc` and `yandexFolderID` come from request context or are injected via the API struct (existing pattern uses `s *API` struct).

- [ ] **Step 4: Run tests, verify pass**

```bash
cd backend && go test ./internal/ai/ -run Translate -v
```

- [ ] **Step 5: Manual smoke test**

Boot the dev backend, generate documents for a real group, confirm translations come back from Yandex (check audit log: provider=yandex-gpt).

- [ ] **Step 6: Commit**

```bash
git add backend/internal/ai/translate.go backend/internal/ai/translate_test.go backend/internal/api/generate.go
git commit -m "feat(ai/translate): switch to YandexGPT 5 Pro"
```

---

### Task 1.B2: Migrate programme.go to YandexGPT (with anti-hallucination guard)

**Files:**
- Modify: `backend/internal/ai/programme.go`
- Modify: `backend/internal/ai/programme_test.go`
- Modify: `backend/internal/api/generate.go:344-353` (call site)

- [ ] **Step 1: Tighten the system prompt against fabricated places**

Append to existing `programmeSystemPrompt`:
```
STRICT FACTUALITY:
- Every place name MUST be a real, well-known landmark verifiable on Google Maps.
- If even slightly unsure, choose a more famous alternative.
- NEVER use hedging phrases like "if time allows" — those are red flags that you are guessing. Output places with full confidence or do not output them at all.
```

- [ ] **Step 2: Refactor GenerateProgramme to take a Translator interface**

Same pattern as Task 1.B1. Accept `t Translator`, build YandexGPT request, parse JSON array.

- [ ] **Step 3: Update call site in generate.go**

```go
p, err := ai.GenerateProgramme(gctx, gpt, programmeInput)
```

- [ ] **Step 4: Run programme test**

```bash
cd backend && go test ./internal/ai/ -run Programme -v
```

- [ ] **Step 5: Manual end-to-end test**

Generate a real group's programme, eyeball the result for:
- Correct YYYY-DD-MM format
- Real (Googlable) places
- Proper `\n\n` separators
- Manager notes honoured

- [ ] **Step 6: Commit**

```bash
git add backend/internal/ai/programme.go backend/internal/ai/programme_test.go backend/internal/api/generate.go
git commit -m "feat(ai/programme): switch to YandexGPT 5 Pro + anti-hallucination guard"
```

---

### Task 1.B3 (CONDITIONAL on Q1): Доверенность formatter — keep local Go OR route through YandexGPT

If user answers "keep local" → skip this task.

If "route through YandexGPT":

**Files:**
- Modify: `backend/internal/ai/doverenost_format.go`
- Modify: `backend/internal/ai/assembler.go` (call site)

- [ ] **Step 1: Define formatter system prompt**

```
You are a Russian text formatter for legal documents (доверенность). Given a free-text Russian field (address or issuing authority), normalize it to canonical formatting:
- Short abbreviations (д, кв, ул, г, р-н, пр-т) lowercased + dot.
- State acronyms (МВД, УФМС, ОУФМС, ГУ) UPPERCASE no dot.
- Proper nouns Title Case.
- Function words lowercased.
- Comma after each address segment.
Return ONLY the formatted string, no prose.
```

- [ ] **Step 2: Refactor `CleanDoverenostFields`**

Accept Translator, batch all fields in one call, parse a JSON object back.

- [ ] **Step 3: Tests + call site**

- [ ] **Step 4: Commit**

---

### Task 1.C1: Migrate ticket_parser.go to Yandex OCR + GPT

**Files:**
- Modify: `backend/internal/ai/ticket_parser.go`
- Modify: `backend/internal/ai/ticket_parser_test.go` (create if missing)
- Modify: `backend/internal/api/uploads.go:250-340` (caller)

- [ ] **Step 1: Define new pipeline**

```go
// In ticket_parser.go:
func ParseTicketScan(ctx context.Context, ocr *yandex.OCRClient, gpt *yandex.GPTClient, scan []byte, mime string) (TicketParse, error) {
    pages, err := ocr.Recognize(ctx, scan, mime)
    if err != nil { return TicketParse{}, fmt.Errorf("ocr: %w", err) }
    fullText := strings.Join(pages, "\n\n")

    raw, err := gpt.Chat(ctx, yandex.ChatRequest{
        System:     ticketParseSystemPrompt,
        User:       fullText,
        Temperature: 0,
        JSONOutput: true,
    })
    if err != nil { return TicketParse{}, fmt.Errorf("gpt: %w", err) }

    var out TicketParse
    if err := json.Unmarshal([]byte(extractJSON(raw)), &out); err != nil {
        return TicketParse{}, fmt.Errorf("decode: %w", err)
    }
    return out, nil
}
```

- [ ] **Step 2: Write the JSON-extraction system prompt**

Adapt the current Anthropic prompt. The shape stays the same (flight number, time, IATA airport code, etc.).

- [ ] **Step 3: Drop the redaction call in uploads.go**

Locate the call to `privacy.RedactScan` and remove it (lines vary; current code is around `uploads.go:280-300`). Pass the raw scan bytes directly to `ParseTicketScan`.

- [ ] **Step 4: Add table-driven test with golden text fixture**

`testdata/ticket-aeroflot.txt` — pre-OCR'd text from a real ticket. Test calls `ParseTicketScan` against a fake OCR (returns the fixture text directly) + fake GPT (returns expected JSON).

- [ ] **Step 5: Manual test against a real PDF ticket**

- [ ] **Step 6: Commit**

```bash
git add backend/internal/ai/ticket_parser.go backend/internal/ai/ticket_parser_test.go backend/internal/api/uploads.go
git commit -m "feat(ai/ticket): switch to Yandex Vision OCR + YandexGPT"
```

---

### Task 1.C2: Migrate voucher_parser.go to Yandex OCR + GPT

Same shape as Task 1.C1. Caller is also in `uploads.go`.

- [ ] **Step 1–6:** Mirror Task 1.C1 with voucher-specific prompt and JSON schema (hotel name, address, check-in, check-out, etc.).

```bash
git commit -m "feat(ai/voucher): switch to Yandex Vision OCR + YandexGPT"
```

---

### Task 1.C3: Add passport_parser.go (NEW)

**Files:**
- Create: `backend/internal/ai/passport_parser.go`
- Create: `backend/internal/ai/passport_parser_test.go`

- [ ] **Step 1: Define output struct**

```go
type PassportFields struct {
    Series         string // "4523" (internal) or "" (foreign)
    Number         string // "172344" (internal) or "1234567" (foreign)
    LastName       string
    FirstName      string
    Patronymic     string
    Gender         string // "МУЖ" | "ЖЕН"
    BirthDate      string // YYYY-MM-DD
    PlaceOfBirth   string
    IssueDate      string
    IssuingAuthor  string
    DepartmentCode string // "770-091"
    RegAddress     string // page 2 of internal passport
    Type           string // "internal" | "foreign"
}
```

- [ ] **Step 2: Write the system prompt**

Two variants — internal vs foreign — selected by Type the user uploaded. For internal, extract fields including page-2 registration; for foreign, extract MRZ fields.

- [ ] **Step 3: Implement ParsePassportScan**

Same shape as ticket parser: OCR → text → GPT JSON extract.

- [ ] **Step 4: Tests with real-passport text fixture**

Strip the actual scan we tested earlier (already produced good OCR output) into a `testdata/passport-internal.txt` fixture. Assert the parser correctly extracts fields, including handling the small OCR errors (e.g. cross-validate dates against MRZ).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ai/passport_parser.go backend/internal/ai/passport_parser_test.go
git commit -m "feat(ai/passport): scan parser via Yandex Vision OCR + YandexGPT"
```

---

### Task 1.D1: Remove Anthropic code, deps, env vars

**Files:**
- Modify: `backend/internal/ai/client.go` — remove `callClaude`, anthropic-specific structs
- Modify: `backend/go.mod` — `go mod tidy` after deletions
- Modify: `cmd/server/main.go` — drop `ANTHROPIC_API_KEY` reads
- Modify: `Dockerfile.backend` — drop pytesseract/opencv pip layer
- Delete: `backend/internal/privacy/` package (and any tests)
- Delete: `docgen/redact_scan.py`
- Modify: `CLAUDE.md` — rewrite AI Privacy section, remove `REDACT_SCAN_SCRIPT` env var, document new Yandex setup

- [ ] **Step 1: Remove Anthropic helpers**

Delete the `callClaude` function and the `anthropicRequest`/`anthropicMessage` structs from `client.go`. If anything still calls them at this point, fix the call site (should be none after Tasks 1.B1, 1.B2, 1.C1, 1.C2).

- [ ] **Step 2: Remove privacy package**

```bash
rm -rf backend/internal/privacy/
rm docgen/redact_scan.py
```

- [ ] **Step 3: go mod tidy**

```bash
cd backend && go mod tidy
```

- [ ] **Step 4: Build + test**

```bash
cd backend && go build ./... && go test ./...
```

Expected: green.

- [ ] **Step 5: Update CLAUDE.md**

Rewrite:
- "Stack" table: Anthropic row → Yandex row.
- "AI Privacy" section: drop the "PII never goes to Claude" framing — replace with "all AI calls go to Yandex Cloud (RF-resident, no cross-border transfer)".
- Remove `REDACT_SCAN_SCRIPT` from Environment Variables.
- Remove the Anthropic API key from required env vars.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "chore(ai): remove Anthropic client, redaction layer, related deps + docs"
```

**End of Phase 1.** No more Anthropic calls. Audit log shows only `yandex-gpt` and `yandex-vision` providers. App functions exactly as before to the user, with no UI changes.

---

# Phase 2 — File Uploads in Public Submission Form

**Goal:** Backend support for tourists to upload internal passport, foreign passport, ticket, and voucher scans through the public form (`/form/<slug>`). Files are stored, listed, deletable, replaceable. Passport scans auto-fill form fields via Yandex OCR. Ticket/voucher scans queue for manager review (the existing parsers will run on them when manager attaches the submission to a group, same flow as today).

**File Structure:**

New migration:
- `backend/migrations/000020_submission_files.up.sql` / `.down.sql`

```sql
-- up
CREATE TABLE submission_files (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    submission_id UUID NOT NULL REFERENCES tourist_submissions(id) ON DELETE CASCADE,
    file_type     TEXT NOT NULL CHECK (file_type IN ('passport_internal','passport_foreign','ticket','voucher')),
    file_path     TEXT NOT NULL,
    original_name TEXT NOT NULL,
    mime_type     TEXT NOT NULL,
    size_bytes    BIGINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (submission_id, file_type)  -- one file per type per submission; replace overwrites
);
CREATE INDEX submission_files_submission_idx ON submission_files (submission_id);
CREATE INDEX submission_files_org_idx ON submission_files (org_id);

-- down
DROP TABLE submission_files;
```

New backend files:
- `backend/internal/api/handlers_public_files.go` — public endpoints for upload/delete/list, scoped to submission slug + temporary submission token
- `backend/internal/db/submission_files.go` — repo with CRUD scoped by org_id

Modified:
- `backend/internal/api/handlers_public.go` — add the new public endpoints to the chi router builder
- `backend/internal/server/router.go` — register routes
- `backend/internal/api/submissions.go` — when a manager attaches a submission to a group (creates `tourists` row), copy the files to the tourist (or just keep them on the submission with org-scoped read access)

### Task 2.E1: Migration for submission_files table

**Files:**
- Create: `backend/migrations/000020_submission_files.up.sql`
- Create: `backend/migrations/000020_submission_files.down.sql`

- [ ] **Step 1: Write the migration SQL** (shown above)

- [ ] **Step 2: Apply locally**

```bash
make db-migrate-up
psql $DATABASE_URL -c "\\d submission_files"
```

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000020*
git commit -m "feat(db): submission_files table for public-form uploads"
```

---

### Task 2.E2: Repository functions for submission_files

**Files:**
- Create: `backend/internal/db/submission_files.go`
- Create: `backend/internal/db/submission_files_test.go`

- [ ] **Step 1: Write tests for InsertOrReplace, ListBySubmission, DeleteByID**

All scoped by org_id (compiler-enforced). Tests use existing testdb harness.

- [ ] **Step 2: Implement repo functions**

```go
type SubmissionFile struct { /* mirrors columns */ }

func InsertOrReplaceSubmissionFile(ctx context.Context, p *pgxpool.Pool, orgID string, f SubmissionFile) error {
    // ON CONFLICT (submission_id, file_type) DO UPDATE
}

func ListSubmissionFiles(ctx context.Context, p *pgxpool.Pool, orgID, submissionID string) ([]SubmissionFile, error) { ... }

func DeleteSubmissionFile(ctx context.Context, p *pgxpool.Pool, orgID, fileID string) error { ... }
```

- [ ] **Step 3: Run tests + commit**

```bash
git add backend/internal/db/submission_files.go backend/internal/db/submission_files_test.go
git commit -m "feat(db): submission_files repo with org-scoped queries"
```

---

### Task 2.E3: Public upload endpoint with temp token

**Files:**
- Create: `backend/internal/api/handlers_public_files.go`

The existing public form is unauthenticated — we need a way to associate uploads with the in-progress submission *before* it's saved. Use a short-lived JWT-like token returned at form-start.

Or simpler: require the form to first POST a "start" call that creates an empty submission and returns its ID + a signed token. All subsequent file uploads include this token. On final submit, the form sends the same submission ID and the saved data is merged in.

- [ ] **Step 1: Decide token vs flat ID-only design**

For MVP, use plain submission ID — the slug already gates access to a specific org's submission pool. To prevent abuse, rate-limit uploads per IP per slug (5/min, 50/hour).

- [ ] **Step 2: Add `POST /api/public/submissions/<slug>/start` (creates draft submission row)**

```go
// Returns {submission_id: "uuid"}
```

- [ ] **Step 3: Add `POST /api/public/submissions/<slug>/files/<type>` (multipart upload)**

```go
// type ∈ {passport_internal, passport_foreign, ticket, voucher}
// query/body: submission_id
// 50 MB max file size
// stores under uploads/{org_id}/submissions/{submission_id}/{type}.{ext}
// inserts/replaces submission_files row
```

- [ ] **Step 4: Add `DELETE /api/public/submissions/<slug>/files/<id>`**

- [ ] **Step 5: Add `GET /api/public/submissions/<slug>/files?submission_id=...`**

Returns list of types currently uploaded.

- [ ] **Step 6: Wire routes + tests + commit**

```bash
git commit -m "feat(api): public file upload/list/delete endpoints"
```

---

### Task 2.F1: Passport-scan auto-fill endpoint

**Files:**
- Modify: `backend/internal/api/handlers_public_files.go`

`POST /api/public/submissions/<slug>/parse-passport` body `{file_id, type}` → returns parsed `PassportFields` JSON for the form to apply locally.

- [ ] **Step 1: Define handler**

```go
func handleParsePassport(...) {
    // 1. Load file from submission_files.
    // 2. Read bytes from disk.
    // 3. Call ai.ParsePassportScan(ctx, ocr, gpt, bytes, mime, type).
    // 4. Return JSON to frontend.
}
```

- [ ] **Step 2: Tests with mocked AI clients**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(api): /parse-passport — auto-fill via Yandex OCR+GPT"
```

---

### Task 2.F2: ИП (Individual Entrepreneur) auto-fill rule

> **Q2 — confirm with user before this task:**
> User specified:
> - `occupation_en = "IP [name_lat]"`
> - `employer_phone = phone (tourist's own)`
>
> Proposed defaults for the rest:
> - `occupation_ru = "ИП " + last_name_ru` (e.g. "ИП Иванов")
> - `employer_ru = same as occupation_ru` (e.g. "ИП Иванов")
> - `employer_address_ru = home_address_ru OR reg_address_ru` — pick whichever is non-empty
>
> Confirm or override these defaults before writing the task.

**Files:**
- Modify: `backend/internal/ai/assembler.go` (or wherever ИП mapping currently happens — search for "ip" / "ИП" in the assembler)
- Modify: `frontend/src/components/forms/OccupationStep.jsx` (created in Phase 3)

This is largely a frontend concern — the form pre-fills hidden fields when the checkbox toggles. Backend assembler may need a sanity-check that `occupation_en` is in the IP format if the marker field is set.

- [ ] **Step 1–N:** TBD after Q2 is answered. Most likely a 50-line frontend change.

---

# Phase 3 — Multi-Step Wizard Form (Frontend Refactor)

**Goal:** Replace the 747-line flat `SubmissionForm.jsx` with a multi-step wizard. Sidebar on the left shows progress and current step. Each step has its own component with its own validation. Form state survives step navigation. New steps can include file-upload widgets that talk to Phase 2 endpoints.

**File Structure:**

New:
- `frontend/src/components/forms/FormWizard.jsx` — orchestrator + state container, sidebar, step transitions
- `frontend/src/components/forms/StepSidebar.jsx` — left sidebar with step list + progress indicator
- `frontend/src/components/forms/steps/PersonalStep.jsx`
- `frontend/src/components/forms/steps/InternalPassportStep.jsx`
- `frontend/src/components/forms/steps/ForeignPassportStep.jsx`
- `frontend/src/components/forms/steps/AddressesStep.jsx`
- `frontend/src/components/forms/steps/OccupationStep.jsx` — includes ИП checkbox + auto-fill
- `frontend/src/components/forms/steps/TravelDocsStep.jsx` — ticket + voucher upload
- `frontend/src/components/forms/steps/ReviewStep.jsx` — read-only summary + submit
- `frontend/src/components/forms/FileUploadField.jsx` — reusable upload widget (file type label, current file, replace, delete)
- `frontend/src/components/forms/DocumentsSection.jsx` — list of uploaded files with delete/replace per entry
- `frontend/src/api/files.js` — client-side helpers for the new endpoints

Modified:
- `frontend/src/pages/SubmissionFormPage.jsx` — render `<FormWizard />` instead of `<SubmissionForm />`
- `frontend/src/components/SubmissionForm.jsx` — keep for now (used by other places? confirm with grep) or delete

Deleted (after migration):
- Old `SubmissionForm.jsx` if no longer used elsewhere

### Task 3.W1: FormWizard skeleton with step navigation

- [ ] **Step 1: Define step contract**

Each step component receives `{payload, setPayload, errors, onNext, onBack}` and renders its fields. Validation runs on Next click; failure prevents advance.

- [ ] **Step 2: Implement FormWizard with hardcoded 7 steps**

State held at FormWizard level (single `payload` object — same shape as today's flat form). Active step index in state. Next/Back buttons in step's footer. Progress saved to `localStorage` keyed by slug.

- [ ] **Step 3: Implement StepSidebar**

Lists 7 step labels with Cyrillic titles. Highlights current. Past steps marked complete (green check). Future steps disabled.

- [ ] **Step 4: Render placeholder content per step**

Each step file just returns `<div>TODO: {step name}</div>` for now.

- [ ] **Step 5: Manual test in browser**

Run `make frontend-dev`, visit `/form/<test-slug>`, click through all 7 steps.

- [ ] **Step 6: Commit**

```bash
git commit -m "feat(form): wizard skeleton with sidebar + step navigation"
```

---

### Task 3.W2–W7: Per-step content

Each step task follows the same pattern:
1. Move the relevant subset of fields from `SubmissionForm.jsx` into the step component.
2. Add per-step validation.
3. Wire to FormWizard.
4. Manual smoke test.
5. Commit.

Step contents:

| Step | Fields | Notes |
|------|--------|-------|
| Personal | name_cyr, name_lat, gender_ru, birth_date, marital_status_ru, place_of_birth_ru, nationality_ru, former_nationality_ru, maiden_name_ru | |
| Internal Passport | internal_series, internal_number, internal_issued_ru, internal_issued_by_ru | + file upload widget (passport_internal). On upload → call `/parse-passport` → fill fields. |
| Foreign Passport | passport_number, passport_type_ru, issue_date, expiry_date, issued_by_ru | + file upload widget (passport_foreign). Same parse hook. |
| Addresses | reg_address_ru, home_address_ru | "Same as registration" checkbox |
| Occupation | occupation_ru, employer_ru, employer_address_ru, employer_phone | ИП checkbox at top — when set, hides the fields and shows summary |
| Travel/Docs | been_to_japan_ru, previous_visits_ru, criminal_record_ru, phone | + ticket/voucher upload widgets (no auto-parse — that runs when manager attaches) |
| Review | All fields read-only | Submit button → `POST /api/public/submissions/<slug>` with full payload |

---

### Task 3.W8: FileUploadField component

- [ ] **Step 1: Define props**

```jsx
<FileUploadField
  fileType="passport_internal"
  submissionId={submissionId}
  slug={slug}
  currentFile={fileMeta}  // {id, original_name, size_bytes} or null
  onUploaded={(meta) => ...}
  onDeleted={() => ...}
  label="Внутренний паспорт"
  acceptMime="application/pdf,image/jpeg,image/png"
  onParseComplete={(parsed) => fill the form}  // optional — for passports
/>
```

- [ ] **Step 2: Implement upload UI**

Drag-drop zone OR click-to-pick. Shows progress bar during upload. After success, shows file name + buttons: Replace, Delete.

- [ ] **Step 3: Wire to API client**

`api/files.js` with `uploadSubmissionFile`, `deleteSubmissionFile`, `parsePassport` helpers.

- [ ] **Step 4: Tests + commit**

---

### Task 3.W9: DocumentsSection (visible on Review step)

A summary card showing all 4 file types with their current status (uploaded/missing) and Delete buttons. This is also embedded inline on each step that has a file widget — same component, different filtering.

---

# Phase 4 — Admin View of User-Uploaded Documents

**Goal:** Managers see which files each submission has, can preview/download them, can delete (rare — usually only if abuse).

**File Structure:**

Modified:
- `backend/internal/api/handlers_public.go` (or new admin file): add authenticated endpoints
  - `GET /api/submissions/{id}/files` — list
  - `GET /api/submissions/{id}/files/{file_id}/download` — stream
  - `DELETE /api/submissions/{id}/files/{file_id}` — admin delete
- `frontend/src/pages/SubmissionDetailPage.jsx` — render new `<SubmissionFilesPanel />`
- `frontend/src/components/SubmissionFilesPanel.jsx` — list with download/preview buttons per file
- `frontend/src/pages/GroupDetailPage.jsx` — when listing tourists in the group, badge each tourist with file counts; click to open files modal

### Task 4.A1: Admin endpoints

- [ ] List, download, delete — all org-scoped via `middleware.OrgID`.

### Task 4.A2: SubmissionFilesPanel component

- [ ] List rows per file type with:
  - File name + size
  - "Download" button (opens authenticated stream)
  - "Delete" button (admin only)
  - Last uploaded date

### Task 4.A3: GroupDetailPage badge integration

- [ ] When manager opens a group, beside each tourist row show counts of attached files. Click → modal showing the same SubmissionFilesPanel.

---

# Cross-Cutting Concerns

## Testing strategy

- Unit tests for every new client (yandex/iam, gpt, ocr) using httptest mock servers.
- Integration tests for new endpoints using existing testdb harness.
- Frontend: no test framework currently in repo — manual smoke tests per step.

## Rollback strategy

Phase 1 commits should be tagged `pre-yandex-migration` (already done as `feature/saas-multitenancy` branch tip and the `ru-ai` branch starts from there). If Yandex production reliability turns out to be worse than expected, revert by checking out the tag.

Phases 2–4 are additive — each can be feature-flagged via a checkbox in `OrganizationsPage` if we want a slow rollout. For MVP, ship as-is.

## Definition of done per phase

- **Phase 1 done when:** all existing flows (translate, programme, ticket parse, voucher parse) function via Yandex; no Anthropic deps; audit log records `yandex-*` providers; manual test of one full group's document generation succeeds end-to-end.
- **Phase 2 done when:** a tourist submitting through the public form can upload all 4 file types, see them listed, replace and delete them; passport upload triggers auto-fill of relevant form fields.
- **Phase 3 done when:** the public form is a 7-step wizard with sidebar; all fields from the old flat form work; navigation persists state; submit produces the same payload structure as before.
- **Phase 4 done when:** managers can view, download, and delete files per submission/tourist.

---

## Self-Review Notes

**Spec coverage check:**
- ✅ Yandex everywhere (Phase 1)
- ✅ Drop Anthropic (Task 1.D1)
- ✅ Passport via Yandex (Task 1.C3)
- ✅ Multi-step form with sidebar (Phase 3)
- ✅ Per-step Next button (Phase 3)
- ✅ Passport upload + auto-fill (Tasks 2.E3, 2.F1, Phase 3 step 2)
- ✅ ИП checkbox auto-fill (Task 2.F2 — pending Q2)
- ✅ Ticket/voucher upload from form (Phase 3 step 6)
- ✅ Documents section on form (Task 3.W9)
- ✅ Admin sees uploaded docs (Phase 4)

**Open ambiguities flagged inline as Q1, Q2, Q3** — must be answered before the touched tasks start.

**Risks:**
- Yandex IAM rotation goroutine — if it fails silently, all AI calls degrade. Add explicit alerting / startup health-check.
- Multi-page PDF OCR loop is sequential — for an 8-page passport-spread it'd take 8× sync calls. Acceptable; if too slow, parallelize per-page calls.
- Form state persistence in `localStorage` — survives refresh but a different device wipes progress. Acceptable for MVP.
