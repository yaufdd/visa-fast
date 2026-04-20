# Custom-Form Workflow — Design Spec

**Date:** 2026-04-20
**Status:** Draft (awaiting user review)
**Author:** Brainstormed together with Claude
**Related git tag:** `v1.0-scan-workflow` (rollback point before this change)

---

## 1. Motivation

The current FujiTravel workflow requires tourists to upload scans of their
internal Russian passport, foreign passport, flight tickets and hotel
vouchers. Those scans are sent to Anthropic Claude for extraction of
structured data (Pass 1). This creates two problems:

1. **Legal exposure.** Storing and processing scans of Russian internal
   passports without explicit written consent is a risk under ФЗ-152. A
   checkbox alone is legally weak when scans of highly sensitive PII
   (internal passport, registration address) are involved.
2. **Third-party dependency for questionnaire.** Tourist questionnaire data
   lives in a Google Sheet, which is an external system.

This design replaces the scan-based Pass 1 with a **custom web form** that
tourists fill in directly, stores submissions in our PostgreSQL database,
and re-architects the AI pipeline so that **no single AI call ever sees a
tourist's passport data together with their name and address**.

## 2. Out-of-Scope

- Authentication/login for tourists (form is public, no account needed)
- Rate limiting on the public form (if abuse shows up, add fail2ban-style
  middleware later)
- Multilingual form (form is Russian-only — the audience is Russian-speaking
  tourists)
- Data migration from Google Sheets (clean start — old Sheets data is
  not imported)
- Refactor of existing live groups in the DB (clean drop — site is
  pre-launch, no production user data to preserve)
- Backwards compatibility with the scan-based workflow (rollback via
  `v1.0-scan-workflow` git tag if ever needed)

## 3. High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     PUBLIC FRONTEND                         │
│                                                             │
│    Tourist  ──► [Form /form] ──► POST /api/submissions      │
│                                         │                   │
│                                         ▼                   │
│                              ┌──────────────────────┐       │
│                              │ tourist_submissions  │       │
│                              │ (pool table)         │       │
│                              └──────────────────────┘       │
└─────────────────────────────────────────────────────────────┘
                                          ▲
                                          │ picks from pool
┌─────────────────────────────────────────│───────────────────┐
│                    ADMIN FRONTEND       │                   │
│                                         │                   │
│   Manager ──► GroupDetailPage ──► "Add tourist"             │
│                                         │                   │
│                                         ▼                   │
│                              AddFromDBModal                 │
│                                         │                   │
│                                         ▼                   │
│                              tourists (existing table)      │
│                                         │                   │
│                                         ▼                   │
│                    [Flights/hotels: scan OR manual JSON]    │
│                                         │                   │
│                                         ▼                   │
│                           "Generate documents"              │
└──────────────────────────────────────────│──────────────────┘
                                           ▼
              ┌────────────────────────────────────────────┐
              │             AI PIPELINE                    │
              │                                            │
              │  1. translate.go  (mini call, anonymized) │
              │     [Russian free-text strings]            │
              │                  → [English strings]       │
              │                                            │
              │  2. programme.go  (main call, no PII)     │
              │     [dates + hotels + flights]             │
              │                  → [programme array]       │
              │                                            │
              │  3. assembler.go (pure Go, deterministic)  │
              │     submission + translations + programme  │
              │                  → [final pass2.json]      │
              └────────────────────────────────────────────┘
                                │
                                ▼
                        Python docgen → .docx/.pdf
