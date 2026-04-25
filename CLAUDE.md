# FujiTravel Admin — Developer Instructions

## Business Rules
All document generation rules (formats, templates, hotel database, prompts) are in:
`/Users/yaufdd/Desktop/FUJIT TRAVEL/CLAUDE.md`

Agents must read this file before working on anything related to document generation or AI prompts.

---

## Project Structure

```
fujitravel-admin/
├── backend/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── api/        — HTTP handlers (auth, public slug, groups, subgroups, hotels, submissions, tourists, flight_data, generate, uploads)
│   │   ├── auth/       — argon2id password hashing, session tokens, org slug generation
│   │   ├── ai/         — Claude API (ticket_parser.go, voucher_parser.go) + YandexGPT seam (translate.go, programme.go, doverenost_clean.go, assembler.go)
│   │   ├── consent/    — consent / form-submission helpers
│   │   ├── db/         — tenant-aware repository functions (every query takes orgID)
│   │   ├── docgen/     — document generation (calls Python)
│   │   ├── middleware/ — requireAuth + in-memory rate limiter
│   │   ├── server/     — chi router builder (shared by main and integration tests)
│   │   ├── storage/    — file uploads
│   │   └── translit/   — Cyrillic transliteration helpers
│   ├── migrations/     — golang-migrate SQL files
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── pages/      — GroupsPage, GroupDetailPage, HotelsPage, HotelEditPage
│   │   ├── components/
│   │   └── api/client.js
│   └── package.json
├── docgen/generate.py  — python-docx + fillpdf generator
├── templates/          — Word + PDF templates (committed)
│   ├── ШАБЛОН программа.docx
│   ├── ШАБЛОН доверенность.docx
│   ├── ШАБЛОН для Инны в ВЦ.docx
│   ├── ШАБЛОН заявка ВЦ.docx
│   └── anketa_template.pdf   (whitelisted in .gitignore)
├── uploads/            — uploaded client files (gitignored)
├── Dockerfile.backend  — Go + Python + templates
├── Dockerfile.frontend — Vite build, served via nginx
├── nginx.conf
├── docker-compose.prod.yml
├── .github/workflows/deploy.yml  — SSH deploy on push to main
└── .claude/agents/
```

---

## Stack

| Layer | Technology |
|---|---|
| Backend | Go, chi router, pgx |
| Database | PostgreSQL 16 (Docker) |
| Migrations | golang-migrate |
| Frontend | React + Vite |
| AI | Anthropic Claude API — Haiku 4.5 for `translate.go`, Opus 4.7 for `programme.go`, Opus 4.6 for scan parsers (`ticket_parser.go`, `voucher_parser.go`) |
| Doc generation | python-docx + fillpdf (Python subprocess) |
| Auth | session cookie + argon2id passwords, row-level multi-tenancy via `org_id` |

---

## Environment Variables

```env
# Required
DATABASE_URL=postgres://fuji:fuji123@localhost:5435/fujitravel?sslmode=disable
ANTHROPIC_API_KEY=...
APP_SECRET=<32-byte-base64 from `openssl rand -base64 32`>

# Optional
APP_ENV=production      # dev|production — controls Secure cookie flag
UPLOADS_DIR=./uploads
PORT=8080
DOCGEN_SCRIPT=../../docgen/generate.py
DOCGEN_PDF_TEMPLATE=./templates/anketa_template.pdf
REDACT_SCAN_SCRIPT=../../docgen/redact_scan.py  # Python OCR redactor for ticket/voucher scans
AI_LOG_RETENTION_DAYS=30  # ai_call_logs auto-cleanup; 0 disables

# Yandex Cloud — required for the Yandex AI features wired in later migration tasks
# (Phase 1 of docs/superpowers/plans/2026-04-25-russian-services-migration.md).
# Currently read but not required — backend boots without them and logs an info
# line that Yandex AI features are disabled. Will become required once Anthropic
# is removed in task 1.D1.
YANDEX_FOLDER_ID=b1gv...                 # Yandex Cloud folder id
YANDEX_SA_KEY_JSON={"id":"...","service_account_id":"...","private_key":"..."}  # full authorized_key.json contents
```

Backend **refuses to start** without `APP_SECRET` set.

> PostgreSQL runs on port **5435** locally (non-standard, 5432/5433 were occupied).
> Inside docker-compose.prod.yml the backend reaches it via the service name `db:5432`.

---

## Agents

| Agent | Role | Files it touches |
|---|---|---|
| db-agent | Schema + migrations + seed | `backend/migrations/*` |
| backend-agent | Go API + business logic + AI prompts | `backend/internal/ai/*`, `backend/internal/api/*` |
| frontend-agent | React UI | `frontend/*` |
| code-reviewer | Review only, no edits | reads everything |

**Agents must not edit files outside their zone.**

---

## SaaS Model

