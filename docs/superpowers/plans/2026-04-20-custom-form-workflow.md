# Custom-Form Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the scan-based Pass 1 / Google-Sheets workflow with a privacy-preserving custom form. Tourists submit data via a public form, the pool lives in our Postgres DB, and the AI pipeline is re-architected so no single call sees full PII.

**Architecture:** See `docs/superpowers/specs/2026-04-20-custom-form-workflow-design.md`. Summary: new `tourist_submissions` table + pool UI, three-file AI pipeline (`translate.go` for free-text, `programme.go` for itinerary, `assembler.go` as deterministic Go code), optional ticket/voucher scan parsers. Old `pass1.go`, `pass2.go`, and Google Sheets integration are fully removed.

**Tech Stack:** Go 1.25, chi router, pgx/v5, Anthropic API (Opus 4.7 for programme, Haiku 4.5 for translate, Opus 4.6 for scan parsers), React + Vite, PostgreSQL 16.

**Rollback safety:** git tag `v1.0-scan-workflow` pins the pre-change state.

---

## Execution Order / Phases

Execute phases sequentially — later phases assume earlier ones compiled and tested.

1. **Phase A — DB Migration** (Tasks 1-2)
2. **Phase B — Backend Foundation Libraries** (Tasks 3-9) — pure Go, no network
3. **Phase C — AI HTTP Client Refactor** (Tasks 10-11)
4. **Phase D — New AI Pipeline** (Tasks 12-16)
5. **Phase E — New API Endpoints** (Tasks 17-25)
6. **Phase F — Modify Existing Handlers** (Tasks 26-28)
7. **Phase G — Route Wiring + Legacy Deletion** (Tasks 29-33)
8. **Phase H — Frontend Public Form** (Tasks 34-38)
9. **Phase I — Frontend Admin UI** (Tasks 39-45)
10. **Phase J — Deployment / Docs / QA** (Tasks 46-48)

Commit after every task's final step. Run `go build ./...` and `go test ./...` after each backend task.

---

## Phase A — DB Migration

### Task 1: Write migration SQL

**Files:**
- Create: `backend/migrations/000013_custom_form_workflow.up.sql`
- Create: `backend/migrations/000013_custom_form_workflow.down.sql`

- [ ] **Step 1: Write the up migration**

Create `backend/migrations/000013_custom_form_workflow.up.sql`:

```sql
-- Pool of form submissions (the "tourist list" the manager picks from)
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

-- Dedup guard: same foreign passport submitted twice on the same day while pending
CREATE UNIQUE INDEX idx_submissions_dedup ON tourist_submissions
  ((payload ->> 'passport_number'), ((created_at AT TIME ZONE 'UTC')::DATE))
  WHERE status = 'pending';

-- Drop legacy scan/sheet columns on tourists
ALTER TABLE tourists
  DROP COLUMN IF EXISTS raw_json,
  DROP COLUMN IF EXISTS matched_sheet_row,
  DROP COLUMN IF EXISTS match_confirmed;

-- Add new columns
ALTER TABLE tourists
  ADD COLUMN submission_id       UUID REFERENCES tourist_submissions(id) ON DELETE SET NULL,
  ADD COLUMN submission_snapshot JSONB,
  ADD COLUMN flight_data         JSONB,
  ADD COLUMN translations        JSONB;

-- Clean legacy passport-scan upload rows before tightening file_type constraint
DELETE FROM uploads WHERE file_type NOT IN ('ticket', 'voucher');

ALTER TABLE uploads
  ADD CONSTRAINT uploads_file_type_check
  CHECK (file_type IN ('ticket', 'voucher'));
```

- [ ] **Step 2: Write the down migration**

Create `backend/migrations/000013_custom_form_workflow.down.sql`:

```sql
ALTER TABLE uploads DROP CONSTRAINT IF EXISTS uploads_file_type_check;

ALTER TABLE tourists
  DROP COLUMN IF EXISTS translations,
  DROP COLUMN IF EXISTS flight_data,
  DROP COLUMN IF EXISTS submission_snapshot,
  DROP COLUMN IF EXISTS submission_id;

ALTER TABLE tourists
  ADD COLUMN raw_json JSONB,
  ADD COLUMN matched_sheet_row JSONB,
  ADD COLUMN match_confirmed BOOLEAN DEFAULT FALSE;

DROP TABLE IF EXISTS tourist_submissions;
```

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000013_custom_form_workflow.up.sql backend/migrations/000013_custom_form_workflow.down.sql
git commit -m "feat(db): migration 000013 — tourist_submissions pool + drop legacy tourist columns"
```

---

### Task 2: Apply and verify migration locally

**Files:**
- Modify (state only): local DB schema

- [ ] **Step 1: Ensure DB is running**

Run: `docker compose up -d db`

- [ ] **Step 2: Apply migration**

Run:
```bash
cd /Users/yaufdd/Desktop/fujitravel-admin
migrate -path backend/migrations -database "$DATABASE_URL" up
```
Expected: `13/u custom_form_workflow (NNNms)`.

- [ ] **Step 3: Verify schema**

Run:
```bash
psql "$DATABASE_URL" -c '\d tourist_submissions'
psql "$DATABASE_URL" -c '\d tourists'
```
Expected: `tourist_submissions` table exists with 8 columns; `tourists` table has `submission_id`, `submission_snapshot`, `flight_data`, `translations`; no `raw_json` / `matched_sheet_row` / `match_confirmed`.

- [ ] **Step 4: Verify the down migration works, then re-apply up**

Run:
```bash
migrate -path backend/migrations -database "$DATABASE_URL" down 1
migrate -path backend/migrations -database "$DATABASE_URL" up
```
Expected: both succeed without errors.

---

## Phase B — Backend Foundation Libraries

These are pure Go, no network, fully unit-testable. Write tests first.

### Task 3: ICAO transliteration library

**Files:**
- Create: `backend/internal/translit/icao.go`
- Create: `backend/internal/translit/icao_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/translit/icao_test.go`:

```go
package translit

import "testing"