```

### 3.1 Key Principles

- **Layered separation.** Public (form, unauthenticated) and admin
  (authenticated) layers communicate only through the database. The
  public form has no visibility of groups, tourists, or documents.
- **AI privacy partitioning.** No AI call ever receives the linked
  triple `(name + passport + address)`. Each call sees only what its
  narrow task requires.
- **Deterministic assembly.** The final `pass2.json` is composed
  entirely by Go code from three sources (form, translations,
  programme). It is never constructed by an LLM.
- **Clean replacement.** `pass1.go`, Google Sheets integration, and the
  `AddFromSheetModal` are deleted. Internal-passport fields are now
  entered in the form.
- **Manual entry is equal-class.** Flights and hotels can be entered
  manually in the admin UI. Scan uploads are optional and produce
  equivalent data through the same code path.

### 3.2 Unchanged Components

- Groups / subgroups data model (`groups`, `subgroups`)
- Hotels table (`hotels`, `group_hotels`)
- Python docgen (`docgen/generate.py`) — receives identical `pass2.json`
- Word / PDF templates
- Deployment infrastructure (docker-compose, nginx, CI/CD workflow)

## 4. Database Schema

### 4.1 New Table: `tourist_submissions`

```sql
CREATE TABLE tourist_submissions (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  payload               JSONB NOT NULL,
  consent_accepted      BOOLEAN NOT NULL,
  consent_accepted_at   TIMESTAMPTZ NOT NULL,
  consent_version       TEXT NOT NULL,
  source                TEXT NOT NULL CHECK (source IN ('tourist', 'manager')),
  status                TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'attached', 'archived', 'deleted')),
  created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_submissions_status   ON tourist_submissions(status);
CREATE INDEX idx_submissions_created  ON tourist_submissions(created_at DESC);
CREATE INDEX idx_submissions_name_lat ON tourist_submissions ((payload ->> 'name_lat'));

-- Dedup guard: same foreign-passport number submitted twice on the same day
CREATE UNIQUE INDEX idx_submissions_dedup ON tourist_submissions
  ((payload ->> 'passport_number'), ((created_at AT TIME ZONE 'UTC')::DATE))
  WHERE status = 'pending';
```

### 4.2 `payload` JSON Shape (Russian keys, values as entered)

```json
{
  "name_lat": "IVANOV IVAN",
  "name_cyr": "Иванов Иван",
  "gender_ru": "Мужской",
  "birth_date": "15.03.1980",
  "marital_status_ru": "Женат/замужем",
  "place_of_birth_ru": "Москва, СССР",
  "nationality_ru": "Россия",
  "former_nationality_ru": "СССР",
  "passport_number": "750123456",
  "passport_type_ru": "Обычный",
  "issued_by_ru": "МВД 77810",
  "issue_date": "14.05.2020",
  "expiry_date": "14.05.2030",
  "home_address_ru": "Москва, ул. Ленина 5, кв. 12",
  "phone": "+7 (999) 123-45-67",
  "occupation_ru": "Директор по развитию",
  "employer_ru": "ООО Ромашка",
  "employer_address_ru": "Москва, ул. Тверская 10",
  "criminal_record_ru": "Нет",
  "been_to_japan_ru": "Да",
  "previous_visits_ru": "Май 2018",
  "maiden_name_ru": "",
  "internal_series": "4521",
  "internal_number": "120035",
  "internal_issued_ru": "12.03.2015",
  "internal_issued_by_ru": "ОУФМС России по г. Москве",
  "reg_address_ru": "Москва, ул. Ленина 5, кв. 12"
}
```

All string values are preserved as entered by the tourist (Russian
Cyrillic where applicable). The assembler later transliterates and
translates into the English/Latin form required for documents.

### 4.3 Modified Table: `tourists`

```sql
-- Drop legacy columns (clean start — site pre-launch)
ALTER TABLE tourists
  DROP COLUMN raw_json,
  DROP COLUMN matched_sheet_row,
  DROP COLUMN match_confirmed;

-- Add new columns
ALTER TABLE tourists
  ADD COLUMN submission_id         UUID REFERENCES tourist_submissions(id) ON DELETE SET NULL,
  ADD COLUMN submission_snapshot   JSONB,
  ADD COLUMN flight_data           JSONB,
  ADD COLUMN translations          JSONB;