Every travel agency is an `organizations` row with a unique random 7-char
`slug`. Users (managers) belong to one org; sessions are scoped to that
user + org. Every owned entity (`groups`, `subgroups`, `tourists`,
`tourist_submissions`, `group_hotels`, `documents`, `uploads`) has an
`org_id NOT NULL`. Hotels are split: `org_id IS NULL` = global catalog
(seed data, read-only for all orgs), `org_id` set = private catalog of
that agency.

- Agencies self-register at `/register` (rate-limited: 5/15min/IP).
- Tourists fill the form at `/form/<slug>` — that slug resolves to an
  `org_id`; their submission lands only in that agency's pool.
- Every protected handler reads `middleware.OrgID(r.Context())` and
  delegates SQL to `backend/internal/db/*.go` functions that take
  `orgID` as a mandatory parameter (compiler-enforced — cannot be
  forgotten).
- Cross-org access returns `404 not found` (never `403`) to prevent
  ID enumeration.

## Data Flow

```
0. Agency registers at /register or signs in at /login.
1. Tourist (or manager) submits form at /form/<slug> → POST /api/public/submissions/<slug> → tourist_submissions row with org_id.
2. Manager creates a group, picks submissions from the pool via AddFromDBModal
   → POST /api/submissions/:id/attach → tourists row with submission_snapshot.
3. Manager optionally:
   - Uploads ticket scan → auto-parsed by ticket_parser.go → tourists.flight_data.
   - Uploads voucher scan → auto-parsed by voucher_parser.go → group_hotels rows.
   - Or enters flight data manually via FlightDataForm (PUT /api/tourists/:id/flight_data).
4. Manager clicks "Сгенерировать документы":
   - Backend translates free-text fields in one batched Haiku call (translate.go)
   - Opus generates the programme from dates + hotels + flights (programme.go)
   - Go code (assembler.go) composes the final pass2.json deterministically
   - Python docgen builds .docx and .pdf files
5. "Сформировать финальные документы" generates group-level docs.
```

### AI Privacy

FujiTravel is a Russian legal entity → 152-ФЗ compliance is mandatory. The
guiding principle: **PII must never leave the Russian Federation.** Two
provider tiers exist:

- **YandexGPT** (RU-resident processing, no cross-border transfer) — may
  receive PII when necessary for canonicalisation / programme building.
- **Anthropic Claude** (US-based) — receives only "dry" (non-identifying)
  fields, batched across multiple tourists so no single entry can be linked
  to an individual.

**What YandexGPT sees today:**

- `translate.go` — batch of Russian→English strings for non-PII fields:
  `place_of_birth_ru`, `issued_by_ru` (foreign passport authority),
  `occupation_ru`, `employer_ru`, `employer_address_ru`,
  `previous_visits_ru`, `nationality_ru`. Strings are de-duplicated across
  all tourists in the batch.
- `programme.go` — dates, flights, hotels, single contact phone, manager
  notes. No tourist names. No passport data.
- `doverenost_clean.go` — batch of Russian free-text fields requiring
  canonical formatting (lower/upper-casing, dotted abbreviations, commas):
  `home_address_ru`, `reg_address_ru`, `internal_issued_by_ru`. These are
  PII; routing them through YandexGPT is permitted under 152-ФЗ because
  there is no cross-border transfer. Output stays in Russian. Dedup is
  identical to translate (`collectDoverenostFreeText` in `api/generate.go`).

**What Claude (Anthropic) sees today:**

- `ticket_parser.go` / `voucher_parser.go` — redacted (passenger-name
  masked) ticket and voucher scans. See "Ticket / voucher scan
  redaction" below.

**What no AI sees at all (formatted locally on the server):**

- Full name (`name_cyr`, `name_lat`, `maiden_name_ru`) — used for the
  доверенность Russian text and the anketa via `translit.RuToLatICAO`.
- Passport numbers (internal series/number, foreign passport number).
- Date of birth.
- Phone numbers (other than the single guide contact phone in programme).

The `HomeAddress` field on the anketa PDF is the YandexGPT-cleaned
`home_address_ru` followed by deterministic ICAO transliteration
(`translit.RuToLatICAO`). The transliteration step stays local because it
is purely deterministic.

**Ticket / voucher scan redaction:** `UploadTouristFile` calls
`backend/internal/privacy.RedactScan` which shells to
`docgen/redact_scan.py` — local Tesseract OCR finds name labels
("Passenger", "Пассажир", "Имя пассажира", "Гость", "ФИО", ...) and black-boxes
the label + the next several tokens on that text line via OpenCV. The
original scan stays on disk for the manager's reference, but only the
redacted copy is uploaded to Anthropic and used for `ticket_parser` /
`voucher_parser`. Multi-page PDFs are processed page-by-page; the output
is a multi-page PDF when the input was. **Fail-loud policy**: if any page
has no detectable name label, redaction fails and the HTTP response
returns 422 with `redact_error` — we refuse to send a partially-redacted
scan to AI.