func TestRuToLatICAO(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Иванов Иван", "IVANOV IVAN"},
		{"Петров Сергей", "PETROV SERGEI"},
		{"Щегловский", "SHCHEGLOVSKII"},
		{"Ёлкина", "ELKINA"},
		{"Цой", "TCOI"},
		{"Жуков", "ZHUKOV"},
		{"Юрий Хромов", "IURII KHROMOV"},
		{"Яковлев", "IAKOVLEV"},
		{"Вячеслав", "VIACHESLAV"},
		{"Анна", "ANNA"},
		{"Мария", "MARIIA"},
		{"Абвгдеёжзийклмнопрстуфхцчшщъыьэюя",
			"ABVGDEELZHZIIKLMNOPRSTUFKHTCCHSHSHCHIEIUIA"},
		{"", ""},
		{"IVANOV", "IVANOV"}, // already Latin
	}
	for _, c := range cases {
		got := RuToLatICAO(c.in)
		if got != c.want {
			t.Errorf("RuToLatICAO(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Verify the test fails**

Run: `cd backend && go test ./internal/translit/...`
Expected: package not found / function not defined.

- [ ] **Step 3: Implement the library**

Create `backend/internal/translit/icao.go`:

```go
// Package translit implements Russian → Latin transliteration per
// ICAO Doc 9303 (the Russian MVD standard used on passports).
package translit

import (
	"strings"
	"unicode"
)

// icaoMap maps one Cyrillic rune to its Latin equivalent. Multi-letter
// digraphs (zh, kh, ts, ch, sh, shch, iu, ia) are produced naturally by
// this mapping per ICAO Doc 9303.
var icaoMap = map[rune]string{
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D",
	'Е': "E", 'Ё': "E", 'Ж': "ZH", 'З': "Z", 'И': "I",
	'Й': "I", 'К': "K", 'Л': "L", 'М': "M", 'Н': "N",
	'О': "O", 'П': "P", 'Р': "R", 'С': "S", 'Т': "T",
	'У': "U", 'Ф': "F", 'Х': "KH", 'Ц': "TC", 'Ч': "CH",
	'Ш': "SH", 'Щ': "SHCH", 'Ъ': "IE", 'Ы': "Y", 'Ь': "",
	'Э': "E", 'Ю': "IU", 'Я': "IA",
}

// RuToLatICAO transliterates Russian Cyrillic to uppercase Latin using
// the ICAO Doc 9303 rules. Non-Cyrillic characters (Latin, digits,
// punctuation, whitespace) are preserved as-is but uppercased.
// Lowercase Cyrillic is handled by uppercasing first, so "Иван" and
// "иван" produce the same output.
func RuToLatICAO(s string) string {
	if s == "" {
		return ""
	}
	upper := strings.ToUpper(s)
	var b strings.Builder
	b.Grow(len(upper) * 2)
	for _, r := range upper {
		if v, ok := icaoMap[r]; ok {
			b.WriteString(v)
			continue
		}
		if unicode.IsSpace(r) || unicode.IsDigit(r) || unicode.IsPunct(r) ||
			(r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/translit/... -v`
Expected: all cases pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/translit
git commit -m "feat(translit): ICAO Doc 9303 Russian→Latin transliteration"
```

---

### Task 4: Dictionary mappings (gender, marital, passport, yes/no, country)

**Files:**
- Create: `backend/internal/ai/mappings.go`
- Create: `backend/internal/ai/mappings_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/ai/mappings_test.go`:

```go
package ai

import "testing"

func TestGenderMap(t *testing.T) {
	cases := map[string]string{
		"Мужской":  "Male",
		"мужской":  "Male",
		"Женский":  "Female",
		"":         "",
		"Unknown":  "",
	}
	for in, want := range cases {
		if got := MapGender(in); got != want {
			t.Errorf("MapGender(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaritalMap(t *testing.T) {
	cases := map[string]string{
		"Холост/не замужем": "Single",
		"Женат/замужем":     "Married",
		"Вдовец/вдова":      "Widowed",
		"Разведен(а)":       "Divorced",
		"":                   "",
	}
	for in, want := range cases {
		if got := MapMaritalStatus(in); got != want {
			t.Errorf("MapMaritalStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPassportTypeMap(t *testing.T) {
	cases := map[string]string{
		"Обычный":         "Ordinary",
		"Дипломатический": "Diplomatic",
		"Служебный":       "Official",
		"":                 "Ordinary", // default fallback
	}
	for in, want := range cases {
		if got := MapPassportType(in); got != want {
			t.Errorf("MapPassportType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestYesNoMap(t *testing.T) {
	cases := map[string]string{
		"Нет": "No",
		"Да":  "Yes",
		"":    "No",
	}
	for in, want := range cases {
		if got := MapYesNo(in); got != want {
			t.Errorf("MapYesNo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountryISO(t *testing.T) {
	cases := map[string]string{
		"Россия":     "RUS",
		"РФ":         "RUS",
		"Казахстан":  "KAZ",
		"Беларусь":   "BLR",
		"Украина":    "UKR",
		"Unknown":    "",
	}
	for in, want := range cases {
		if got := CountryISO(in); got != want {
			t.Errorf("CountryISO(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRadioButtonCodes(t *testing.T) {
	if got := GenderRB("Male"); got != "0" {
		t.Errorf("GenderRB(Male) = %q, want 0", got)
	}
	if got := GenderRB("Female"); got != "1" {
		t.Errorf("GenderRB(Female) = %q, want 1", got)
	}
	if got := MaritalRB("Married"); got != "1" {
		t.Errorf("MaritalRB(Married) = %q, want 1", got)
	}
	if got := PassportTypeRB("Ordinary"); got != "2" {
		t.Errorf("PassportTypeRB(Ordinary) = %q, want 2", got)
	}
}
```

- [ ] **Step 2: Verify tests fail**

Run: `cd backend && go test ./internal/ai/... -run TestGenderMap`
Expected: `undefined: MapGender`.

- [ ] **Step 3: Implement mappings**

Create `backend/internal/ai/mappings.go`:

```go
package ai

import "strings"

// MapGender converts Russian gender values to English.
func MapGender(ru string) string {
	switch strings.ToLower(strings.TrimSpace(ru)) {
	case "мужской", "м":
		return "Male"
	case "женский", "ж":
		return "Female"
	}
	return ""
}

// MapMaritalStatus converts Russian marital status to English.
func MapMaritalStatus(ru string) string {
	switch strings.TrimSpace(ru) {
	case "Холост/не замужем", "Холост", "Не замужем":
		return "Single"
	case "Женат/замужем", "Женат", "Замужем":
		return "Married"
	case "Вдовец/вдова", "Вдовец", "Вдова":
		return "Widowed"
	case "Разведен(а)", "Разведен", "Разведена":
		return "Divorced"
	}
	return ""
}

// MapPassportType converts Russian passport type. Empty/unknown defaults
// to "Ordinary" (the overwhelming majority of Russian passports).
func MapPassportType(ru string) string {
	switch strings.TrimSpace(ru) {
	case "Дипломатический":
		return "Diplomatic"
	case "Служебный":
		return "Official"
	default:
		return "Ordinary"
	}
}

// MapYesNo normalises Russian Yes/No. Empty defaults to "No".
func MapYesNo(ru string) string {
	switch strings.ToLower(strings.TrimSpace(ru)) {
	case "да":
		return "Yes"
	}
	return "No"
}

// countryISOMap covers post-Soviet countries and common Russian
// passport-issued nationalities. Extend as needed.
var countryISOMap = map[string]string{
	"Россия":          "RUS",
	"РФ":              "RUS",
	"Российская Федерация": "RUS",
	"Казахстан":       "KAZ",
	"Беларусь":        "BLR",
	"Белоруссия":      "BLR",
	"Украина":         "UKR",
	"Узбекистан":      "UZB",
	"Киргизия":        "KGZ",
	"Кыргызстан":      "KGZ",
	"Таджикистан":     "TJK",
	"Туркменистан":    "TKM",
	"Армения":         "ARM",
	"Азербайджан":     "AZE",
	"Грузия":          "GEO",
	"Молдова":         "MDA",
	"Латвия":          "LVA",
	"Литва":           "LTU",
	"Эстония":         "EST",
}

// CountryISO returns the 3-letter ISO code for a Russian country name.
// Empty string if unknown.
func CountryISO(ru string) string {
	return countryISOMap[strings.TrimSpace(ru)]
}

// PDF radio-button codes (from existing docgen/generate.py contract)

// GenderRB: "0" = Male, "1" = Female
func GenderRB(gender string) string {
	if gender == "Female" {
		return "1"
	}
	return "0"
}

// MaritalRB: "0" = Single, "1" = Married, "2" = Widowed, "3" = Divorced
func MaritalRB(status string) string {
	switch status {
	case "Married":
		return "1"
	case "Widowed":
		return "2"
	case "Divorced":
		return "3"
	}
	return "0"
}

// PassportTypeRB: "0" = Diplomatic, "1" = Official, "2" = Ordinary, "3" = Other
func PassportTypeRB(pt string) string {
	switch pt {
	case "Diplomatic":
		return "0"
	case "Official":
		return "1"
	case "Other":
		return "3"
	}
	return "2"
}
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/ai/... -run '^Test(GenderMap|MaritalMap|PassportTypeMap|YesNoMap|CountryISO|RadioButtonCodes)$' -v`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ai/mappings.go backend/internal/ai/mappings_test.go
git commit -m "feat(ai): dictionary mappings (gender, marital, passport, country ISO)"
```

---

### Task 5: Former-nationality logic

**Files:**
- Create: `backend/internal/ai/nationality.go`
- Create: `backend/internal/ai/nationality_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/ai/nationality_test.go`:

```go
package ai

import "testing"

func TestComputeFormerNationality(t *testing.T) {
	cases := []struct {
		name           string
		formerRu       string
		placeOfBirthRu string
		birthDate      string // DD.MM.YYYY
		want           string
	}{
		{"explicit USSR", "СССР", "Москва", "15.03.1970", "USSR"},
		{"explicit Soviet Union", "Soviet Union", "Moscow", "15.03.1970", "USSR"},
		{"USSR in place of birth", "", "Москва, СССР", "15.03.1970", "USSR"},
		{"born before end of USSR (Dec 1991)", "", "Москва", "15.03.1985", "USSR"},
		{"born on dissolution day", "", "Москва", "25.12.1991", "USSR"},
		{"born after USSR", "", "Москва", "15.03.1995", "NO"},
		{"empty everything", "", "", "", "NO"},
		{"malformed birth date, no USSR elsewhere", "", "Almaty", "garbage", "NO"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeFormerNationality(c.formerRu, c.placeOfBirthRu, c.birthDate)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `cd backend && go test ./internal/ai/... -run TestComputeFormerNationality`
Expected: `undefined: ComputeFormerNationality`.

- [ ] **Step 3: Implement**

Create `backend/internal/ai/nationality.go`:

```go
package ai

import (
	"strings"
	"time"
)

// ussrDissolution is 25 December 1991 — last day the USSR existed.
var ussrDissolution = time.Date(1991, 12, 25, 23, 59, 59, 0, time.UTC)

// ComputeFormerNationality applies the rules:
//  1. explicit "СССР" / "Soviet" in formerRu     → "USSR"
//  2. "СССР" / "Soviet" / "USSR" in placeOfBirth → "USSR"
//  3. birth date on or before 25.12.1991         → "USSR"
//  4. otherwise                                   → "NO"
func ComputeFormerNationality(formerRu, placeOfBirthRu, birthDate string) string {
	if containsUSSR(formerRu) {
		return "USSR"
	}
	if containsUSSR(placeOfBirthRu) {
		return "USSR"
	}
	if t, err := time.Parse("02.01.2006", strings.TrimSpace(birthDate)); err == nil {
		if !t.After(ussrDissolution) {
			return "USSR"
		}
	}
	return "NO"
}

func containsUSSR(s string) bool {
	lower := strings.ToLower(s)
	return strings.Contains(lower, "ссср") ||
		strings.Contains(lower, "soviet") ||
		strings.Contains(lower, "ussr")
}
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/ai/... -run TestComputeFormerNationality -v`
Expected: all cases pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ai/nationality.go backend/internal/ai/nationality_test.go
git commit -m "feat(ai): former-nationality USSR/NO rule"
```

---

### Task 6: Intended-stay-days calculator

**Files:**
- Create: `backend/internal/ai/stay_days.go`
- Create: `backend/internal/ai/stay_days_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/ai/stay_days_test.go`:

```go
package ai

import "testing"

func TestComputeIntendedStayDays(t *testing.T) {
	cases := []struct {
		name           string
		arrival, dep   string
		want           int
	}{
		{"normal", "25.04.2026", "05.05.2026", 11},
		{"same day", "25.04.2026", "25.04.2026", 1},
		{"one-way empty departure", "25.04.2026", "", 0},
		{"cross-month", "30.04.2026", "02.05.2026", 3},
		{"invalid arrival", "garbage", "05.05.2026", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeIntendedStayDays(c.arrival, c.dep)
			if got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Implement**

Create `backend/internal/ai/stay_days.go`:

```go
package ai

import "time"

// ComputeIntendedStayDays returns (departure - arrival) + 1 in days.
// Returns 0 for one-way tickets (empty departure) or unparseable dates.
// Dates are DD.MM.YYYY.
func ComputeIntendedStayDays(arrival, departure string) int {
	if departure == "" {
		return 0
	}
	a, aErr := time.Parse("02.01.2006", arrival)
	d, dErr := time.Parse("02.01.2006", departure)
	if aErr != nil || dErr != nil {
		return 0
	}
	return int(d.Sub(a).Hours()/24) + 1
}
```

- [ ] **Step 3: Run tests**

Run: `cd backend && go test ./internal/ai/... -run TestComputeIntendedStayDays -v`
Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/ai/stay_days.go backend/internal/ai/stay_days_test.go
git commit -m "feat(ai): intended-stay-days calculator"
```

---

### Task 7: Russian genitive case for doverenost child names

**Files:**
- Create: `backend/internal/ai/genitive.go`
- Create: `backend/internal/ai/genitive_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/ai/genitive_test.go`:

```go
package ai

import "testing"

func TestGenitiveCase(t *testing.T) {
	cases := []struct {
		name      string
		surname   string
		firstName string
		gender    string // "Male" / "Female"
		wantName  string
		wantTag   bool   // if true, expect "[ПРОВЕРЬТЕ ПАДЕЖ]" suffix
	}{
		{"male consonant surname + consonant first", "Кузнецов", "Александр", "Male",
			"Кузнецова Александра", false},
		{"male -й first name", "Иванов", "Андрей", "Male",
			"Иванова Андрея", false},
		{"female -а surname, consonant first", "Кузнецова", "Анна", "Female",
			"Кузнецовой Анны", false},
		{"female -ая surname, -я first", "Преображенская", "Мария", "Female",
			"Преображенской Марии", false},
		{"female -а after ш", "Петрова", "Даша", "Female",
			"Петровой Даши", false},
		{"edge: unknown ending", "Ким", "Пак", "Male",
			"Ким Пак [ПРОВЕРЬТЕ ПАДЕЖ]", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, tagged := GenitiveFullName(c.surname, c.firstName, c.gender)
			if got != c.wantName {
				t.Errorf("name got %q, want %q", got, c.wantName)
			}
			if tagged != c.wantTag {
				t.Errorf("tagged got %v, want %v", tagged, c.wantTag)
			}
		})
	}
}
```

- [ ] **Step 2: Implement**

Create `backend/internal/ai/genitive.go`:

```go
package ai

import "strings"

// GenitiveFullName returns the surname+firstName in Russian genitive case
// (родительный падеж — отвечает на вопрос "кого?"), used for doverenost
// of a minor tourist. The second return is true if the algorithm was
// unable to handle the ending and the caller should add the suffix
// "[ПРОВЕРЬТЕ ПАДЕЖ]" — already appended in the first return.
// Rules cover the vast majority of Russian names; exotic endings fall
// back to the tagged form for manager review.
func GenitiveFullName(surname, firstName, gender string) (string, bool) {
	gSurname, okSurname := genitiveWord(surname, gender, true)
	gFirst, okFirst := genitiveWord(firstName, gender, false)
	full := strings.TrimSpace(gSurname + " " + gFirst)
	if !okSurname || !okFirst {
		return full + " [ПРОВЕРЬТЕ ПАДЕЖ]", true
	}
	return full, false
}

// genitiveWord returns genitive form of one word + a success flag.
func genitiveWord(word, gender string, isSurname bool) (string, bool) {
	if word == "" {
		return "", true
	}
	runes := []rune(word)
	last := runes[len(runes)-1]
	switch gender {
	case "Male":
		// consonant ending → add "а"
		if isRussianConsonant(last) {
			return word + "а", true
		}
		// ending "й" → replace with "я"
		if last == 'й' {
			return string(runes[:len(runes)-1]) + "я", true
		}
	case "Female":
		if isSurname {
			// -ая → -ой
			if len(runes) >= 2 && runes[len(runes)-2] == 'а' && last == 'я' {
				return string(runes[:len(runes)-2]) + "ой", true
			}
			// -а → -ой
			if last == 'а' {
				return string(runes[:len(runes)-1]) + "ой", true
			}
		} else {
			// first name ending "я" → "и"
			if last == 'я' {
				return string(runes[:len(runes)-1]) + "и", true
			}
			// first name ending "а" after ж/ш/щ/ч → "и"; otherwise → "ы"
			if last == 'а' {
				if len(runes) >= 2 && isSoftConsonant(runes[len(runes)-2]) {
					return string(runes[:len(runes)-1]) + "и", true
				}
				return string(runes[:len(runes)-1]) + "ы", true
			}
		}
	}
	return word, false
}

func isRussianConsonant(r rune) bool {
	const consonants = "бвгджзйклмнпрстфхцчшщБВГДЖЗЙКЛМНПРСТФХЦЧШЩ"
	return strings.ContainsRune(consonants, r)
}

func isSoftConsonant(r rune) bool {
	return strings.ContainsRune("жшщчЖШЩЧ", r)
}
```

- [ ] **Step 3: Run tests**

Run: `cd backend && go test ./internal/ai/... -run TestGenitiveCase -v`
Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/ai/genitive.go backend/internal/ai/genitive_test.go
git commit -m "feat(ai): Russian genitive case for doverenost child names"
```

---

### Task 8: Minor detection + parent finder

**Files:**
- Create: `backend/internal/ai/minor.go`
- Create: `backend/internal/ai/minor_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/ai/minor_test.go`:

```go
package ai

import "testing"

func TestIsMinorOnDate(t *testing.T) {
	cases := []struct {
		birth, departure string
		want             bool
	}{
		{"01.01.2010", "01.01.2026", true},  // 15, 16 on next bday after dep
		{"15.03.2008", "14.03.2026", true},  // turns 18 the next day
		{"15.03.2008", "15.03.2026", false}, // turns 18 on departure day
		{"15.03.2008", "16.03.2026", false},
		{"01.01.1990", "01.01.2026", false},
		{"garbage", "01.01.2026", false},
	}
	for _, c := range cases {
		got := IsMinorOnDate(c.birth, c.departure)
		if got != c.want {
			t.Errorf("IsMinorOnDate(%q,%q) = %v, want %v", c.birth, c.departure, got, c.want)
		}
	}
}

func TestFindParentBySurname(t *testing.T) {
	tourists := []TouristRef{
		{ID: "a", SurnameCyr: "Кузнецов", IsMinor: true, BirthDate: "01.01.2015"},
		{ID: "b", SurnameCyr: "Кузнецов", IsMinor: false, BirthDate: "01.01.1985"},
		{ID: "c", SurnameCyr: "Иванов", IsMinor: false, BirthDate: "01.01.1980"},
	}
	got := FindParent(tourists[0], tourists)
	if got == nil {
		t.Fatal("expected parent found")
	}
	if got.ID != "b" {
		t.Errorf("expected parent b, got %s", got.ID)
	}

	// No parent case
	tourists2 := []TouristRef{
		{ID: "x", SurnameCyr: "Петров", IsMinor: true, BirthDate: "01.01.2015"},
	}
	if FindParent(tourists2[0], tourists2) != nil {
		t.Error("expected nil when no parent in group")
	}
}
```

- [ ] **Step 2: Implement**

Create `backend/internal/ai/minor.go`:

```go
package ai

import (
	"strings"
	"time"
)

// TouristRef is a thin reference to a tourist used for minor-detection /
// parent-matching logic. It intentionally avoids coupling to the full
// Tourist DB type so this package stays DB-free.
type TouristRef struct {
	ID         string
	SurnameCyr string // first word of name_cyr
	BirthDate  string // DD.MM.YYYY
	IsMinor    bool
}

// IsMinorOnDate returns true if the tourist is under 18 as of the given
// departure date. Dates are DD.MM.YYYY. Unparseable dates → false.
func IsMinorOnDate(birthDate, departureDate string) bool {
	birth, bErr := time.Parse("02.01.2006", strings.TrimSpace(birthDate))
	dep, dErr := time.Parse("02.01.2006", strings.TrimSpace(departureDate))
	if bErr != nil || dErr != nil {
		return false
	}
	eighteenth := birth.AddDate(18, 0, 0)
	// Minor if they haven't reached their 18th birthday by departure.
	return dep.Before(eighteenth)
}

// FindParent returns the first adult tourist in the group with the same
// surname as the minor, or nil if none found.
func FindParent(minor TouristRef, group []TouristRef) *TouristRef {
	target := strings.ToLower(strings.TrimSpace(minor.SurnameCyr))
	if target == "" {
		return nil
	}
	for i := range group {
		t := group[i]
		if t.ID == minor.ID || t.IsMinor {
			continue
		}
		if strings.ToLower(strings.TrimSpace(t.SurnameCyr)) == target {
			return &t
		}
	}
	return nil
}

// FirstWord returns the first whitespace-separated word of a string,
// useful for extracting surname from "Фамилия Имя".
func FirstWord(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexAny(s, " \t"); idx >= 0 {
		return s[:idx]
	}
	return s
}
```

- [ ] **Step 3: Run tests**

Run: `cd backend && go test ./internal/ai/... -run 'TestIsMinorOnDate|TestFindParentBySurname' -v`
Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/ai/minor.go backend/internal/ai/minor_test.go
git commit -m "feat(ai): minor detection + parent finder by surname"
```

---

### Task 9: Consent text constant

**Files:**
- Create: `backend/internal/consent/text.go`
- Create: `backend/internal/consent/text_test.go`

- [ ] **Step 1: Write failing test**

Create `backend/internal/consent/text_test.go`:

```go
package consent

import "testing"

func TestCurrentReturnsNonEmpty(t *testing.T) {
	c := Current()
	if c.Version == "" {
		t.Error("consent version is empty")
	}
	if len(c.Body) < 500 {
		t.Error("consent body too short, expected full legal text")
	}
}
```

- [ ] **Step 2: Implement**

Create `backend/internal/consent/text.go`:

```go
// Package consent holds the versioned text of the data-processing
// consent shown to tourists before they submit the form.
package consent

// Agreement is a single consent version.
type Agreement struct {
	Version string
	Body    string // plain-text / markdown
}

// currentVersion bumps whenever the text changes.
const currentVersion = "2026.04"

const currentBody = `Согласие на обработку персональных данных

Я, заполняя настоящую форму, в соответствии с Федеральным законом от
27.07.2006 № 152-ФЗ «О персональных данных», даю согласие ООО «FujiTravel»
(далее — Оператор) на обработку моих персональных данных, в том числе:

Перечень обрабатываемых данных:
— фамилия, имя (латиницей и кириллицей);
— пол, дата и место рождения;
— гражданство, прежнее гражданство;
— данные загранпаспорта (номер, срок действия, орган выдачи);
— данные внутреннего паспорта РФ (серия, номер, орган выдачи, адрес регистрации);
— контактные данные (адрес, телефон);
— сведения о работе (должность, работодатель);
— сведения о семейном положении;
— сведения о судимости и предыдущих визитах в Японию.

Цели обработки:
— оформление туристической визы в Японию через Посольство Японии
  и уполномоченный визовый центр;
— формирование сопроводительных документов (программа пребывания,
  доверенность, заявка в визовый центр);
— коммуникация по вопросам, связанным с поездкой.

Способы обработки: автоматизированная и неавтоматизированная обработка,
включая сбор, запись, систематизацию, накопление, хранение, уточнение,
использование, передачу (предоставление в Посольство Японии и визовый
центр), удаление.

Передача третьим лицам:
— Посольство Японии и уполномоченный визовый центр;
— поставщик облачного ИИ-сервиса Anthropic — только обезличенные
  фрагменты текста свободных полей (описание должности, адреса) для
  перевода на английский язык. ФИО, паспортные данные, номер телефона
  и привязка к конкретному туристу в облачный сервис не передаются.

Срок обработки: до отзыва согласия, но не менее срока, установленного
законодательством для хранения визовых документов.

Я подтверждаю, что предоставленные данные являются моими и достоверными.
Я понимаю, что могу отозвать настоящее согласие, направив запрос на
tour@fujitravel.ru.
`

// Current returns the current agreement (version + body).
func Current() Agreement {
	return Agreement{Version: currentVersion, Body: currentBody}
}
```

- [ ] **Step 3: Run tests**

Run: `cd backend && go test ./internal/consent/... -v`
Expected: pass.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/consent
git commit -m "feat(consent): hardcoded consent text v2026.04"
```

---

## Phase C — AI HTTP Client Refactor

Goal: extract the shared Anthropic HTTP request helper from the current
`pass1.go` into `client.go`, leaving `pass1.go` still compiling.

### Task 10: Extract shared client into `client.go`

**Files:**
- Create: `backend/internal/ai/client.go`
- Modify: `backend/internal/ai/pass1.go` (remove the extracted code; import from client.go inside same package — same package, no import needed)

- [ ] **Step 1: Read existing pass1.go**

Read `backend/internal/ai/pass1.go`. Identify:
- Types: `anthropicRequest`, `anthropicMessage`, `anthropicContent`, `contentSource`, `anthropicResponse`, `FileInput`
- Functions: `callClaude`, `extractJSON`, `isJSONResponse`, `summarizeUpstreamBody`, `sleepCtx`, `isTransientHTTP`
- Constants: `claudeModel`, `anthropicAPI`

- [ ] **Step 2: Create client.go with the extracted common code**

Create `backend/internal/ai/client.go`. Move the following from `pass1.go`:

```go
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Anthropic API constants. Model ids are overridable at call sites.
const (
	AnthropicAPI            = "https://api.anthropic.com/v1/messages"
	ModelOpusParser         = "claude-opus-4-6"  // ticket/voucher scan parsing
	ModelOpusProgramme      = "claude-opus-4-7"  // creative itinerary
	ModelHaikuTranslate     = "claude-haiku-4-5" // free-text translation
	anthropicVersionHeader  = "2023-06-01"
	anthropicBetaFilesHdr   = "files-api-2025-04-14"
)

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	System      string             `json:"system"`
	Messages    []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicContent struct {
	Type   string         `json:"type"`
	Text   string         `json:"text,omitempty"`
	Source *contentSource `json:"source,omitempty"`
}

type contentSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	FileID    string `json:"file_id,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// FileInput represents one file to send to Claude (either an Anthropic
// Files API reference or raw inline bytes).
type FileInput struct {
	AnthropicFileID string
	Name            string
	Data            []byte
}

// callClaude POSTs to /v1/messages with retries on transient failures.
func callClaude(ctx context.Context, apiKey string, reqBody anthropicRequest) (string, error) {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal claude request: %w", err)
	}

	client := &http.Client{Timeout: 300 * time.Second}
	backoff := 1 * time.Second
	const maxAttempts = 4

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, rerr := http.NewRequestWithContext(ctx, http.MethodPost, AnthropicAPI, bytes.NewReader(bodyBytes))
		if rerr != nil {
			return "", fmt.Errorf("build claude request: %w", rerr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", anthropicVersionHeader)
		req.Header.Set("anthropic-beta", anthropicBetaFilesHdr)

		resp, derr := client.Do(req)
		if derr != nil {
			lastErr = fmt.Errorf("claude http (attempt %d/%d): %w", attempt, maxAttempts, derr)
			if !isTransientHTTP(derr) || attempt == maxAttempts {
				return "", lastErr
			}
			if err := sleepCtx(ctx, backoff); err != nil {
				return "", err
			}
			backoff *= 2
			continue
		}

		body, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if rerr != nil {
			lastErr = fmt.Errorf("read claude response (attempt %d/%d): %w", attempt, maxAttempts, rerr)
			if attempt == maxAttempts {
				return "", lastErr
			}
			if err := sleepCtx(ctx, backoff); err != nil {
				return "", err
			}
			backoff *= 2
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("claude upstream %d (attempt %d/%d): %s",
				resp.StatusCode, attempt, maxAttempts, summarizeUpstreamBody(resp, body))
			if attempt == maxAttempts {
				return "", lastErr
			}
			if err := sleepCtx(ctx, backoff); err != nil {
				return "", err
			}
			backoff *= 2
			continue
		}

		if !isJSONResponse(resp, body) {
			return "", fmt.Errorf("claude returned non-JSON response (status %d, content-type %q): %s",
				resp.StatusCode, resp.Header.Get("Content-Type"), summarizeUpstreamBody(resp, body))
		}

		var ar anthropicResponse
		if err := json.Unmarshal(body, &ar); err != nil {
			return "", fmt.Errorf("unmarshal claude response (status %d): %w — body: %s", resp.StatusCode, err, body)
		}
		if ar.Error != nil {
			return "", fmt.Errorf("claude API error: %s", ar.Error.Message)
		}
		if len(ar.Content) == 0 {
			return "", fmt.Errorf("claude returned empty content")
		}
		return ar.Content[0].Text, nil
	}
	return "", lastErr
}

// extractJSON strips prose around the first balanced JSON object/array.
func extractJSON(s string) string {
	for _, markers := range []struct{ open, close byte }{{'{', '}'}, {'[', ']'}} {
		start := strings.IndexByte(s, markers.open)
		end := strings.LastIndexByte(s, markers.close)
		if start != -1 && end != -1 && end > start {
			return s[start : end+1]
		}
	}
	return s
}

func isJSONResponse(resp *http.Response, body []byte) bool {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "json") {
		return true
	}
	trimmed := bytes.TrimLeft(body, " \t\r\n")
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

func summarizeUpstreamBody(resp *http.Response, body []byte) string {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	lower := strings.ToLower(string(body))
	if strings.Contains(ct, "html") || strings.Contains(lower, "<html") {
		if strings.Contains(lower, "cloudflare") {
			return fmt.Sprintf("cloudflare HTML error page (status %d) — likely Anthropic upstream outage", resp.StatusCode)
		}
		return fmt.Sprintf("HTML error page (status %d)", resp.StatusCode)
	}
	const maxLen = 300
	if len(body) > maxLen {
		return string(body[:maxLen]) + "…"
	}
	return string(body)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// isTransientHTTP returns true if the given error is a retriable
// transport-layer error (timeout, connection reset, DNS).
func isTransientHTTP(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "dial tcp") ||
		strings.Contains(msg, "no such host")
}
```

- [ ] **Step 3: Delete duplicated definitions from pass1.go**

Remove from `backend/internal/ai/pass1.go`:
- Constants `claudeModel`, `anthropicAPI`
- All types: `anthropicRequest`, `anthropicMessage`, `anthropicContent`, `contentSource`, `anthropicResponse`, `FileInput`
- All functions: `callClaude`, `extractJSON`, `isJSONResponse`, `summarizeUpstreamBody`, `sleepCtx`, `isTransientHTTP`

In the remaining `ParseDocuments` body of `pass1.go`, change `claudeModel` → `ModelOpusParser` and `anthropicAPI` → `AnthropicAPI`.

- [ ] **Step 4: Verify build and existing tests**

Run:
```bash
cd backend && go build ./... && go test ./...
```
Expected: builds cleanly; all existing tests pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ai/client.go backend/internal/ai/pass1.go
git commit -m "refactor(ai): extract Anthropic HTTP helpers into client.go"
```

---

### Task 11: Add `isTransientHTTP` guard file if missing

**Files:**
- Check: `backend/internal/ai/pass1.go` — verify `isTransientHTTP` was removed in previous task.

- [ ] **Step 1: grep for duplicate definitions**

Run: `grep -nE 'func isTransientHTTP|const claudeModel|const anthropicAPI' backend/internal/ai/*.go`
Expected: each appears exactly once (in `client.go`).

- [ ] **Step 2: If duplicates exist, remove them from pass1.go**

Open `backend/internal/ai/pass1.go` and remove any remaining duplicates of constants or helpers now living in `client.go`.

- [ ] **Step 3: Verify build**

Run: `cd backend && go build ./...`
Expected: no errors.

- [ ] **Step 4: Commit if changed**

```bash
git add backend/internal/ai/pass1.go
git commit -m "refactor(ai): remove remaining duplicates from pass1.go" || true
```

---

## Phase D — New AI Pipeline

### Task 12: `translate.go` — mini translator

**Files:**
- Create: `backend/internal/ai/translate.go`
- Create: `backend/internal/ai/translate_test.go`

- [ ] **Step 1: Write failing test (uses stub HTTP server)**

Create `backend/internal/ai/translate_test.go`:

```go
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
```

- [ ] **Step 2: Add `AnthropicAPIOverride` hook in `client.go`**

Edit `backend/internal/ai/client.go`. Above the `callClaude` function, add:

```go
// AnthropicAPIOverride, if non-empty, is used instead of AnthropicAPI.
// Test-only hook (do not set in production).
var AnthropicAPIOverride string

func anthropicURL() string {
	if AnthropicAPIOverride != "" {
		return AnthropicAPIOverride
	}
	return AnthropicAPI
}
```

Then inside `callClaude`, replace `AnthropicAPI` (in the `http.NewRequestWithContext` call) with `anthropicURL()`.

- [ ] **Step 3: Implement `translate.go`**

Create `backend/internal/ai/translate.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

const translateSystemPrompt = `You are a Russian → English translator for Japanese visa documents. Translate each string to natural English. For proper names (companies, people, street names), transliterate using the standard Russian → Latin system. For descriptive words (job titles, address parts), translate them fully. Return ONLY a JSON array of translations, same length and order as the input array. No markdown fences, no prose.

Examples:
- "Директор по развитию" → "Director of Development"
- "ООО Ромашка" → "LLC Romashka"
- "Москва, ул. Ленина 5, кв. 12" → "Moscow, Lenin St. 5, Apt. 12"
- "ИП Иванов Петр" → "IE Ivanov Petr"
- "ОУФМС России по г. Москве" → "Federal Migration Service in Moscow"
- "МВД 77810" → "MVD 77810"
- "СССР" → "USSR"
- "январь 2020" → "January 2020"`

// TranslateStrings sends a batch of Russian strings to Claude Haiku for
// English translation. Nil or empty input → nil output, no API call.
// The result slice is exactly the same length as the input.
func TranslateStrings(ctx context.Context, apiKey string, src []string) ([]string, error) {
	if len(src) == 0 {
		return nil, nil
	}
	userBody, err := json.Marshal(map[string]any{"strings": src})
	if err != nil {
		return nil, fmt.Errorf("marshal translate input: %w", err)
	}

	req := anthropicRequest{
		Model:       ModelHaikuTranslate,
		MaxTokens:   2048,
		Temperature: 0,
		System:      translateSystemPrompt,
		Messages: []anthropicMessage{{
			Role: "user",
			Content: []anthropicContent{
				{Type: "text", Text: string(userBody)},
			},
		}},
	}

	raw, err := callClaude(ctx, apiKey, req)
	if err != nil {
		return nil, fmt.Errorf("translate claude call: %w", err)
	}

	js := extractJSON(raw)
	var out []string
	if err := json.Unmarshal([]byte(js), &out); err != nil {
		return nil, fmt.Errorf("translate decode array: %w — raw: %s", err, raw)
	}
	if len(out) != len(src) {
		return nil, fmt.Errorf("translate length mismatch: got %d, want %d — raw: %s", len(out), len(src), raw)
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/ai/... -run TestTranslateStrings -v`
Expected: both pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ai/translate.go backend/internal/ai/translate_test.go backend/internal/ai/client.go
git commit -m "feat(ai): translate.go — batched mini translator (Haiku)"
```

---

### Task 13: `programme.go` — main programme generator

**Files:**
- Create: `backend/internal/ai/programme.go`
- Create: `backend/internal/ai/programme_test.go`

- [ ] **Step 1: Write failing test**

Create `backend/internal/ai/programme_test.go`:

```go
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
		Hotels: []HotelBrief{{Name: "H1", City: "TOKYO", CheckIn: "25.04.2026", CheckOut: "27.04.2026"}},
		ContactPhone: "+7",
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
```

- [ ] **Step 2: Implement**

Create `backend/internal/ai/programme.go`:

```go
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

	userMsg := fmt.Sprintf("Trip data:\n\n```json\n%s\n```\n\nProduce the programme array.", string(body))

	req := anthropicRequest{
		Model:       ModelOpusProgramme,
		MaxTokens:   4096,
		Temperature: 0.3,
		System:      programmeSystemPrompt,
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
```

- [ ] **Step 3: Run tests**

Run: `cd backend && go test ./internal/ai/... -run TestGenerateProgramme -v`
Expected: pass.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/ai/programme.go backend/internal/ai/programme_test.go
git commit -m "feat(ai): programme.go — itinerary generator (Opus 4.7, no PII)"
```

---

### Task 14: `ticket_parser.go` — scan → flight_data

**Files:**
- Create: `backend/internal/ai/ticket_parser.go`

- [ ] **Step 1: Implement**

Create `backend/internal/ai/ticket_parser.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"encoding/base64"
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

const ticketSystemPrompt = `You are a flight-ticket parser for a Japanese visa agency. Given one or more ticket scans, extract the arrival leg INTO Japan and the return leg FROM Japan. Return ONLY valid JSON matching the schema below — no prose, no markdown.

OUTPUT SCHEMA:
{
  "arrival":   { "flight_number": "...", "date": "DD.MM.YYYY", "time": "HH:MM", "airport": "CITY AIRPORT" },
  "departure": { "flight_number": "...", "date": "DD.MM.YYYY", "time": "HH:MM", "airport": "CITY AIRPORT" }
}

RULES:
- arrival: the LAST leg that lands in Japan (for multi-leg itineraries, the final leg; date is Japan local).
- departure: the FIRST leg leaving Japan (takeoff from Japan; date is Japan local).
- If the ticket is strictly ONE-WAY (no return), leave all departure.* fields "".
- Airport format: "CITY AIRPORTNAME" in CAPS (e.g. "TOKYO NARITA", "OSAKA KANSAI").
- Flight number: include space, e.g. "SU 262", "CZ 8101".
- All dates DD.MM.YYYY.
- Never invent data. Missing → "".`

// ParseTicket sends the given ticket files (PDF/JPG/PNG) to Claude and
// returns the extracted flight data.
func ParseTicket(ctx context.Context, apiKey string, files []FileInput) (TicketFlights, error) {
	contents, err := buildFileContents(files)
	if err != nil {
		return TicketFlights{}, err
	}
	contents = append(contents, anthropicContent{
		Type: "text",
		Text: "Extract the flight data from the scan(s) above per the schema.",
	})

	req := anthropicRequest{
		Model:       ModelOpusParser,
		MaxTokens:   1024,
		Temperature: 0,
		System:      ticketSystemPrompt,
		Messages:    []anthropicMessage{{Role: "user", Content: contents}},
	}
	raw, err := callClaude(ctx, apiKey, req)
	if err != nil {
		return TicketFlights{}, fmt.Errorf("ticket parse call: %w", err)
	}
	var out TicketFlights
	if err := json.Unmarshal([]byte(extractJSON(raw)), &out); err != nil {
		return TicketFlights{}, fmt.Errorf("ticket parse decode: %w — raw: %s", err, raw)
	}
	return out, nil
}

// buildFileContents converts FileInput slices into Anthropic content blocks.
// Images → "image", PDFs/others → "document".
func buildFileContents(files []FileInput) ([]anthropicContent, error) {
	var contents []anthropicContent
	for _, inp := range files {
		if inp.AnthropicFileID != "" {
			ext := strings.ToLower(filepath.Ext(inp.Name))
			blockType := "document"
			if ext == ".jpg" || ext == ".jpeg" || ext == ".png" {
				blockType = "image"
			}
			contents = append(contents, anthropicContent{
				Type: blockType,
				Source: &contentSource{Type: "file", FileID: inp.AnthropicFileID},
			})
			continue
		}
		ext := strings.ToLower(filepath.Ext(inp.Name))
		switch ext {
		case ".jpg", ".jpeg":
			contents = append(contents, anthropicContent{
				Type: "image",
				Source: &contentSource{
					Type: "base64", MediaType: "image/jpeg",
					Data: base64.StdEncoding.EncodeToString(inp.Data),
				},
			})
		case ".png":
			contents = append(contents, anthropicContent{
				Type: "image",
				Source: &contentSource{
					Type: "base64", MediaType: "image/png",
					Data: base64.StdEncoding.EncodeToString(inp.Data),
				},
			})
		case ".pdf":
			contents = append(contents, anthropicContent{
				Type: "document",
				Source: &contentSource{
					Type: "base64", MediaType: "application/pdf",
					Data: base64.StdEncoding.EncodeToString(inp.Data),
				},
			})
		default:
			contents = append(contents, anthropicContent{
				Type: "text",
				Text: fmt.Sprintf("File: %s\n\n%s", inp.Name, string(inp.Data)),
			})
		}
	}
	return contents, nil
}
```

- [ ] **Step 2: Verify build**

Run: `cd backend && go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/ai/ticket_parser.go
git commit -m "feat(ai): ticket_parser.go — scan → flight_data"
```

---

### Task 15: `voucher_parser.go` — scan → hotel list

**Files:**
- Create: `backend/internal/ai/voucher_parser.go`

- [ ] **Step 1: Implement**

Create `backend/internal/ai/voucher_parser.go`:

```go
package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// VoucherHotel is one hotel extracted from a voucher.
type VoucherHotel struct {
	Name     string `json:"name"`
	City     string `json:"city"`
	Address  string `json:"address"`
	Phone    string `json:"phone"`
	CheckIn  string `json:"check_in"`  // DD.MM.YYYY
	CheckOut string `json:"check_out"` // DD.MM.YYYY
}

const voucherSystemPrompt = `You are a hotel voucher parser for a Japanese visa agency. Given voucher scans, extract every hotel stay found. Return ONLY a JSON array — no markdown, no prose.

SCHEMA:
[ { "name": "...", "city": "CITY CAPS", "address": "...", "phone": "+81 ...", "check_in": "DD.MM.YYYY", "check_out": "DD.MM.YYYY" } ]

RULES:
- name: official hotel name in English (as on the voucher).
- city: Japanese city, English CAPS (e.g. "TOKYO", "KYOTO", "OSAKA").
- address: full street address in English. First check the voucher; if the voucher has no address, use your general knowledge of well-known Japanese hotels (e.g. "Dusit Thani Kyoto" → official address). If you genuinely do not know, use "".
- phone: international format (+81 ...). Same fallback as address.
- Dates DD.MM.YYYY.
- Do not invent names or dates. Only addresses/phones may be filled from general knowledge.`

// ParseVouchers returns the hotels found across all voucher files.
func ParseVouchers(ctx context.Context, apiKey string, files []FileInput) ([]VoucherHotel, error) {
	contents, err := buildFileContents(files)
	if err != nil {
		return nil, err
	}
	contents = append(contents, anthropicContent{
		Type: "text",
		Text: "Extract every hotel stay from the voucher(s) above per the schema.",
	})

	req := anthropicRequest{
		Model:       ModelOpusParser,
		MaxTokens:   2048,
		Temperature: 0,
		System:      voucherSystemPrompt,
		Messages:    []anthropicMessage{{Role: "user", Content: contents}},
	}
	raw, err := callClaude(ctx, apiKey, req)
	if err != nil {
		return nil, fmt.Errorf("voucher parse call: %w", err)
	}
	var out []VoucherHotel
	if err := json.Unmarshal([]byte(extractJSON(raw)), &out); err != nil {
		return nil, fmt.Errorf("voucher parse decode: %w — raw: %s", err, raw)
	}
	return out, nil
}
```

- [ ] **Step 2: Verify build**

Run: `cd backend && go build ./...`

- [ ] **Step 3: Commit**

```bash
git add backend/internal/ai/voucher_parser.go
git commit -m "feat(ai): voucher_parser.go — scan → hotel list"
```

---

### Task 16: `assembler.go` — build final pass2.json deterministically

**Files:**
- Create: `backend/internal/ai/assembler.go`
- Create: `backend/internal/ai/assembler_test.go`
- Create: `backend/internal/ai/pass2_schema.go` (structs describing pass2.json)

- [ ] **Step 1: Define pass2 schema**

Create `backend/internal/ai/pass2_schema.go`:

```go
package ai

// Pass2Tourist is one tourist row in the final pass2.json.
// JSON field names and shape must exactly match what docgen/generate.py
// expects (do not rename without updating the Python side).
type Pass2Tourist struct {
	NameLat           string `json:"name_lat"`
	NameCyr           string `json:"name_cyr"`
	PassportNumber    string `json:"passport_number"`
	BirthDate         string `json:"birth_date"`
	Nationality       string `json:"nationality"`
	PlaceOfBirth      string `json:"place_of_birth"`
	IssueDate         string `json:"issue_date"`
	ExpiryDate        string `json:"expiry_date"`
	FormerNationality string `json:"former_nationality"`
	Gender            string `json:"gender"`
	MaritalStatus     string `json:"marital_status"`
	PassportType      string `json:"passport_type"`
	IssuedBy          string `json:"issued_by"`
	HomeAddress       string `json:"home_address"`
	Phone             string `json:"phone"`
	Occupation        string `json:"occupation"`
	Employer          string `json:"employer"`
	EmployerAddress   string `json:"employer_address"`
	BeenToJapan       string `json:"been_to_japan"`
	PreviousVisits    string `json:"previous_visits"`
	CriminalRecord    string `json:"criminal_record"`
	MaidenName        string `json:"maiden_name"`
	NationalityISO    string `json:"nationality_iso"`
	FormerNationalityText string `json:"former_nationality_text"`
	GenderRB          string `json:"gender_rb"`
	MaritalStatusRB   string `json:"marital_status_rb"`
	PassportTypeRB    string `json:"passport_type_rb"`
	ArrivalDateJapan  string `json:"arrival_date_japan"`
	ArrivalTime       string `json:"arrival_time"`
	ArrivalAirport    string `json:"arrival_airport"`
	ArrivalFlight     string `json:"arrival_flight"`
	DepartureDateJapan string `json:"departure_date_japan"`
	DepartureTime     string `json:"departure_time"`
	DepartureAirport  string `json:"departure_airport"`
	DepartureFlight   string `json:"departure_flight"`
	IntendedStayDays  int    `json:"intended_stay_days"`
}

// Pass2 is the root document passed to docgen/generate.py.
type Pass2 struct {
	DocumentDate      string           `json:"document_date"`
	Tourists          []Pass2Tourist   `json:"tourists"`
	Programme         []ProgrammeDay   `json:"programme"`
	Anketa            Pass2Anketa      `json:"anketa"`
	Doverenost        []Pass2Dov       `json:"doverenost"`
	VCRequest         Pass2VC          `json:"vc_request"`
	InnaDoc           Pass2Inna        `json:"inna_doc"`
	FirstHotel        Pass2Hotel       `json:"first_hotel"`
	Arrival           Pass2ArrDep      `json:"arrival"`
	Departure         Pass2ArrDep      `json:"departure"`
	IntendedStayDays  int              `json:"intended_stay_days"`
	Email             Pass2Email       `json:"email"`
}

type Pass2Anketa struct {
	CriminalRB        string `json:"criminal_rb"`
	Email             string `json:"email"`
	DateOfApplication string `json:"date_of_application"`
	FirstHotelName    string `json:"first_hotel_name"`
	FirstHotelAddress string `json:"first_hotel_address"`
	FirstHotelPhone   string `json:"first_hotel_phone"`
}

type Pass2Dov struct {
	NameRu         string `json:"name_ru"`
	DOB            string `json:"dob"`
	PassportSeries string `json:"passport_series"`
	PassportNumber string `json:"passport_number"`
	IssuedDate     string `json:"issued_date"`
	IssuedBy       string `json:"issued_by"`
	RegAddress     string `json:"reg_address"`
	IsMinor        bool   `json:"is_minor"`
	ChildNameRu    string `json:"child_name_ru"`
	ChildGender    string `json:"child_gender"`
}

type Pass2VC struct {
	Applicants          []string `json:"applicants"`
	Count               int      `json:"count"`
	ServiceFeePerPerson int      `json:"service_fee_per_person"`
	ServiceFeeTotal     int      `json:"service_fee_total"`
}

type Pass2Inna struct {
	SubmissionDate string   `json:"submission_date"`
	ApplicantsRu   []string `json:"applicants_ru"`
}

type Pass2Hotel struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Phone   string `json:"phone"`
}

type Pass2ArrDep struct {
	Date    string `json:"date"`
	Airport string `json:"airport"`
	Flight  string `json:"flight"`
	Time    string `json:"time"`
}

type Pass2Email struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}
```

- [ ] **Step 2: Write tests for the assembler (small, focused slices)**

Create `backend/internal/ai/assembler_test.go`:

```go
package ai

import (
	"testing"
)

func TestAssembleTourist_MaidenNameTransliterated(t *testing.T) {
	payload := map[string]any{
		"name_cyr":           "Иванова Анна",
		"name_lat":           "IVANOVA ANNA",
		"maiden_name_ru":     "Петрова",
		"gender_ru":          "Женский",
		"marital_status_ru":  "Замужем",
		"passport_type_ru":   "Обычный",
		"criminal_record_ru": "Нет",
		"been_to_japan_ru":   "Нет",
		"birth_date":         "15.03.1980",
		"nationality_ru":     "Россия",
		"former_nationality_ru": "",
		"place_of_birth_ru":  "Москва",
	}
	translations := map[string]string{
		"Москва": "Moscow",
	}
	flight := map[string]any{
		"arrival":   map[string]any{"flight_number": "SU 262", "date": "25.04.2026", "time": "12:45", "airport": "TOKYO NARITA"},
		"departure": map[string]any{"flight_number": "SU 263", "date": "05.05.2026", "time": "14:20", "airport": "TOKYO NARITA"},
	}

	got := AssembleTourist(payload, translations, flight)

	if got.MaidenName != "PETROVA" {
		t.Errorf("maiden_name = %q, want PETROVA", got.MaidenName)
	}
	if got.Gender != "Female" {
		t.Errorf("gender = %q", got.Gender)
	}
	if got.GenderRB != "1" {
		t.Errorf("gender_rb = %q", got.GenderRB)
	}
	if got.MaritalStatus != "Married" {
		t.Errorf("marital_status = %q", got.MaritalStatus)
	}
	if got.PlaceOfBirth != "Moscow" {
		t.Errorf("place_of_birth = %q", got.PlaceOfBirth)
	}
	if got.NationalityISO != "RUS" {
		t.Errorf("nationality_iso = %q", got.NationalityISO)
	}
	if got.IntendedStayDays != 11 {
		t.Errorf("intended_stay_days = %d, want 11", got.IntendedStayDays)
	}
	if got.ArrivalFlight != "SU 262" {
		t.Errorf("arrival_flight = %q", got.ArrivalFlight)
	}
}

func TestAssembleTourist_OneWay(t *testing.T) {
	payload := map[string]any{
		"name_cyr":          "Сидоров Петр",
		"gender_ru":         "Мужской",
		"passport_type_ru":  "Обычный",
		"birth_date":        "10.06.1990",
	}
	flight := map[string]any{
		"arrival":   map[string]any{"flight_number": "SU 262", "date": "25.04.2026", "time": "12:45", "airport": "TOKYO NARITA"},
		"departure": map[string]any{}, // empty — one-way
	}

	got := AssembleTourist(payload, nil, flight)
	if got.IntendedStayDays != 0 {
		t.Errorf("one-way intended_stay_days = %d, want 0", got.IntendedStayDays)
	}
	if got.DepartureFlight != "" {
		t.Errorf("one-way departure_flight = %q, want empty", got.DepartureFlight)
	}
}
```

- [ ] **Step 3: Implement**

Create `backend/internal/ai/assembler.go`:

```go
package ai

import (
	"fmt"
	"strings"
	"time"

	"fujitravel-admin/backend/internal/translit"
)

// AssembleTourist builds one Pass2Tourist from the submission payload,
// translations map, and flight_data.
// Arguments use untyped maps because they come from JSONB columns — the
// caller (orchestrator) already has them as map[string]any.
func AssembleTourist(payload map[string]any, translations map[string]string, flight map[string]any) Pass2Tourist {
	get := func(k string) string {
		if v, ok := payload[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	tr := func(k string) string {
		src := get(k)
		if src == "" {
			return ""
		}
		if translations != nil {
			if t, ok := translations[src]; ok && t != "" {
				return t
			}
		}
		return src // fallback to raw if no translation
	}

	gender := MapGender(get("gender_ru"))
	marital := MapMaritalStatus(get("marital_status_ru"))
	passportType := MapPassportType(get("passport_type_ru"))

	arrival := subMap(flight, "arrival")
	departure := subMap(flight, "departure")

	stayDays := ComputeIntendedStayDays(strGet(arrival, "date"), strGet(departure, "date"))

	return Pass2Tourist{
		NameLat:           firstNonEmpty(get("name_lat"), translit.RuToLatICAO(get("name_cyr"))),
		NameCyr:           get("name_cyr"),
		PassportNumber:    get("passport_number"),
		BirthDate:         get("birth_date"),
		Nationality:       strings.ToUpper(tr("nationality_ru")),
		PlaceOfBirth:      tr("place_of_birth_ru"),
		IssueDate:         get("issue_date"),
		ExpiryDate:        get("expiry_date"),
		FormerNationality: ComputeFormerNationality(get("former_nationality_ru"), get("place_of_birth_ru"), get("birth_date")),
		Gender:            gender,
		MaritalStatus:     marital,
		PassportType:      passportType,
		IssuedBy:          tr("issued_by_ru"),
		HomeAddress:       tr("home_address_ru"),
		Phone:             get("phone"),
		Occupation:        tr("occupation_ru"),
		Employer:          tr("employer_ru"),
		EmployerAddress:   tr("employer_address_ru"),
		BeenToJapan:       MapYesNo(get("been_to_japan_ru")),
		PreviousVisits:    tr("previous_visits_ru"),
		CriminalRecord:    MapYesNo(get("criminal_record_ru")),
		MaidenName:        translit.RuToLatICAO(get("maiden_name_ru")),
		NationalityISO:    CountryISO(get("nationality_ru")),
		FormerNationalityText: ComputeFormerNationality(get("former_nationality_ru"), get("place_of_birth_ru"), get("birth_date")),
		GenderRB:          GenderRB(gender),
		MaritalStatusRB:   MaritalRB(marital),
		PassportTypeRB:    PassportTypeRB(passportType),
		ArrivalDateJapan:  strGet(arrival, "date"),
		ArrivalTime:       strGet(arrival, "time"),
		ArrivalAirport:    strGet(arrival, "airport"),
		ArrivalFlight:     strGet(arrival, "flight_number"),
		DepartureDateJapan: strGet(departure, "date"),
		DepartureTime:     strGet(departure, "time"),
		DepartureAirport:  strGet(departure, "airport"),
		DepartureFlight:   strGet(departure, "flight_number"),
		IntendedStayDays:  stayDays,
	}
}

// AssembleDoverenost builds the doverenost entries. `group` is the list
// of tourists already assembled (needed for parent matching).
func AssembleDoverenost(tourists []Pass2Tourist, payloads []map[string]any, departureDate string) []Pass2Dov {
	refs := make([]TouristRef, len(tourists))
	for i, t := range tourists {
		refs[i] = TouristRef{
			ID:         fmt.Sprint(i),
			SurnameCyr: FirstWord(t.NameCyr),
			BirthDate:  t.BirthDate,
			IsMinor:    IsMinorOnDate(t.BirthDate, departureDate),
		}
	}

	out := make([]Pass2Dov, len(tourists))
	for i, t := range tourists {
		minor := refs[i].IsMinor
		payload := payloads[i]
		dov := Pass2Dov{
			NameRu:         t.NameCyr,
			DOB:            t.BirthDate,
			PassportSeries: strGet(payload, "internal_series"),
			PassportNumber: strGet(payload, "internal_number"),
			IssuedDate:     russianIssuedDate(strGet(payload, "internal_issued_ru")),
			IssuedBy:       strGet(payload, "internal_issued_by_ru"),
			RegAddress:     strGet(payload, "reg_address_ru"),
			IsMinor:        minor,
		}
		if minor {
			parent := FindParent(refs[i], refs)
			if parent != nil {
				pp := payloads[indexOf(refs, parent.ID)]
				dov.NameRu = tourists[indexOf(refs, parent.ID)].NameCyr
				dov.DOB = tourists[indexOf(refs, parent.ID)].BirthDate
				dov.PassportSeries = strGet(pp, "internal_series")
				dov.PassportNumber = strGet(pp, "internal_number")
				dov.IssuedDate = russianIssuedDate(strGet(pp, "internal_issued_ru"))
				dov.IssuedBy = strGet(pp, "internal_issued_by_ru")
				dov.RegAddress = strGet(pp, "reg_address_ru")
			}
			// Child name in genitive case
			surname := FirstWord(t.NameCyr)
			firstName := strings.TrimSpace(strings.TrimPrefix(t.NameCyr, surname))
			childName, _ := GenitiveFullName(surname, firstName, t.Gender)
			dov.ChildNameRu = childName
			if t.Gender == "Male" {
				dov.ChildGender = "сына"
			} else {
				dov.ChildGender = "дочери"
			}
		}
		out[i] = dov
	}
	return out
}

// AssemblePass2 is the top-level entry point called by generate.go.
// It composes the full pass2.json structure from already-fetched inputs.
// todayDate: DD.MM.YYYY for the date_of_application.
func AssemblePass2(
	payloads []map[string]any,
	translations []map[string]string,
	flights []map[string]any,
	programme []ProgrammeDay,
	hotels []HotelBrief,
	todayDate string,
) Pass2 {
	tourists := make([]Pass2Tourist, len(payloads))
	for i := range payloads {
		tourists[i] = AssembleTourist(payloads[i], translations[i], flights[i])
	}

	var firstHotel Pass2Hotel
	if len(hotels) > 0 {
		firstHotel = Pass2Hotel{Name: hotels[0].Name, Address: hotels[0].Address, Phone: hotels[0].Phone}
	}

	// Arrival/departure block uses first tourist with populated flight data
	var arr, dep Pass2ArrDep
	for _, t := range tourists {
		if t.ArrivalFlight != "" {
			arr = Pass2ArrDep{Date: t.ArrivalDateJapan, Airport: t.ArrivalAirport, Flight: t.ArrivalFlight, Time: t.ArrivalTime}
			break
		}
	}
	for _, t := range tourists {
		if t.DepartureFlight != "" {
			dep = Pass2ArrDep{Date: t.DepartureDateJapan, Airport: t.DepartureAirport, Flight: t.DepartureFlight, Time: t.DepartureTime}
			break
		}
	}

	doverenost := AssembleDoverenost(tourists, payloads, dep.Date)

	var cyrNames []string
	for _, t := range tourists {
		cyrNames = append(cyrNames, t.NameCyr)
	}

	return Pass2{
		DocumentDate: todayDate,
		Tourists:     tourists,
		Programme:    programme,
		Anketa: Pass2Anketa{
			CriminalRB:        "1",
			Email:             "tour@fujitravel.ru",
			DateOfApplication: todayDate,
			FirstHotelName:    firstHotel.Name,
			FirstHotelAddress: firstHotel.Address,
			FirstHotelPhone:   firstHotel.Phone,
		},
		Doverenost: doverenost,
		VCRequest: Pass2VC{
			Applicants:          cyrNames,
			Count:               len(cyrNames),
			ServiceFeePerPerson: 970,
			ServiceFeeTotal:     970 * len(cyrNames),
		},
		InnaDoc: Pass2Inna{
			SubmissionDate: arr.Date,
			ApplicantsRu:   cyrNames,
		},
		FirstHotel:       firstHotel,
		Arrival:          arr,
		Departure:        dep,
		IntendedStayDays: 0, // per-tourist field is authoritative
		Email: Pass2Email{
			To:      "ta_japan_moscow@vfsglobal.com",
			Subject: todayDate + " FujiTravel",
			Body:    strings.Join(cyrNames, "\n"),
		},
	}
}

// ---------- helpers ----------

func subMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key]; ok {
		if sub, ok := v.(map[string]any); ok {
			return sub
		}
	}
	return nil
}

func strGet(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

var russianMonths = []string{"", "января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}

// russianIssuedDate formats DD.MM.YYYY → «DD» Month YYYY
func russianIssuedDate(s string) string {
	t, err := time.Parse("02.01.2006", strings.TrimSpace(s))
	if err != nil {
		return s
	}
	return fmt.Sprintf("«%02d» %s %d", t.Day(), russianMonths[int(t.Month())], t.Year())
}

func indexOf(refs []TouristRef, id string) int {
	for i, r := range refs {
		if r.ID == id {
			return i
		}
	}
	return 0
}
```

- [ ] **Step 4: Run tests**

Run: `cd backend && go test ./internal/ai/... -v`
Expected: assembler tests pass, all prior tests still pass.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ai/assembler.go backend/internal/ai/assembler_test.go backend/internal/ai/pass2_schema.go
git commit -m "feat(ai): assembler.go — deterministic pass2.json composition"
```

---

## Phase E — New API Endpoints

### Task 17: POST /api/submissions (public form submit)

**Files:**
- Create: `backend/internal/api/submissions.go`

- [ ] **Step 1: Scaffold the file and implement POST**

Create `backend/internal/api/submissions.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/consent"
)

// TouristSubmission mirrors the DB row.
type TouristSubmission struct {
	ID                string          `json:"id"`
	Payload           json.RawMessage `json:"payload"`
	ConsentAccepted   bool            `json:"consent_accepted"`
	ConsentAcceptedAt time.Time       `json:"consent_accepted_at"`
	ConsentVersion    string          `json:"consent_version"`
	Source            string          `json:"source"`
	Status            string          `json:"status"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// Required top-level keys in payload for a submission to be considered valid.
var requiredPayloadKeys = []string{
	"name_lat", "name_cyr", "gender_ru", "birth_date",
	"passport_number", "issue_date", "expiry_date",
	"internal_series", "internal_number",
	"phone", "home_address_ru",
}

// CreateSubmission handles POST /api/submissions (public, no auth).
func CreateSubmission(db *pgxpool.Pool) http.HandlerFunc {
	type reqBody struct {
		Payload         map[string]any `json:"payload"`
		ConsentAccepted bool           `json:"consent_accepted"`
		Source          string         `json:"source"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body reqBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !body.ConsentAccepted {
			writeError(w, http.StatusBadRequest, "consent not accepted")
			return
		}
		if body.Source != "tourist" && body.Source != "manager" {
			body.Source = "tourist"
		}

		var missing []string
		for _, k := range requiredPayloadKeys {
			v, ok := body.Payload[k].(string)
			if !ok || v == "" {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			writeErrorWithDetails(w, http.StatusBadRequest, "missing fields", map[string]any{
				"missing": missing,
			})
			return
		}

		payloadBytes, _ := json.Marshal(body.Payload)
		agreement := consent.Current()

		var id string
		err := db.QueryRow(r.Context(),
			`INSERT INTO tourist_submissions
			   (payload, consent_accepted, consent_accepted_at, consent_version, source)
			 VALUES ($1, TRUE, NOW(), $2, $3)
			 RETURNING id`,
			payloadBytes, agreement.Version, body.Source).Scan(&id)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeError(w, http.StatusConflict, "duplicate submission (same passport, same day)")
				return
			}
			slog.Error("create submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": id})
	}
}

// writeErrorWithDetails is a helper that wraps writeError with extra fields.
func writeErrorWithDetails(w http.ResponseWriter, status int, msg string, details map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload := map[string]any{"error": msg}
	for k, v := range details {
		payload[k] = v
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// ensurePgxNoRowsIsNil unwraps pgx.ErrNoRows into a nil-safe check.
func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
```

- [ ] **Step 2: Commit** (route wiring happens later in Task 29)

```bash
git add backend/internal/api/submissions.go
git commit -m "feat(api): POST /api/submissions — create form submission"
```

---

### Task 18: GET /api/submissions — list with search

**Files:**
- Modify: `backend/internal/api/submissions.go`

- [ ] **Step 1: Append handler**

Append to `backend/internal/api/submissions.go`:

```go
// ListSubmissions handles GET /api/submissions?q=&status=
func ListSubmissions(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		status := r.URL.Query().Get("status")

		args := []any{}
		where := []string{}
		if q != "" {
			args = append(args, "%"+q+"%")
			where = append(where, "payload ->> 'name_lat' ILIKE $"+itoa(len(args)))
		}
		if status != "" {
			args = append(args, status)
			where = append(where, "status = $"+itoa(len(args)))
		}

		sql := `SELECT id, payload, consent_accepted, consent_accepted_at, consent_version,
			           source, status, created_at, updated_at
			      FROM tourist_submissions`
		if len(where) > 0 {
			sql += " WHERE " + joinAnd(where)
		}
		sql += " ORDER BY created_at DESC LIMIT 500"

		rows, err := db.Query(r.Context(), sql, args...)
		if err != nil {
			slog.Error("list submissions", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		defer rows.Close()

		out := []TouristSubmission{}
		for rows.Next() {
			var s TouristSubmission
			var payload []byte
			if err := rows.Scan(&s.ID, &payload, &s.ConsentAccepted, &s.ConsentAcceptedAt,
				&s.ConsentVersion, &s.Source, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
				slog.Error("scan submission", "err", err)
				continue
			}
			s.Payload = json.RawMessage(payload)
			out = append(out, s)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// itoa / joinAnd are small helpers local to this file (avoid pulling strconv/strings imports twice)
func itoa(n int) string {
	// only used for small positives — placeholder numbers up to ~20
	digits := "0123456789"
	if n < 10 {
		return string(digits[n])
	}
	return string(digits[n/10]) + string(digits[n%10])
}

func joinAnd(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += " AND " + p
	}
	return out
}
```

- [ ] **Step 2: Verify build**

Run: `cd backend && go build ./...`

- [ ] **Step 3: Commit**

```bash
git add backend/internal/api/submissions.go
git commit -m "feat(api): GET /api/submissions — list with search"
```

---

### Task 19: GET /api/submissions/:id — single detail

**Files:**
- Modify: `backend/internal/api/submissions.go`

- [ ] **Step 1: Append handler**

Append to `backend/internal/api/submissions.go`:

```go
// GetSubmission handles GET /api/submissions/:id
func GetSubmission(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var s TouristSubmission
		var payload []byte
		err := db.QueryRow(r.Context(),
			`SELECT id, payload, consent_accepted, consent_accepted_at, consent_version,
			        source, status, created_at, updated_at
			   FROM tourist_submissions WHERE id = $1`, id).
			Scan(&s.ID, &payload, &s.ConsentAccepted, &s.ConsentAcceptedAt,
				&s.ConsentVersion, &s.Source, &s.Status, &s.CreatedAt, &s.UpdatedAt)
		if isNoRows(err) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		if err != nil {
			slog.Error("get submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		s.Payload = json.RawMessage(payload)
		writeJSON(w, http.StatusOK, s)
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/api/submissions.go
git commit -m "feat(api): GET /api/submissions/:id"
```

---

### Task 20: PUT /api/submissions/:id — edit

**Files:**
- Modify: `backend/internal/api/submissions.go`

- [ ] **Step 1: Append handler**

Append to `backend/internal/api/submissions.go`:

```go
// UpdateSubmission handles PUT /api/submissions/:id — manager edits payload.
func UpdateSubmission(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			Payload map[string]any `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		payloadBytes, _ := json.Marshal(body.Payload)
		tag, err := db.Exec(r.Context(),
			`UPDATE tourist_submissions
			     SET payload = $1, updated_at = NOW()
			   WHERE id = $2`, payloadBytes, id)
		if err != nil {
			slog.Error("update submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/api/submissions.go
git commit -m "feat(api): PUT /api/submissions/:id — edit payload"
```

---

### Task 21: DELETE /api/submissions/:id — archive

**Files:**
- Modify: `backend/internal/api/submissions.go`

- [ ] **Step 1: Append handler**

Append to `backend/internal/api/submissions.go`:

```go
// ArchiveSubmission handles DELETE /api/submissions/:id — soft archive.
func ArchiveSubmission(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		tag, err := db.Exec(r.Context(),
			`UPDATE tourist_submissions SET status = 'archived', updated_at = NOW()
			    WHERE id = $1 AND status != 'archived'`, id)
		if err != nil {
			slog.Error("archive submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "not found or already archived")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/api/submissions.go
git commit -m "feat(api): DELETE /api/submissions/:id — archive"
```

---

### Task 22: DELETE /api/submissions/:id/erase — hard delete

**Files:**
- Modify: `backend/internal/api/submissions.go`

- [ ] **Step 1: Append handler**

Append to `backend/internal/api/submissions.go`:

```go
// EraseSubmission handles DELETE /api/submissions/:id/erase — hard delete.
// Clears submission_snapshot on attached tourists in the same transaction.
func EraseSubmission(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		ctx := r.Context()
		tx, err := db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx begin")
			return
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		if _, err := tx.Exec(ctx,
			`UPDATE tourists SET submission_snapshot = NULL, submission_id = NULL
			    WHERE submission_id = $1`, id); err != nil {
			slog.Error("erase tourists snapshot", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		tag, err := tx.Exec(ctx, `DELETE FROM tourist_submissions WHERE id = $1`, id)
		if err != nil {
			slog.Error("delete submission", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "tx commit")
			return
		}
		slog.Info("submission erased", "id", id)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/api/submissions.go
git commit -m "feat(api): DELETE /api/submissions/:id/erase — hard delete"
```

---

### Task 23: POST /api/submissions/:id/attach

**Files:**
- Modify: `backend/internal/api/submissions.go`

- [ ] **Step 1: Append handler with race-safe transaction**

Append to `backend/internal/api/submissions.go`:

```go
// AttachSubmission handles POST /api/submissions/:id/attach
// Body: { "group_id": "...", "subgroup_id": "..." | null }
// Transaction + row lock protects against concurrent attach.
func AttachSubmission(db *pgxpool.Pool) http.HandlerFunc {
	type reqBody struct {
		GroupID    string  `json:"group_id"`
		SubgroupID *string `json:"subgroup_id"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		submissionID := chi.URLParam(r, "id")
		var body reqBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.GroupID == "" {
			writeError(w, http.StatusBadRequest, "group_id required")
			return
		}

		ctx := r.Context()
		tx, err := db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx begin")
			return
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		var payload []byte
		var status string
		err = tx.QueryRow(ctx,
			`SELECT payload, status FROM tourist_submissions
			    WHERE id = $1 FOR UPDATE`, submissionID).Scan(&payload, &status)
		if isNoRows(err) {
			writeError(w, http.StatusNotFound, "submission not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "select for update")
			return
		}
		if status == "attached" {
			writeError(w, http.StatusConflict, "submission already attached")
			return
		}

		var touristID string
		err = tx.QueryRow(ctx,
			`INSERT INTO tourists (group_id, subgroup_id, submission_id, submission_snapshot)
			 VALUES ($1, $2, $3, $4)
			 RETURNING id`,
			body.GroupID, body.SubgroupID, submissionID, payload).Scan(&touristID)
		if err != nil {
			slog.Error("insert tourist on attach", "err", err)
			writeError(w, http.StatusInternalServerError, "insert tourist")
			return
		}
		if _, err := tx.Exec(ctx,
			`UPDATE tourist_submissions SET status = 'attached', updated_at = NOW() WHERE id = $1`,
			submissionID); err != nil {
			writeError(w, http.StatusInternalServerError, "update submission status")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			writeError(w, http.StatusInternalServerError, "tx commit")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"tourist_id": touristID})
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/api/submissions.go
git commit -m "feat(api): POST /api/submissions/:id/attach — transactional attach"
```

---

### Task 24: PUT /api/tourists/:id/flight_data — manual flight entry

**Files:**
- Create: `backend/internal/api/flight_data.go`

- [ ] **Step 1: Implement**

Create `backend/internal/api/flight_data.go`:

```go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UpdateFlightData handles PUT /api/tourists/:id/flight_data
// Body: { "arrival": {...}, "departure": {...} }  (departure may be empty)
func UpdateFlightData(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		buf, _ := json.Marshal(body)
		tag, err := db.Exec(r.Context(),
			`UPDATE tourists SET flight_data = $1, updated_at = NOW() WHERE id = $2`, buf, id)
		if err != nil {
			slog.Error("update flight_data", "err", err)
			writeError(w, http.StatusInternalServerError, "database error")
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "tourist not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/api/flight_data.go
git commit -m "feat(api): PUT /api/tourists/:id/flight_data — manual flight entry"
```

---

### Task 25: GET /api/consent/text

**Files:**
- Modify: `backend/internal/api/submissions.go`

- [ ] **Step 1: Append handler**

Append to `backend/internal/api/submissions.go`:

```go
// GetConsentText returns the current consent agreement text + version.
func GetConsentText() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := consent.Current()
		writeJSON(w, http.StatusOK, map[string]string{
			"version": a.Version,
			"body":    a.Body,
		})
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add backend/internal/api/submissions.go
git commit -m "feat(api): GET /api/consent/text"
```

---

## Phase F — Modify Existing Handlers

### Task 26: Modify `uploads.go` — restrict file types + auto-parse

**Files:**
- Modify: `backend/internal/api/uploads.go`

- [ ] **Step 1: Read the file**

Read `backend/internal/api/uploads.go` to understand the current structure (four handlers: `ListUploads`, `UploadTouristFile`, `ListTouristUploads`, `UploadFile`).

- [ ] **Step 2: Delete group-level handlers, keep only tourist-level**

Remove `ListUploads` and `UploadFile` entirely. Only keep `UploadTouristFile` and `ListTouristUploads`.

- [ ] **Step 3: Replace body of `UploadTouristFile` to validate file_type and trigger parser**

Update `UploadTouristFile` so that:
- It rejects requests where form value `file_type` is not in `{"ticket", "voucher"}`.
- After upload completes, if `file_type == "ticket"`, call `ai.ParseTicket` with the just-uploaded file and persist result into `tourists.flight_data`.
- If `file_type == "voucher"`, call `ai.ParseVouchers` and upsert `hotels` + `group_hotels` rows.
- Parser errors are logged and returned as a non-fatal 200 response with `{"parse_error": "..."}`; the file itself is still saved.

Illustrative snippet (adapt to existing handler style):

```go
// Inside UploadTouristFile, after saving the file and uploading to Anthropic Files API:
switch fileType {
case "ticket":
    file := ai.FileInput{AnthropicFileID: anthropicFileID, Name: filename}
    flights, err := ai.ParseTicket(r.Context(), apiKey, []ai.FileInput{file})
    if err != nil {
        slog.Warn("ticket parse failed", "err", err)
        writeJSON(w, http.StatusOK, map[string]any{"id": uploadID, "parse_error": err.Error()})
        return
    }
    // Persist into flight_data
    buf, _ := json.Marshal(map[string]any{
        "arrival":   flights.Arrival,
        "departure": flights.Departure,
    })
    if _, err := db.Exec(r.Context(),
        `UPDATE tourists SET flight_data = $1, updated_at = NOW() WHERE id = $2`,
        buf, touristID); err != nil {
        slog.Warn("update flight_data", "err", err)
    }

case "voucher":
    file := ai.FileInput{AnthropicFileID: anthropicFileID, Name: filename}
    hotels, err := ai.ParseVouchers(r.Context(), apiKey, []ai.FileInput{file})
    if err != nil {
        slog.Warn("voucher parse failed", "err", err)
        writeJSON(w, http.StatusOK, map[string]any{"id": uploadID, "parse_error": err.Error()})
        return
    }
    // For each hotel: lookup by name (case-insensitive), create if missing,
    // then insert a row into group_hotels for the tourist's group/subgroup.
    for _, h := range hotels {
        var hotelID string
        err := db.QueryRow(r.Context(),
            `INSERT INTO hotels (name_en, city, address, phone)
             VALUES ($1, $2, $3, $4)
             ON CONFLICT (name_en) DO UPDATE SET name_en = EXCLUDED.name_en
             RETURNING id`,
            h.Name, h.City, h.Address, h.Phone).Scan(&hotelID)
        if err != nil {
            slog.Warn("upsert hotel", "err", err)
            continue
        }
        _, _ = db.Exec(r.Context(),
            `INSERT INTO group_hotels (group_id, subgroup_id, hotel_id, check_in, check_out, sort_order)
             SELECT group_id, subgroup_id, $1, $2, $3,
                    (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM group_hotels WHERE group_id = tourists.group_id)
               FROM tourists WHERE id = $4`,
            hotelID, h.CheckIn, h.CheckOut, touristID)
    }
}
```

If the `hotels` table lacks a unique constraint on `name_en`, add one in the migration beforehand or skip the `ON CONFLICT`.

- [ ] **Step 4: Verify build**

Run: `cd backend && go build ./...`

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/uploads.go
git commit -m "feat(api): uploads — restrict to ticket/voucher + auto-parse"
```

---

### Task 27: Modify `tourists.go` — drop match endpoint + legacy fields

**Files:**
- Modify: `backend/internal/api/tourists.go`

- [ ] **Step 1: Remove `ConfirmMatch` handler and all references to `raw_json` / `matched_sheet_row` / `match_confirmed`**

Open `backend/internal/api/tourists.go`. Delete:
- `ConfirmMatch` function
- `RawJSON`, `MatchedSheetRow`, `MatchConfirmed` fields from the `Tourist` struct
- All `SELECT` / `INSERT` references to those columns — replace with `submission_id`, `submission_snapshot`, `flight_data`

Example replacement for the Tourist struct:

```go
type Tourist struct {
	ID                 string          `json:"id"`
	GroupID            string          `json:"group_id"`
	SubgroupID         *string         `json:"subgroup_id"`
	SubmissionID       *string         `json:"submission_id"`
	SubmissionSnapshot json.RawMessage `json:"submission_snapshot"`
	FlightData         json.RawMessage `json:"flight_data"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}
```

Update `ListTourists` SQL:

```go
rows, err := db.Query(r.Context(),
    `SELECT id, group_id, subgroup_id, submission_id, submission_snapshot, flight_data,
            created_at, updated_at
       FROM tourists WHERE group_id = $1 ORDER BY created_at`, groupID)
```

Remove `AddTouristFromSheet` entirely (attachment now happens through `POST /api/submissions/:id/attach`).

- [ ] **Step 2: Verify build**

Run: `cd backend && go build ./...`

- [ ] **Step 3: Commit**

```bash
git add backend/internal/api/tourists.go
git commit -m "refactor(api): tourists — drop match + sheet-based creation"
```

---

### Task 28: Rewrite `generate.go` — new AI pipeline orchestration

**Files:**
- Modify: `backend/internal/api/generate.go`

- [ ] **Step 1: Read existing generate.go to understand the current docgen integration**

Open `backend/internal/api/generate.go`. The current flow calls `ai.FormatDocuments` (Pass 2) then invokes Python docgen. Keep the downstream Python invocation part unchanged; replace the AI call with the new pipeline.

- [ ] **Step 2: Replace the AI section**

The new `GenerateDocuments` handler does:
1. Load all tourists in the group (with `submission_snapshot`, `flight_data`).
2. Load hotels joined from `group_hotels` + `hotels`.
3. For each tourist, collect the free-text fields to translate into a de-duplicated list.
4. Call `ai.TranslateStrings` once for the whole batch; cache per-tourist into `tourists.translations`.
5. Determine dates and flights for the programme (use first tourist with populated flights).
6. Call `ai.GenerateProgramme`.
7. Call `ai.AssemblePass2`.
8. Marshal to JSON, write to the same temp path the Python subprocess expects, invoke docgen, return zip.

Use `golang.org/x/sync/errgroup` for parallelism (add to go.mod: `go get golang.org/x/sync`).

Key changes to the orchestration body:

```go
// Pseudocode fleshed out in the actual file — full code follows:
tourists, err := loadTouristsForGeneration(ctx, db, groupID)  // payload + flight_data per tourist
if err != nil { ... }
hotels, err := loadGroupHotels(ctx, db, groupID)
if err != nil { ... }

// Collect distinct free-text strings across the whole group.
freeTextFields := []string{"place_of_birth_ru", "issued_by_ru", "home_address_ru",
    "occupation_ru", "employer_ru", "employer_address_ru",
    "previous_visits_ru", "nationality_ru"}
uniqueStrings, indexPerTourist := collectFreeText(tourists, freeTextFields)

// Parallel: translate + programme
var translations []string
var programme []ai.ProgrammeDay
g, gctx := errgroup.WithContext(ctx)
g.Go(func() error {
    t, err := ai.TranslateStrings(gctx, anthropicKey, uniqueStrings)
    translations = t
    return err
})
g.Go(func() error {
    input := buildProgrammeInput(tourists, hotels)
    p, err := ai.GenerateProgramme(gctx, anthropicKey, input)
    programme = p
    return err
})
if err := g.Wait(); err != nil {
    writeError(w, http.StatusInternalServerError, fmt.Sprintf("ai pipeline: %s", err))
    return
}

// Per-tourist translation map
translationMaps := make([]map[string]string, len(tourists))
for i, idx := range indexPerTourist {
    m := make(map[string]string, len(idx))
    for src, k := range idx {
        if k >= 0 && k < len(translations) {
            m[src] = translations[k]
        }
    }
    translationMaps[i] = m
}

// Cache translations into tourists.translations
for i, t := range tourists {
    buf, _ := json.Marshal(translationMaps[i])
    _, _ = db.Exec(ctx, `UPDATE tourists SET translations = $1 WHERE id = $2`, buf, t.ID)
}

pass2 := ai.AssemblePass2(
    touristPayloads(tourists),
    translationMaps,
    touristFlights(tourists),
    programme,
    toHotelBriefs(hotels),
    time.Now().Format("02.01.2006"),
)

// Then write pass2.json and invoke Python docgen (keep existing logic).
pass2Bytes, _ := json.MarshalIndent(pass2, "", "  ")
// ...existing docgen subprocess call unchanged...
```

Helper functions `collectFreeText`, `buildProgrammeInput`, `loadTouristsForGeneration`, `loadGroupHotels`, `touristPayloads`, `touristFlights`, `toHotelBriefs` live in the same file (private to package).

- [ ] **Step 3: Add errgroup dependency if missing**

Run:
```bash
cd backend && go get golang.org/x/sync/errgroup && go mod tidy
```

- [ ] **Step 4: Verify build**

Run: `cd backend && go build ./...`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/generate.go backend/go.mod backend/go.sum
git commit -m "refactor(api): generate — new translate+programme+assembler pipeline"
```

---

## Phase G — Route Wiring + Legacy Deletion

### Task 29: Rewire routes in `main.go`

**Files:**
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: Remove Google Sheets bootstrap**

Delete the block that creates `sheetsClient` (lines creating the sheets client and the `if sheetsClient != nil { ... } else { ... }` guards).

Remove imports: `"fujitravel-admin/backend/internal/sheets"`.

Remove `googleCredsPath` and `sheetID` env-var reads.

- [ ] **Step 2: Remove deleted endpoints from router**

Delete these `r.Get(...)` / `r.Post(...)` lines:
- `r.Get("/groups/{id}/uploads", api.ListUploads(...))`
- `r.Post("/groups/{id}/uploads", api.UploadFile(...))`
- `r.Post("/groups/{id}/parse", api.ParseGroup(...))`
- `r.Post("/tourists/{id}/match", api.ConfirmMatch(...))`
- `r.Post("/tourists/{id}/parse", api.ParseTourist(...))`
- `r.Post("/subgroups/{id}/parse", api.ParseSubgroup(...))`
- `r.Post("/groups/{id}/tourists", api.AddTouristFromSheet(...))`
- `r.Get("/sheets/search", api.SearchSheets(...))`
- `r.Get("/sheets/rows", api.ListSheetRows(...))`

- [ ] **Step 3: Add new routes**

Add the new route block inside `r.Route("/api", ...)`:

```go
// Submissions (form-based workflow)
r.Post("/submissions", api.CreateSubmission(pool))        // public
r.Get("/submissions", api.ListSubmissions(pool))           // admin
r.Get("/submissions/{id}", api.GetSubmission(pool))
r.Put("/submissions/{id}", api.UpdateSubmission(pool))
r.Delete("/submissions/{id}", api.ArchiveSubmission(pool))
r.Delete("/submissions/{id}/erase", api.EraseSubmission(pool))
r.Post("/submissions/{id}/attach", api.AttachSubmission(pool))
r.Get("/consent/text", api.GetConsentText())

// Flight data
r.Put("/tourists/{id}/flight_data", api.UpdateFlightData(pool))
```

- [ ] **Step 4: Verify build**

Run: `cd backend && go build ./...`

- [ ] **Step 5: Commit**

```bash
git add backend/cmd/server/main.go
git commit -m "refactor(server): wire new form-workflow routes, drop scan/sheets routes"
```

---

### Task 30: Delete legacy files — pass1, pass2, files

**Files:**
- Delete: `backend/internal/ai/pass1.go`
- Delete: `backend/internal/ai/pass2.go`
- Delete: `backend/internal/ai/files.go`

- [ ] **Step 1: Remove files**

Run:
```bash
cd /Users/yaufdd/Desktop/fujitravel-admin
rm backend/internal/ai/pass1.go backend/internal/ai/pass2.go backend/internal/ai/files.go
```

- [ ] **Step 2: Verify build**

Run: `cd backend && go build ./...`
Expected: no errors (all callers were rewired in Phase F).

- [ ] **Step 3: Commit**

```bash
git add -A backend/internal/ai
git commit -m "chore(ai): delete legacy pass1/pass2/files — superseded by new pipeline"
```

---

### Task 31: Delete sheets + matcher packages

**Files:**
- Delete: `backend/internal/sheets/` (whole dir)
- Delete: `backend/internal/matcher/` (whole dir)

- [ ] **Step 1: Remove directories**

Run:
```bash
cd /Users/yaufdd/Desktop/fujitravel-admin
rm -rf backend/internal/sheets backend/internal/matcher
```

- [ ] **Step 2: Remove unused dependencies**

Run:
```bash
cd backend && go mod tidy
```
This should remove `google.golang.org/api` and `golang.org/x/oauth2` from `go.mod`.

- [ ] **Step 3: Verify build**

Run: `cd backend && go build ./...`

- [ ] **Step 4: Commit**

```bash
git add -A backend
git commit -m "chore: drop sheets + matcher packages + google API deps"
```

---

### Task 32: Delete legacy API handlers

**Files:**
- Delete: `backend/internal/api/sheetsearch.go`
- Delete: `backend/internal/api/parse.go`

- [ ] **Step 1: Remove files**

Run:
```bash
cd /Users/yaufdd/Desktop/fujitravel-admin
rm backend/internal/api/sheetsearch.go backend/internal/api/parse.go
```

- [ ] **Step 2: Verify build**

Run: `cd backend && go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add -A backend/internal/api
git commit -m "chore(api): drop legacy sheetsearch.go + parse.go"
```

---

### Task 33: Run full backend test suite

- [ ] **Step 1: Run all tests**

Run: `cd backend && go test ./... -v`
Expected: all pass.

- [ ] **Step 2: Start backend locally**

Run:
```bash
cd backend && go run cmd/server/main.go
```
Expected: no panics, logs show `database connected` and `starting server`.

Verify with curl in a second terminal:
```bash
curl http://localhost:8080/api/consent/text
curl http://localhost:8080/api/submissions
```

- [ ] **Step 3: Stop server (Ctrl-C)**

---

## Phase H — Frontend Public Form

### Task 34: Add submissions API client methods

**Files:**
- Modify: `frontend/src/api/client.js`

- [ ] **Step 1: Read current client.js to match style**

- [ ] **Step 2: Add new functions**

Append to `frontend/src/api/client.js`:

```javascript
// ── Submissions (form-workflow) ──
export async function createSubmission(payload, consentAccepted, source = 'tourist') {
  const res = await fetch(`${API_BASE}/submissions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ payload, consent_accepted: consentAccepted, source }),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function listSubmissions(q = '', status = '') {
  const params = new URLSearchParams()
  if (q) params.set('q', q)
  if (status) params.set('status', status)
  const res = await fetch(`${API_BASE}/submissions?${params}`)
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function getSubmission(id) {
  const res = await fetch(`${API_BASE}/submissions/${id}`)
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function updateSubmission(id, payload) {
  const res = await fetch(`${API_BASE}/submissions/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ payload }),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function archiveSubmission(id) {
  const res = await fetch(`${API_BASE}/submissions/${id}`, { method: 'DELETE' })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function attachSubmission(id, groupId, subgroupId = null) {
  const res = await fetch(`${API_BASE}/submissions/${id}/attach`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ group_id: groupId, subgroup_id: subgroupId }),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function getConsentText() {
  const res = await fetch(`${API_BASE}/consent/text`)
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function updateFlightData(touristId, data) {
  const res = await fetch(`${API_BASE}/tourists/${touristId}/flight_data`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

// Remove the old listSheetRows / searchSheets exports since the endpoints are gone.
```

Also remove any existing `listSheetRows` / `searchSheets` functions that reference the deleted endpoints.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/api/client.js
git commit -m "feat(frontend): add submissions + flight_data API client methods"
```

---

### Task 35: Build shared `SubmissionForm` component

**Files:**
- Create: `frontend/src/components/SubmissionForm.jsx`

- [ ] **Step 1: Implement the shared form**

Create `frontend/src/components/SubmissionForm.jsx`:

```jsx
import { useState, useEffect } from 'react'
import { getConsentText } from '../api/client'

/**
 * SubmissionForm — shared between public /form and admin "Create manually".
 * Props:
 *   onSubmit(payload, consentAccepted) → Promise
 *   initialPayload (optional) — for edit mode
 *   submitLabel (default "Отправить анкету")
 *   showConsent (default true) — admin edit mode can hide
 */
export default function SubmissionForm({
  onSubmit, initialPayload = {}, submitLabel = 'Отправить анкету',
  showConsent = true,
}) {
  const [p, setP] = useState({
    name_lat: '', name_cyr: '', gender_ru: '', birth_date: '',
    marital_status_ru: '', place_of_birth_ru: '',
    nationality_ru: '', former_nationality_ru: '',
    passport_number: '', passport_type_ru: 'Обычный',
    issued_by_ru: '', issue_date: '', expiry_date: '',
    home_address_ru: '', phone: '',
    occupation_ru: '', employer_ru: '', employer_address_ru: '',
    criminal_record_ru: 'Нет', been_to_japan_ru: 'Нет',
    previous_visits_ru: '', maiden_name_ru: '',
    internal_series: '', internal_number: '',
    internal_issued_ru: '', internal_issued_by_ru: '',
    reg_address_ru: '',
    ...initialPayload,
  })
  const [consent, setConsent] = useState(false)
  const [consentText, setConsentText] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState(null)

  useEffect(() => {
    if (showConsent) getConsentText().then(setConsentText).catch(() => {})
  }, [showConsent])

  const set = (k) => (e) => setP({ ...p, [k]: e.target.value })
  const upperOnChange = (k) => (e) => setP({ ...p, [k]: e.target.value.toUpperCase() })

  const valid =
    p.name_lat && /^[A-Z ]+$/.test(p.name_lat) &&
    p.name_cyr &&
    p.gender_ru && p.birth_date && p.passport_number &&
    p.issue_date && p.expiry_date &&
    /^\d{4}$/.test(p.internal_series) &&
    /^\d{6}$/.test(p.internal_number) &&
    p.phone && p.home_address_ru &&
    (!showConsent || consent)

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!valid || submitting) return
    setSubmitting(true)
    setError(null)
    try {
      await onSubmit(p, consent)
    } catch (err) {
      setError(err.message || 'Ошибка отправки')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="submission-form">
      <Section title="Персональные данные">
        <Input label="ФИО латиницей (как в загранпаспорте)" value={p.name_lat} onChange={upperOnChange('name_lat')} required />
        <Input label="ФИО кириллицей (как в паспорте РФ)" value={p.name_cyr} onChange={set('name_cyr')} required />
        <Select label="Пол" value={p.gender_ru} onChange={set('gender_ru')} options={['', 'Мужской', 'Женский']} required />
        <Input label="Дата рождения (ДД.ММ.ГГГГ)" value={p.birth_date} onChange={set('birth_date')} required />
        <Select label="Семейное положение" value={p.marital_status_ru} onChange={set('marital_status_ru')} options={['', 'Холост/не замужем', 'Женат/замужем', 'Вдовец/вдова', 'Разведен(а)']} />
        <Input label="Место рождения" value={p.place_of_birth_ru} onChange={set('place_of_birth_ru')} />
        <Input label="Гражданство" value={p.nationality_ru} onChange={set('nationality_ru')} />
        <Input label="Прежнее гражданство (если было)" value={p.former_nationality_ru} onChange={set('former_nationality_ru')} />
        <Input label="Девичья фамилия (если была)" value={p.maiden_name_ru} onChange={set('maiden_name_ru')} />
      </Section>

      <Section title="Загранпаспорт">
        <Input label="Номер" value={p.passport_number} onChange={set('passport_number')} required />
        <Select label="Вид" value={p.passport_type_ru} onChange={set('passport_type_ru')} options={['Обычный', 'Дипломатический', 'Служебный']} />
        <Input label="Кем выдан" value={p.issued_by_ru} onChange={set('issued_by_ru')} />
        <Input label="Когда выдан (ДД.ММ.ГГГГ)" value={p.issue_date} onChange={set('issue_date')} required />
        <Input label="Действителен до (ДД.ММ.ГГГГ)" value={p.expiry_date} onChange={set('expiry_date')} required />
      </Section>

      <Section title="Внутренний паспорт РФ">
        <Input label="Серия (4 цифры)" value={p.internal_series} onChange={set('internal_series')} required maxLength={4} />
        <Input label="Номер (6 цифр)" value={p.internal_number} onChange={set('internal_number')} required maxLength={6} />
        <Input label="Кем выдан" value={p.internal_issued_by_ru} onChange={set('internal_issued_by_ru')} />
        <Input label="Дата выдачи (ДД.ММ.ГГГГ)" value={p.internal_issued_ru} onChange={set('internal_issued_ru')} />
        <Input label="Адрес регистрации" value={p.reg_address_ru} onChange={set('reg_address_ru')} />
      </Section>

      <Section title="Контакты и адрес">
        <Input label="Домашний адрес" value={p.home_address_ru} onChange={set('home_address_ru')} required />
        <Input label="Телефон" value={p.phone} onChange={set('phone')} required />
      </Section>

      <Section title="Работа">
        <Input label="Занимаемая должность" value={p.occupation_ru} onChange={set('occupation_ru')} />
        <Input label="Название предприятия" value={p.employer_ru} onChange={set('employer_ru')} />
        <Input label="Адрес офиса и телефон" value={p.employer_address_ru} onChange={set('employer_address_ru')} />
      </Section>

      <Section title="История">
        <Select label="Была ли судимость" value={p.criminal_record_ru} onChange={set('criminal_record_ru')} options={['Нет', 'Да']} />
        <Select label="Был ли в Японии" value={p.been_to_japan_ru} onChange={set('been_to_japan_ru')} options={['Нет', 'Да']} />
        <Input label="Даты прошлых визитов (если был)" value={p.previous_visits_ru} onChange={set('previous_visits_ru')} />
      </Section>

      {showConsent && (
        <Section title="Согласие">
          {consentText && (
            <details>
              <summary>Прочитать полный текст согласия</summary>
              <pre className="consent-text">{consentText.body}</pre>
            </details>
          )}
          <label className="consent-checkbox">
            <input type="checkbox" checked={consent} onChange={(e) => setConsent(e.target.checked)} />
            Я прочитал и принимаю условия обработки персональных данных
          </label>
        </Section>
      )}

      {error && <div className="form-error">{error}</div>}
      <button type="submit" disabled={!valid || submitting}>
        {submitting ? 'Отправка…' : submitLabel}
      </button>
    </form>
  )
}

function Section({ title, children }) {
  return (
    <fieldset className="form-section">
      <legend>{title}</legend>
      {children}
    </fieldset>
  )
}

function Input({ label, ...rest }) {
  return (
    <label className="form-field">
      <span>{label}{rest.required && ' *'}</span>
      <input {...rest} />
    </label>
  )
}

function Select({ label, options, ...rest }) {
  return (
    <label className="form-field">
      <span>{label}{rest.required && ' *'}</span>
      <select {...rest}>
        {options.map((o) => <option key={o} value={o}>{o || '—'}</option>)}
      </select>
    </label>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/SubmissionForm.jsx
git commit -m "feat(frontend): shared SubmissionForm component"
```

---

### Task 36: ConsentPage

**Files:**
- Create: `frontend/src/pages/ConsentPage.jsx`

- [ ] **Step 1: Implement**

```jsx
import { useEffect, useState } from 'react'
import { getConsentText } from '../api/client'

export default function ConsentPage() {
  const [c, setC] = useState(null)
  useEffect(() => { getConsentText().then(setC) }, [])
  if (!c) return <div>Загрузка…</div>
  return (
    <article className="consent-page">
      <h1>Согласие на обработку персональных данных</h1>
      <p className="version">Версия: {c.version}</p>
      <pre className="consent-text">{c.body}</pre>
    </article>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/pages/ConsentPage.jsx
git commit -m "feat(frontend): public /consent page"
```

---

### Task 37: SubmissionFormPage (public /form)

**Files:**
- Create: `frontend/src/pages/SubmissionFormPage.jsx`

- [ ] **Step 1: Implement**

```jsx
import { useNavigate } from 'react-router-dom'
import SubmissionForm from '../components/SubmissionForm'
import { createSubmission } from '../api/client'

export default function SubmissionFormPage() {
  const nav = useNavigate()
  const handleSubmit = async (payload, consent) => {
    await createSubmission(payload, consent, 'tourist')
    nav('/form/thanks')
  }
  return (
    <main className="public-form">
      <h1>Анкета для оформления визы в Японию</h1>
      <p className="lead">FujiTravel · пожалуйста, заполните данные как в паспорте.</p>
      <SubmissionForm onSubmit={handleSubmit} />
    </main>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/pages/SubmissionFormPage.jsx
git commit -m "feat(frontend): public /form page"
```

---

### Task 38: FormThanksPage

**Files:**
- Create: `frontend/src/pages/FormThanksPage.jsx`

- [ ] **Step 1: Implement**

```jsx
export default function FormThanksPage() {
  return (
    <main className="thanks-page">
      <h1>Спасибо!</h1>
      <p>Ваша анкета отправлена. Менеджер свяжется с вами в ближайшее время.</p>
    </main>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/pages/FormThanksPage.jsx
git commit -m "feat(frontend): /form/thanks page"
```

---

## Phase I — Frontend Admin UI

### Task 39: SubmissionsListPage

**Files:**
- Create: `frontend/src/pages/SubmissionsListPage.jsx`

- [ ] **Step 1: Implement**

```jsx
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { listSubmissions } from '../api/client'

export default function SubmissionsListPage() {
  const [rows, setRows] = useState([])
  const [q, setQ] = useState('')
  const [status, setStatus] = useState('pending')

  const load = () => listSubmissions(q, status).then(setRows)
  useEffect(() => { load() }, [q, status])

  return (
    <div className="submissions-page">
      <div className="toolbar">
        <h1>Анкеты</h1>
        <Link className="btn" to="/submissions/new">+ Создать вручную</Link>
      </div>
      <div className="filters">
        <input placeholder="Поиск по имени" value={q} onChange={(e) => setQ(e.target.value)} />
        <select value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">Все</option>
          <option value="pending">Новые</option>
          <option value="attached">Привязаны</option>
          <option value="archived">Архив</option>
        </select>
      </div>
      <table className="submissions-table">
        <thead>
          <tr><th>Имя</th><th>Статус</th><th>Дата</th><th>Источник</th></tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const p = JSON.parse(r.payload)
            return (
              <tr key={r.id} onClick={() => window.location.assign(`/submissions/${r.id}`)}>
                <td>{p.name_lat}</td>
                <td>{statusLabel(r.status)}</td>
                <td>{new Date(r.created_at).toLocaleDateString('ru')}</td>
                <td>{r.source === 'tourist' ? 'Турист' : 'Менеджер'}</td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function statusLabel(s) {
  return { pending: 'Новая', attached: 'Привязана', archived: 'Архив', deleted: 'Удалена' }[s] || s
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/pages/SubmissionsListPage.jsx
git commit -m "feat(frontend): /submissions list page"
```

---

### Task 40: SubmissionDetailPage + AttachGroupModal

**Files:**
- Create: `frontend/src/pages/SubmissionDetailPage.jsx`
- Create: `frontend/src/components/AttachGroupModal.jsx`

- [ ] **Step 1: Implement AttachGroupModal**

Create `frontend/src/components/AttachGroupModal.jsx`:

```jsx
import { useEffect, useState } from 'react'
import { listGroups, listSubgroups } from '../api/client'

export default function AttachGroupModal({ onClose, onConfirm }) {
  const [groups, setGroups] = useState([])
  const [subgroups, setSubgroups] = useState([])
  const [groupId, setGroupId] = useState('')
  const [subgroupId, setSubgroupId] = useState('')

  useEffect(() => { listGroups().then(setGroups) }, [])
  useEffect(() => {
    if (groupId) listSubgroups(groupId).then(setSubgroups)
    else setSubgroups([])
  }, [groupId])

  return (
    <div className="modal">
      <div className="modal-body">
        <h2>Привязать к группе</h2>
        <label>Группа
          <select value={groupId} onChange={(e) => setGroupId(e.target.value)}>
            <option value="">— выберите —</option>
            {groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}
          </select>
        </label>
        {subgroups.length > 0 && (
          <label>Подгруппа
            <select value={subgroupId} onChange={(e) => setSubgroupId(e.target.value)}>
              <option value="">— без подгруппы —</option>
              {subgroups.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
            </select>
          </label>
        )}
        <div className="modal-actions">
          <button onClick={onClose}>Отмена</button>
          <button onClick={() => onConfirm(groupId, subgroupId || null)} disabled={!groupId}>Привязать</button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Implement SubmissionDetailPage**

Create `frontend/src/pages/SubmissionDetailPage.jsx`:

```jsx
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import SubmissionForm from '../components/SubmissionForm'
import AttachGroupModal from '../components/AttachGroupModal'
import { getSubmission, updateSubmission, archiveSubmission, attachSubmission, createSubmission } from '../api/client'

export default function SubmissionDetailPage() {
  const { id } = useParams()
  const nav = useNavigate()
  const [s, setS] = useState(null)
  const [showAttach, setShowAttach] = useState(false)
  const isNew = id === 'new'

  useEffect(() => {
    if (!isNew) getSubmission(id).then(setS)
  }, [id])

  const handleSave = async (payload) => {
    if (isNew) {
      const { id: newId } = await createSubmission(payload, true, 'manager')
      nav(`/submissions/${newId}`)
    } else {
      await updateSubmission(id, payload)
      alert('Сохранено')
    }
  }

  const handleArchive = async () => {
    if (!confirm('Архивировать анкету?')) return
    await archiveSubmission(id)
    nav('/submissions')
  }

  const handleAttach = async (groupId, subgroupId) => {
    const { tourist_id } = await attachSubmission(id, groupId, subgroupId)
    setShowAttach(false)
    nav(`/groups/${groupId}`)
  }

  if (!isNew && !s) return <div>Загрузка…</div>

  const initialPayload = isNew ? {} : JSON.parse(s.payload)

  return (
    <div className="submission-detail">
      <h1>{isNew ? 'Новая анкета' : 'Анкета'}</h1>
      <SubmissionForm
        initialPayload={initialPayload}
        onSubmit={handleSave}
        submitLabel={isNew ? 'Создать' : 'Сохранить'}
        showConsent={isNew}
      />
      {!isNew && s.status !== 'attached' && (
        <div className="detail-actions">
          <button onClick={() => setShowAttach(true)}>Привязать к группе</button>
          <button onClick={handleArchive}>Архивировать</button>
        </div>
      )}
      {showAttach && <AttachGroupModal onClose={() => setShowAttach(false)} onConfirm={handleAttach} />}
    </div>
  )
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/pages/SubmissionDetailPage.jsx frontend/src/components/AttachGroupModal.jsx
git commit -m "feat(frontend): /submissions/:id detail page + attach modal"
```

---

### Task 41: Replace AddFromSheetModal with AddFromDBModal

**Files:**
- Delete: `frontend/src/components/AddFromSheetModal.jsx`
- Create: `frontend/src/components/AddFromDBModal.jsx`

- [ ] **Step 1: Implement AddFromDBModal**

```jsx
import { useEffect, useState } from 'react'
import { listSubmissions, attachSubmission } from '../api/client'

export default function AddFromDBModal({ groupId, subgroupId, onClose, onAdded }) {
  const [rows, setRows] = useState([])
  const [q, setQ] = useState('')

  const load = () => listSubmissions(q, 'pending').then(setRows)
  useEffect(() => { load() }, [q])

  const attach = async (id) => {
    await attachSubmission(id, groupId, subgroupId)
    onAdded()
  }

  return (
    <div className="modal">
      <div className="modal-body">
        <div className="modal-header">
          <h2>Добавить туриста</h2>
          <button onClick={onClose}>×</button>
        </div>
        <input autoFocus placeholder="Поиск по имени" value={q} onChange={(e) => setQ(e.target.value)} />
        <ul className="submission-list">
          {rows.map((r) => {
            const p = JSON.parse(r.payload)
            return (
              <li key={r.id}>
                <span>{p.name_lat}</span>
                <span className="muted">паспорт {p.passport_number}</span>
                <button onClick={() => attach(r.id)}>Добавить</button>
              </li>
            )
          })}
          {rows.length === 0 && <li className="empty">Нет анкет в пуле</li>}
        </ul>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Delete old modal**

Run: `rm frontend/src/components/AddFromSheetModal.jsx`

- [ ] **Step 3: Commit**

```bash
git add -A frontend/src/components
git commit -m "feat(frontend): AddFromDBModal replaces AddFromSheetModal"
```

---

### Task 42: FlightDataCard + FlightDataForm

**Files:**
- Create: `frontend/src/components/FlightDataForm.jsx`
- Create: `frontend/src/components/FlightDataCard.jsx`

- [ ] **Step 1: Implement FlightDataForm (modal)**

```jsx
import { useState } from 'react'

export default function FlightDataForm({ initial, onClose, onSave }) {
  const [a, setA] = useState(initial?.arrival || { flight_number: '', date: '', time: '', airport: '' })
  const [d, setD] = useState(initial?.departure || { flight_number: '', date: '', time: '', airport: '' })

  const save = async () => {
    await onSave({ arrival: a, departure: d.flight_number ? d : {} })
    onClose()
  }

  return (
    <div className="modal">
      <div className="modal-body">
        <h2>Данные рейсов</h2>
        <h3>Прилёт в Японию</h3>
        <FlightLeg leg={a} setLeg={setA} />
        <h3>Вылет из Японии</h3>
        <FlightLeg leg={d} setLeg={setD} />
        <p className="muted">Если билет только «туда» — оставьте поля вылета пустыми.</p>
        <div className="modal-actions">
          <button onClick={onClose}>Отмена</button>
          <button onClick={save}>Сохранить</button>
        </div>
      </div>
    </div>
  )
}

function FlightLeg({ leg, setLeg }) {
  const set = (k) => (e) => setLeg({ ...leg, [k]: e.target.value })
  return (
    <div className="flight-leg">
      <label>Рейс <input value={leg.flight_number} onChange={set('flight_number')} placeholder="SU 262" /></label>
      <label>Дата <input value={leg.date} onChange={set('date')} placeholder="ДД.ММ.ГГГГ" /></label>
      <label>Время <input value={leg.time} onChange={set('time')} placeholder="12:45" /></label>
      <label>Аэропорт <input value={leg.airport} onChange={set('airport')} placeholder="TOKYO NARITA" /></label>
    </div>
  )
}
```

- [ ] **Step 2: Implement FlightDataCard**

```jsx
import { useState } from 'react'
import FlightDataForm from './FlightDataForm'
import { updateFlightData } from '../api/client'

export default function FlightDataCard({ tourist, onUpdated }) {
  const [editing, setEditing] = useState(false)
  const fd = tourist.flight_data ? JSON.parse(tourist.flight_data) : {}
  const arr = fd.arrival || {}
  const dep = fd.departure || {}

  const save = async (data) => {
    await updateFlightData(tourist.id, data)
    onUpdated()
  }

  return (
    <div className="flight-card">
      <div className="flight-card-header">
        <span>✈️ Рейсы</span>
        <button onClick={() => setEditing(true)}>Изменить</button>
      </div>
      <div className="flight-leg-view">
        <strong>Прилёт:</strong> {arr.flight_number || '—'}, {arr.date} {arr.time} → {arr.airport || '—'}
      </div>
      <div className="flight-leg-view">
        <strong>Отлёт:</strong> {dep.flight_number ? `${dep.flight_number}, ${dep.date} ${dep.time} ← ${dep.airport}` : '— (one-way)'}
      </div>
      {editing && <FlightDataForm initial={fd} onClose={() => setEditing(false)} onSave={save} />}
    </div>
  )
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/FlightDataForm.jsx frontend/src/components/FlightDataCard.jsx
git commit -m "feat(frontend): FlightDataCard + FlightDataForm"
```

---

### Task 43: Modify GroupDetailPage

**Files:**
- Modify: `frontend/src/pages/GroupDetailPage.jsx`

- [ ] **Step 1: Remove passport/scan UI and AddFromSheetModal usage**

Open `frontend/src/pages/GroupDetailPage.jsx`. Remove:
- Drag-drop zone for passport/foreign_passport uploads
- "Parse group" / "Parse tourist" buttons
- Match confirmation UI
- Import + usage of `AddFromSheetModal`
- References to `raw_json` / `matched_sheet_row` / `match_confirmed` / `parse` API calls

Replace with:
- Import `AddFromDBModal` and use it when clicking "+ Добавить туриста"
- For each tourist card, render `<FlightDataCard tourist={t} onUpdated={reload} />`
- Keep the existing hotel management UI and generate buttons

- [ ] **Step 2: Verify frontend builds**

Run:
```bash
cd frontend && npm run build
```
Expected: builds without errors.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/pages/GroupDetailPage.jsx
git commit -m "refactor(frontend): GroupDetailPage — drop scan UI, adopt form-workflow"
```

---

### Task 44: Update App.jsx routing and main nav

**Files:**
- Modify: `frontend/src/App.jsx` (or equivalent router entry)
- Modify: main navigation component if separate

- [ ] **Step 1: Add new routes**

Add:
- `<Route path="/form" element={<SubmissionFormPage />} />`
- `<Route path="/form/thanks" element={<FormThanksPage />} />`
- `<Route path="/consent" element={<ConsentPage />} />`
- `<Route path="/submissions" element={<SubmissionsListPage />} />`
- `<Route path="/submissions/:id" element={<SubmissionDetailPage />} />`

- [ ] **Step 2: Add "Анкеты" to main nav**

In the top navigation, add a link to `/submissions` between "Отели" and (after group-related entries).

- [ ] **Step 3: Verify build**

Run: `cd frontend && npm run build`

- [ ] **Step 4: Commit**

```bash
git add frontend/src/App.jsx frontend/src/components/*.jsx
git commit -m "feat(frontend): routes for public form + admin submissions pages"
```

---

### Task 45: Styles for new pages

**Files:**
- Modify: `frontend/src/index.css` (or whichever global stylesheet exists)

- [ ] **Step 1: Add CSS**

Append to the global stylesheet:

```css
/* ── Submission form ─────────────────────────────────────── */
.submission-form .form-section { border: 1px solid var(--border); padding: 1rem; margin-bottom: 1rem; border-radius: 6px; }
.submission-form .form-section legend { padding: 0 0.5rem; font-weight: 600; }
.submission-form .form-field { display: flex; flex-direction: column; margin-bottom: 0.75rem; }
.submission-form .form-field span { font-size: 0.85rem; color: var(--muted); margin-bottom: 0.2rem; }
.submission-form .form-field input, .submission-form .form-field select {
  padding: 0.5rem; border: 1px solid var(--border); background: var(--input-bg);
  color: var(--text); border-radius: 4px;
}
.consent-checkbox { display: flex; align-items: center; gap: 0.5rem; margin-top: 1rem; }
.consent-text { white-space: pre-wrap; background: var(--card); padding: 1rem; border-radius: 4px; max-height: 300px; overflow: auto; }
.form-error { color: var(--danger); margin: 0.5rem 0; }

/* Submissions table */
.submissions-page .toolbar { display: flex; justify-content: space-between; align-items: center; }
.submissions-page .filters { display: flex; gap: 1rem; margin: 1rem 0; }
.submissions-table { width: 100%; border-collapse: collapse; }
.submissions-table tbody tr { cursor: pointer; }
.submissions-table tbody tr:hover { background: var(--hover); }
.submissions-table th, .submissions-table td { padding: 0.5rem; border-bottom: 1px solid var(--border); text-align: left; }

/* Public form layout */
.public-form { max-width: 720px; margin: 2rem auto; padding: 1rem; }
.public-form .lead { color: var(--muted); }
.thanks-page { text-align: center; margin-top: 4rem; }
.consent-page { max-width: 720px; margin: 2rem auto; padding: 1rem; }
.consent-page .version { color: var(--muted); }

/* Flight card */
.flight-card { border: 1px solid var(--border); padding: 0.75rem; border-radius: 6px; margin-top: 0.5rem; }
.flight-card-header { display: flex; justify-content: space-between; margin-bottom: 0.5rem; }
.flight-leg-view { font-size: 0.9rem; }
.flight-leg { display: grid; grid-template-columns: repeat(2, 1fr); gap: 0.5rem; }

/* Submission list (modal) */
.submission-list { list-style: none; padding: 0; max-height: 400px; overflow: auto; }
.submission-list li { display: grid; grid-template-columns: 1fr 1fr auto; gap: 1rem; align-items: center; padding: 0.5rem; border-bottom: 1px solid var(--border); }
.submission-list .empty { grid-column: 1 / -1; color: var(--muted); text-align: center; }

@media (max-width: 600px) {
  .flight-leg { grid-template-columns: 1fr; }
  .submission-list li { grid-template-columns: 1fr; }
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/index.css
git commit -m "style(frontend): submissions + flight-card + public form CSS"
```

---

## Phase J — Deployment / Docs / QA

### Task 46: Drop Google env vars from deployment config

**Files:**
- Modify: `docker-compose.prod.yml`
- Modify: `.env.example` (if present; create if not)

- [ ] **Step 1: Remove GOOGLE_* references**

In `docker-compose.prod.yml`, remove the `GOOGLE_CREDENTIALS_PATH` and `GOOGLE_SHEET_ID` env keys from the backend service.

In `.env.example`, remove the same keys.

Add a new section to `.env.example`:

```env
# Required
DATABASE_URL=postgres://fuji:fuji123@localhost:5435/fujitravel?sslmode=disable
ANTHROPIC_API_KEY=sk-ant-...

# Optional
UPLOADS_DIR=./uploads
PORT=8080
DOCGEN_SCRIPT=../../docgen/generate.py
DOCGEN_PDF_TEMPLATE=./templates/anketa_template.pdf
```

- [ ] **Step 2: Commit**

```bash
git add docker-compose.prod.yml .env.example
git commit -m "chore(deploy): drop Google Sheets env vars"
```

---

### Task 47: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Replace the Data Flow and Environment Variables sections**

Open `CLAUDE.md`. Update:
- **Environment Variables:** remove `GOOGLE_CREDENTIALS_PATH` and `GOOGLE_SHEET_ID`.
- **Data Flow:** replace with the new flow (form submission → pool → attach → optional ticket/voucher scan → generate).
- **Stack:** change the AI line to mention split pipeline (translate.go Haiku, programme.go Opus 4.7, parsers Opus 4.6).

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for form-workflow"
```

---

### Task 48: Full-stack manual QA

**Files:**
- No code changes.

- [ ] **Step 1: Start backend + frontend**

Two terminals:
```bash
# terminal 1
docker compose up -d db
cd backend && go run cmd/server/main.go

# terminal 2
cd frontend && npm run dev
```

- [ ] **Step 2: Submit form from public URL**

Open `http://localhost:5173/form`. Fill all required fields with sample data. Submit. Expect redirect to `/form/thanks`.

- [ ] **Step 3: Verify submission in admin list**

Open `http://localhost:5173/submissions`. Expect the just-submitted row.

- [ ] **Step 4: Create a group and attach the submission**

Open `/groups`, create a new group. Inside the group, click "+ Добавить туриста", pick the submission from the list, attach.

- [ ] **Step 5: Enter flight data manually**

On the tourist card, click "Изменить" on the flight section and enter arrival + departure data.

- [ ] **Step 6: Add a hotel manually**

Add one hotel with check-in/check-out dates.

- [ ] **Step 7: Generate documents**

Click "Сгенерировать документы туристов". Wait for zip. Open `.docx` and verify:
- ФИО latin matches form
- Program has correct dates
- Passport fields fill in anketa

- [ ] **Step 8: Commit nothing — just verify**

No commit. If bugs found, fix and commit separately with `fix:` prefix.

---

## Post-Plan Self-Check

- Spec section 3 (architecture) → implemented across Phases D / E / F
- Spec section 4 (DB) → Tasks 1-2
- Spec section 5 (API) → Tasks 17-28
- Spec section 6 (frontend) → Tasks 34-45
- Spec section 7 (AI pipeline) → Tasks 12-16
- Spec section 8 (consent) → Tasks 9, 25, 36
- Spec section 9-10 (errors/edge cases) → embedded across tasks
- Spec section 11 (testing) → unit tests in Tasks 3-8, 12-16; manual QA in Task 48
- Spec section 12 (deployment) → Tasks 1-2, 46
- Spec section 13 (file summary) → whole plan
- Spec section 14 (open questions) → deferred per spec (no task)

**Done.**