```

The `ON DELETE SET NULL` on `submission_id` is what makes the erasure
endpoint in §8.4 safe: when a pool row is hard-deleted, attached
`tourists` rows lose their FK back but keep their `submission_snapshot`
(the frozen copy of payload taken at attach time). Deletion of the
snapshot itself is a separate explicit action.

- `submission_id` — FK back to the pool row (for audit / re-sync if the
  tourist updates the same submission later)
- `submission_snapshot` — frozen copy of `tourist_submissions.payload` at
  attach time, so later edits to the pool don't mutate attached
  tourists without explicit action
- `flight_data` — shape `{"arrival": {...}, "departure": {...}}`. Populated
  either by the manual-entry form (`PUT /api/tourists/:id/flight_data`)
  or by the ticket-scan parser.
- `translations` — cached output of `translate.go` mini-call, so repeated
  generation does not re-pay for translation

### 4.4 `uploads` Table — Restricted File Types

```sql
-- Clear legacy scan rows first (pre-launch — no production data)
DELETE FROM uploads WHERE file_type NOT IN ('ticket', 'voucher');

ALTER TABLE uploads
  ADD CONSTRAINT uploads_file_type_check
  CHECK (file_type IN ('ticket', 'voucher'));
```

The `DELETE` step is required because the `CHECK` constraint would
otherwise reject the migration against existing `passport` /
`foreign_passport` / `unknown` rows. Since the site is pre-launch,
deleting them is safe. In a post-launch re-do, this would become a
data-migration step instead.

### 4.5 Unchanged Tables

`groups`, `subgroups`, `hotels`, `group_hotels`, `documents` — no
schema changes.

## 5. API Endpoints

### 5.1 New — Public (No Auth)

| Method | Path                | Purpose                                                  |
| ------ | ------------------- | -------------------------------------------------------- |
| GET    | `/form`             | Serves the public form page (static SPA route)           |
| GET    | `/consent`          | Full consent text page (linked from form)                |
| POST   | `/api/submissions`  | Creates a new `tourist_submissions` row                  |
| GET    | `/api/consent/text` | Returns current consent text + version (for form render) |

**`POST /api/submissions` body:**

```json
{
  "payload": { ...all form fields... },
  "consent_accepted": true,
  "source": "tourist"
}
```

Response: `201 Created` with `{ "id": "uuid" }` on success; `409 Conflict`
on dedup; `400` on validation errors.

### 5.2 New — Admin

| Method | Path                              | Purpose                                       |
| ------ | --------------------------------- | --------------------------------------------- |
| GET    | `/api/submissions?q=&status=`     | List pool with search by name + status filter |
| GET    | `/api/submissions/:id`            | Single submission details                     |
| PUT    | `/api/submissions/:id`            | Manager edits submission (typo fixes)         |
| DELETE | `/api/submissions/:id`            | Archive (status → `archived`)                 |
| DELETE | `/api/submissions/:id/erase`      | Hard-delete (GDPR-style, on tourist request)  |
| POST   | `/api/submissions/:id/attach`     | Attach to group/subgroup → create `tourists`  |
| PUT    | `/api/tourists/:id/flight_data`   | Manual flight data entry / edit               |

**`POST /api/submissions/:id/attach` body:**

```json
{ "group_id": "uuid", "subgroup_id": "uuid | null" }
```

Transaction: `SELECT ... FOR UPDATE`, verifies `status != 'attached'`,
creates the tourist row, copies `payload → submission_snapshot`, sets
submission `status = 'attached'`.

**`PUT /api/tourists/:id/flight_data` body:**

```json
{
  "arrival":   { "flight_number": "SU 262", "date": "25.04.2026",
                 "time": "12:45", "airport": "TOKYO NARITA" },
  "departure": { "flight_number": "SU 263", "date": "05.05.2026",
                 "time": "14:20", "airport": "TOKYO NARITA" }
}
```

Empty `departure` object is accepted (one-way ticket).

### 5.3 Modified Endpoints

| Method | Path                          | Change                                                                                                      |
| ------ | ----------------------------- | ----------------------------------------------------------------------------------------------------------- |
| POST   | `/api/tourists/:id/uploads`   | Accepts only `file_type ∈ {ticket, voucher}`. Auto-triggers `ticket_parser` / `voucher_parser` after upload |
| POST   | `/api/groups/:id/generate`    | Internally rewritten — calls `translate.go` + `programme.go` + `assembler.go` instead of Pass 1/Pass 2      |
| POST   | `/api/groups/:id/finalize`    | Unchanged externally                                                                                        |

### 5.4 Removed Endpoints

- `GET /api/sheets/rows`
- `POST /api/tourists/:id/match`
- `POST /api/tourists/:id/parse`
- `POST /api/groups/:id/parse`
- `POST /api/groups/:id/uploads`

## 6. Frontend

### 6.1 New Pages

**`/form` (public).** Single scrolling page with accordion sections:

1. Personal data
2. Foreign passport
3. Internal Russian passport (new — this block was previously scan-only)
4. Work
5. Travel history
6. Consent checkbox + link to `/consent`

Client-side validation:

- `name_lat`: `/^[A-Z ]+$/`, uppercase only
- All dates: `DD.MM.YYYY`
- `internal_series`: exactly 4 digits
- `internal_number`: exactly 6 digits
- Foreign passport number: 9 characters (Russian standard)
- Consent checkbox: required (Submit disabled until checked)

Submit → `POST /api/submissions` → redirect to `/form/thanks`.

**`/submissions` (admin).** List view:

- Search by name (`q=`)
- Filter by status
- Actions: view details, "Create manually" button (opens form in
  admin wrapper with `source: "manager"` on submit)

**`/submissions/:id` (admin).** Detail + edit view:

- Same fields as public form, editable
- Buttons: Save, Archive, **Attach to group** (opens group/subgroup
  picker modal)

### 6.2 Modified Components

- `AddFromSheetModal` → `AddFromDBModal` — same UX, data source changes
  from Google Sheets to `GET /api/submissions?q=...&status=pending`
- `GroupDetailPage` — remove all passport-upload UI, add per-tourist
  `FlightDataCard` (upload ticket OR enter manually)

### 6.3 New Components

- `FlightDataForm` — modal for manual flight entry (4+4 fields, empty
  departure allowed)
- `SubmissionForm` — reusable component shared by `/form` and the admin
  "Create manually" flow; differs only in submit behavior

### 6.4 Removed Components

- `AddFromSheetModal.jsx`
- Passport upload drop zones
- `MatchConfirm*` components
- "Parse tourist" / "Parse group" buttons

### 6.5 Navigation

Main menu in admin adds one entry:

```
Groups | Hotels | Submissions
```

## 7. AI Pipeline

File layout in `backend/internal/ai/`:

```
ai/
├── translate.go       — mini call: translate free-text fields
├── programme.go       — main call: build programme array
├── ticket_parser.go   — optional: scan → flight_data
├── voucher_parser.go  — optional: scan → hotel list
├── assembler.go       — Go code: compose final pass2.json (NOT AI)
└── client.go          — shared Anthropic HTTP helpers (extracted from current pass1.go)
```

### 7.1 `translate.go` — Mini Translator

- **Trigger:** once per `Generate(groupID)` call. Collects all
  untranslated free-text fields across all tourists in the group into a
  single de-duplicated array, makes ONE batched call, then distributes
  results back per-tourist. Tourists whose `translations` JSONB already
  covers all their current free-text values are skipped (the cache key
  is the set of Russian source strings).
- **Input:** `{ "strings": ["Директор по развитию", "ООО Ромашка", ...] }`
- **Output:** `["Director of Development", "LLC Romashka", ...]`
  (same length, same order)
- **Model:** `claude-haiku-4-5`
- **Temperature:** `0`
- **Max tokens:** ~500 per batch
- **Caches to:** `tourists.translations` JSONB, keyed by the Russian source
- **Privacy:** no names, no passports, no dates — only anonymized
  free-text strings

### 7.2 `programme.go` — Main Programme Generator

- **Trigger:** during document generation, called once per group.
- **Input (no PII):**

  ```json
  {
    "arrival_date": "25.04.2026",
    "departure_date": "05.05.2026",
    "arrival_flight":   {"flight": "SU 262", "time": "12:45", "airport": "TOKYO NARITA"},
    "departure_flight": {"flight": "SU 263", "time": "14:20", "airport": "TOKYO NARITA"},
    "hotels": [
      {"name": "...", "city": "TOKYO", "address": "...", "phone": "...",
       "check_in": "25.04.2026", "check_out": "29.04.2026"}
    ],
    "contact_phone": "+7 999 123 45 67"
  }
  ```

- **Output:** programme array (day rows with `date / activity / contact /
  accommodation`)
- **Model:** `claude-opus-4-7` (creativity needed)
- **Temperature:** `0.3`
- **Max tokens:** ~3000
- **Prompt:** condensed Section 1 of the current Pass 2 prompt, only
  programme rules. No translation rules, no passport rules, no anketa
  rules.

### 7.3 `ticket_parser.go`

- **Trigger:** `POST /api/tourists/:id/uploads` with `file_type=ticket`,
  synchronously after file is stored.
- **Input:** one PDF/JPG file (uploaded via Anthropic Files API).
- **Output:** `{ arrival: {...}, departure: {...} }`
- **Model:** `claude-opus-4-6` (multimodal, accurate)
- **Prompt:** extracted from current Pass 1 prompt — only the flight
  block rules (last leg into Japan, first leg from Japan, one-way case).
- **Persistence:** writes to `tourists.flight_data`.

### 7.4 `voucher_parser.go`

- **Trigger:** `POST /api/tourists/:id/uploads` with `file_type=voucher`,
  or optional group-level batch endpoint.
- **Input:** one or more voucher files.
- **Output:** hotel list (`[{name, city, address, phone, check_in, check_out}]`)
- **Model:** `claude-opus-4-6`
- **Persistence:** our code looks up / creates `hotels` rows and inserts
  `group_hotels` rows.

### 7.5 `assembler.go` — Deterministic Go Assembly

No AI calls. Takes as input:

- `submission_snapshot` per tourist
- `translations` per tourist (from `translate.go`)
- `flight_data` per tourist
- Programme array (from `programme.go`)
- Group hotels

Produces the exact `pass2.json` structure expected by
`docgen/generate.py`.

Deterministic transformations handled in Go:

1. Name transliteration — ICAO Doc 9303 (Russian МВД standard)
2. Dictionary mappings — gender, marital status, passport type, yes/no
3. `former_nationality` rule — explicit / place_of_birth contains USSR /
   birth year ≤ 1991 / else "NO"
4. Country ISO code lookup (`"Россия" → "RUS"`)
5. PDF radio-button codes (gender_rb, marital_status_rb, passport_type_rb)
6. Free-text substitution from `translations`
7. `intended_stay_days` = `(departure - arrival) + 1` per tourist, `0`
   for one-way tickets
8. Flight field copy from `flight_data` to output per-tourist fields
9. First-hotel block for anketa
10. Doverenost — minor detection (age < 18 on departure date), parent
    match by first surname word, genitive case of child name via rule
    table, fallback suffix `[ПРОВЕРЬТЕ ПАДЕЖ]` for unhandled endings
11. vc_request / inna_doc / email — trivial string assembly

### 7.6 Orchestration

```go
func Generate(ctx context.Context, groupID uuid.UUID) (zipPath string, err error) {
    tourists, hotels, err := loadGroupData(ctx, groupID)
    if err != nil { return "", err }

    // translations and programme run in parallel — they have no
    // dependency on each other.
    var (
        translations map[TouristID]map[string]string
        programme    []ProgrammeDay
    )
    g, gctx := errgroup.WithContext(ctx)
    g.Go(func() error {
        t, err := ai.TranslateAll(gctx, tourists)
        translations = t
        return err
    })
    g.Go(func() error {
        p, err := ai.GenerateProgramme(gctx, hotels, tourists)
        programme = p
        return err
    })
    if err := g.Wait(); err != nil { return "", err }

    finalJSON := ai.Assemble(tourists, translations, programme, hotels)
    return docgen.Run(ctx, finalJSON)
}
```

### 7.7 What Each AI Call Sees

| Component          | Sees                                             |
| ------------------ | ------------------------------------------------ |
| `translate.go`     | Anonymized array of Russian strings              |
| `programme.go`     | Dates, hotels, flights, one contact phone        |
| `ticket_parser.go` | Ticket scan only (contains name + flight)        |
| `voucher_parser.go`| Voucher scan only (contains name + hotel)        |
| `assembler.go`     | Everything — but it's pure Go, no network calls  |
| `docgen` (Python)  | Final JSON — local subprocess, no network        |

**Invariant:** the triple `(full name + foreign passport + internal
passport + home address)` is never present together in any single
outbound AI call.

## 8. Consent & Legal

### 8.1 Consent Text Location

- Hardcoded constant in `backend/internal/consent/text.go` with a version
  string (e.g. `"2026.04"`).
- Rendered by both `/consent` (full page) and the form submit section
  (inline abbreviated).
- Change = new version string + new commit.

### 8.2 Text Content

Template covers (subject to lawyer review before production):

- Data categories listed explicitly (foreign passport, internal passport,
  work, travel history)
- Purposes (visa application, supporting documents, communication)
- Third parties (Japanese Embassy, VFS Global, Anthropic for anonymized
  free-text translation)
- Retention period (until withdrawal or legally mandated document
  retention period, whichever is longer)
- Right to withdraw (write to `tour@fujitravel.ru`)

### 8.3 Audit Fields Stored

- `consent_accepted: true`
- `consent_accepted_at: timestamptz`
- `consent_version: "2026.04"`
- **Not stored in MVP:** source IP, user agent (can be added if a lawyer
  advises; IP itself is arguably PII under some interpretations)

### 8.4 Right to Erasure

`DELETE /api/submissions/:id/erase` — admin-only endpoint. Physically
deletes the row from `tourist_submissions`. Per §4.3, attached
`tourists` rows lose `submission_id` (`ON DELETE SET NULL`) but keep
their `submission_snapshot`. The endpoint therefore also wipes
`submission_snapshot` on those `tourists` rows in the same transaction.
Attached documents (already generated `.docx` / `.pdf` on disk) are
left alone — they can be cleaned up separately if the tourist
explicitly requests destruction of already-issued documents.

An audit log entry (actor, submission id, timestamp) is written to an
append-only admin log. The exact mechanism (dedicated table vs.
structured application log) is chosen during implementation planning.

### 8.5 Scan Consent (Tickets / Vouchers)

Scan uploads happen in the admin UI, not from the public form. Manager
must tick an admin-side checkbox "client agreed to ticket/voucher
scanning" before uploading. Scans are auto-deleted N days after the
group moves to `visa_issued` status (cron job, N = 90 days default,
configurable via env var).

## 9. Error Handling

### 9.1 Form (Client)

- Field-level validation errors displayed inline.
- Submit disabled until consent checked.
- Network error on submit → toast; form data kept in localStorage so
  user can retry without re-entering.
- 500 response → generic "contact manager" toast.

### 9.2 Submissions API

- Dedup (same foreign passport + same day, status pending) → `409`.
- Missing required fields → `400` with list of missing keys.
- Invalid JSON → `400`.
- Attach race condition → `409` on second attempt.

### 9.3 AI Calls

- Reuse existing `callClaude` retry policy (exponential backoff,
  retries on 5xx and Cloudflare HTML pages, up to 4 attempts).
- `translate` or `programme` failure → generation aborts with a
  user-facing "try again" message; partial progress is not committed.
- `ticket_parser` failure → scan stored, `flight_data` left empty, UI
  shows "could not parse, please enter manually".
- `voucher_parser` failure → same fallback (manual entry).

### 9.4 Python docgen

Unchanged. Existing stderr/exit-code handling surfaces errors to API
response.

## 10. Edge Cases

1. **One-way ticket** — `flight_data.departure` is `null`;
   `intended_stay_days = 0`; programme has no Departure block; final
   day = "Free day" in the hotel.
2. **Different tourists, different flights** — each tourist's
   `flight_data` is independent; final JSON contains per-tourist
   `arrival_*` / `departure_*`. The shared programme uses the "first
   populated" tourist's flights (same as current Pass 2 behavior).
3. **Minor without parent in group** — `is_minor: true`, doverenost
   fields fall back to the minor's own data, UI surfaces a warning to
   the manager.
4. **Group with no hotels** — generation endpoint returns `400`
   "Add at least one hotel before generating".
5. **Tourist with no attached submission** — generation endpoint
   returns `400` listing tourists without `submission_snapshot`.
6. **Unhandled Russian name endings in genitive case** — assembler
   appends `[ПРОВЕРЬТЕ ПАДЕЖ]` to the field; manager sees and corrects
   in UI.
7. **Double submit on form** — front-end disables button after click;
   DB has partial unique index on `(passport_number, created_at::date)
   WHERE status = 'pending'`.
8. **Concurrent attach** — `SELECT FOR UPDATE` transaction protects
   against double-attach; loser sees `409`.
9. **Long addresses** — soft warning in UI at 500 chars; no hard DB
   limit (columns are TEXT).
10. **Non-Cyrillic / non-Latin symbols** — emoji and CJK characters
    stripped server-side; only Cyrillic + Latin + digits + punctuation
    allowed.

## 11. Testing Strategy

### 11.1 Unit Tests (Go)

Priority-1 coverage of `assembler.go`:

- `TranslitRuToLatICAO` — 20+ cases (`Щегловский → SHCHEGLOVSKIY`,
  `Ёлкина → ELKINA`, `Цой → TSOI`)
- `ComputeFormerNationality` — all five branches
- `ComputeIntendedStayDays` — normal, one-way, cross-month
- `GenitiveCase` — masculine / feminine / various endings +
  fallback tag
- `MinorDetection` — birthday before / after departure date
- `FindParent` — match, no match, multi-parent pick
- All dictionary mappings (gender, marital, passport type, yes/no,
  country ISO)

Priority-2: `consent/text.go` version read, rendering.

### 11.2 Integration Tests

- Stub HTTP server for Anthropic API
- Happy-path translate → assembler receives expected translations
- Malformed JSON response → proper error handling
- Programme timeout → retry → success
- Ticket parser on sample scan → expected `flight_data`

### 11.3 End-to-End Tests

- Tourist fills form → submission persisted in DB
- Manager opens admin → sees submission → attaches to group →
  tourist appears in group
- Manager clicks Generate → receives zip with documents

### 11.4 Manual QA Checklist

- Create group → add tourist → enter flight manually → generate →
  open .docx → verify name latin matches form input
- Same with ticket scan upload → verify `flight_data` auto-populated
- Minor tourist case → verify genitive case in доверенность

### 11.5 Regression Safety

- Freeze one "golden" `pass2.json` from a controlled group
- Assembler must produce it byte-identical (modulo timestamps) on
  every run

### 11.6 Not Automated

- Quality of programme itinerary (creative — human review per group)
- Word document visual rendering (opened manually in Word)
- Real Anthropic API calls in CI (expensive + flaky — use mocks)

## 12. Migration / Deployment

Site is pre-launch, so:

1. Run new migration (`000013_custom_form_workflow.sql`) which:
   - Creates `tourist_submissions` table and indexes
   - Drops `tourists.raw_json`, `tourists.matched_sheet_row`,
     `tourists.match_confirmed`
   - Adds `tourists.submission_id`, `submission_snapshot`,
     `flight_data`, `translations`
   - Adds file-type check on `uploads`
2. Remove env vars `GOOGLE_CREDENTIALS_PATH`, `GOOGLE_SHEET_ID` from
   `.env` and `docker-compose.prod.yml`.
3. Deploy new code. The `v1.0-scan-workflow` tag serves as the
   rollback point if anything catastrophic happens.

No data migration from Google Sheets or from existing DB rows is
performed.

## 13. File-Level Change Summary

### New Files

- `backend/internal/consent/text.go`
- `backend/internal/ai/translate.go`
- `backend/internal/ai/programme.go`
- `backend/internal/ai/ticket_parser.go`
- `backend/internal/ai/voucher_parser.go`
- `backend/internal/ai/assembler.go`
- `backend/internal/ai/client.go` (extracted from current `pass1.go`)
- `backend/internal/api/submissions.go`
- `backend/internal/api/flight_data.go`
- `backend/internal/translit/icao.go`
- `backend/migrations/000013_custom_form_workflow.{up,down}.sql`
- `frontend/src/pages/SubmissionFormPage.jsx` (public `/form`)
- `frontend/src/pages/SubmissionsListPage.jsx`
- `frontend/src/pages/SubmissionDetailPage.jsx`
- `frontend/src/pages/ConsentPage.jsx`
- `frontend/src/pages/FormThanksPage.jsx`
- `frontend/src/components/SubmissionForm.jsx` (shared)
- `frontend/src/components/FlightDataForm.jsx`
- `frontend/src/components/AddFromDBModal.jsx`

### Deleted Files

- `backend/internal/ai/pass1.go`
- `backend/internal/ai/pass2.go`
- `backend/internal/ai/files.go`
- `backend/internal/sheets/client.go` (whole `sheets` package)
- `backend/internal/matcher/fuzzy.go` (whole `matcher` package)
- `backend/internal/api/sheetsearch.go`
- `backend/internal/api/parse.go` (both group-parse and tourist-parse handlers)
- `frontend/src/components/AddFromSheetModal.jsx`
- Passport-related upload UI in `GroupDetailPage.jsx` (inline removal)

### Modified Files

- `backend/cmd/server/main.go` — route wiring changes (removed routes
  for sheets/parse/group-uploads, added submissions/flight_data routes)
- `backend/internal/api/generate.go` — full rewrite of the orchestration,
  no longer calls `pass1`/`pass2`; calls the new
  `translate`/`programme`/`assembler` pipeline
- `backend/internal/api/uploads.go` — keeps `UploadTouristFile` /
  `ListTouristUploads`; removes `UploadFile` (group-level) and
  `ListUploads` (if unused). Adds `file_type` validation to reject
  `passport` / `foreign_passport`. Triggers ticket/voucher parser
  synchronously after upload.
- `backend/internal/api/tourists.go` — drops match endpoint; adds the
  flight_data endpoint (can also live in the new `flight_data.go`)
- `frontend/src/pages/GroupDetailPage.jsx` — remove scan UI, add
  `FlightDataCard` per tourist
- `docker-compose.prod.yml` + `.env.example` — drop Google env vars
- `CLAUDE.md` — update project description to reflect new workflow

## 14. Open Questions / Deferred

- Lawyer review of consent text before production launch.
- Whether to log source IP / user agent on submission (legal call).
- Whether to add email confirmation to tourists after submission
  (nice-to-have, post-MVP).
- Whether tourists should be able to edit their submission after
  initial submit (token-based link — post-MVP).
- Russian morphology library for perfect genitive case (deferred;
  `[ПРОВЕРЬТЕ ПАДЕЖ]` fallback is acceptable for MVP).