Configured via `REDACT_SCAN_SCRIPT` env var (default
`../../docgen/redact_scan.py`).

Passport scans (`docgen/passport_parser.py`, planned — see
`docs/superpowers/plans/2026-04-22-passport-scan-parser.md`) are parsed
fully locally and never reach Claude.

### AI Audit Log

Every call to Anthropic is persisted to the `ai_call_logs` table as an
observational record (request JSON with image bytes redacted, response text,
status, duration, model). Rows are scoped to `org_id` + `group_id` +
`generation_id` (one UUID per `/generate`, `/finalize`, or scan-upload run).
The log is wired at the `callClaude` seam in `backend/internal/ai/client.go`
so no high-level function can forget to log — `ai.WithLogger(ctx, ...)` +
`ai.WithGenerationID(ctx, ...)` at the handler entry are enough.

Managers can inspect the log via the "Аудит-лог ИИ-вызовов" expandable
section on the Documents tab of each group (UI component
`frontend/src/components/AILogsSection.jsx`). `GET /api/groups/{id}/ai_logs`
returns up to the latest 500 rows, newest first, grouped by `generation_id`.

**Retention:** a background goroutine in `cmd/server/main.go` deletes
`ai_call_logs` rows older than `AI_LOG_RETENTION_DAYS` (default `30`) once
a day. Set `AI_LOG_RETENTION_DAYS=0` to disable auto-cleanup; any positive
integer overrides the default.

### Hotels CRUD
- `/hotels` — list all hotels (city tags normalized to Title Case + RU translation for known Japanese cities).
- Row click → `/hotels/:id` (HotelEditPage) — dedicated edit page, PUT /api/hotels/:id.
- Create via inline form at top of `/hotels`.

---

## Key Business Rules (summary — full rules in FUJIT TRAVEL/CLAUDE.md)

- Programme date format: `YYYY-DD-MM` (non-standard, intentional)
- Contact column: first row = tourist phone from Sheets["Телефон"], rest = "Same as above"
- Accommodation: first row of each hotel = full details, same hotel = "Same as above"
- Transfer day: show NEW hotel (not the checked-out one)
- Arrival/departure days: logistics only, no sightseeing
- Sightseeing: 3–4 places per day, geographically realistic, no repeats
- Cell content: `\n` → soft line break (shift+enter) in Word

## Nationality Rules (for AI prompts)
- nationality: full English name ("RUSSIA", not "RUS")
- former_nationality:
  - Stated as USSR → "USSR"
  - Not stated but place_of_birth contains "USSR" → "USSR"
  - Not stated, place_of_birth has no USSR → "NO"

## All Fields Must Be in English
Pass 2 translates ALL fields to English. Only these may stay in Russian Cyrillic:
- name_cyr, doverenost fields, inna_doc applicants, vc_request applicants

## Document Generation Split
- **tourists mode** (POST /api/groups/:id/generate): программа, доверенность, анкета — per tourist → output.zip
- **final mode** (POST /api/groups/:id/finalize): для Инны в ВЦ, заявка ВЦ — group level → final.zip
- письмо в ВЦ.txt — removed, not generated

## заявка ВЦ — Service Fee in Words
`docgen/generate.py` has `_AMOUNT_WORDS` dict mapping 1–20 tourists → Russian words for 970×N rubles.
No AI needed — hardcoded lookup table.

## Hotels Auto-Creation
If a hotel from a voucher is not found in the `hotels` DB table, it is automatically created
with the name from the voucher (city left empty). Manager can edit later.

---

## Running locally

For a fully containerised build:

```bash
docker compose -f docker-compose.prod.yml build
docker compose -f docker-compose.prod.yml up -d
```

---

## Deployment

Production runs on a single VM via `docker-compose.prod.yml`:

- `backend` — Go API (Dockerfile.backend, includes Python + templates + anketa_template.pdf)
- `frontend` — Vite build served by nginx (Dockerfile.frontend + nginx.conf)
- `db` — PostgreSQL 16 with a named volume for persistence

### CI/CD
`.github/workflows/deploy.yml` triggers on push to `main`:
1. SSH into server (secrets: `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_KEY`)
2. `cd /opt/visa-fast && git pull origin main`
3. `docker compose -f docker-compose.prod.yml build`
4. `docker compose -f docker-compose.prod.yml up -d`
5. `docker image prune -f` (clean up orphaned <none> images)

Migrations are **not** auto-run by the workflow — run manually on the server
after pulling if a new `backend/migrations/*.sql` was added.

### Data NOT in git
After `git clone` on a fresh server, these must be placed manually:
- `.env` (prod secrets)

The anketa PDF template **is** whitelisted in `.gitignore`
(`!templates/anketa_template.pdf`), so docgen works out of the box.
