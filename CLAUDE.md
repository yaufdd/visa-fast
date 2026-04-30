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
│   │   ├── ai/         — Yandex Cloud seam (translate.go, programme.go, doverenost_clean.go, ticket_parser.go, voucher_parser.go, passport_parser.go, assembler.go) + adapters (yandex_adapter.go, yandex_ocr_adapter.go) + audit log (logger.go)
│   │   ├── ai/yandex/  — low-level Yandex Cloud HTTP clients (GPT, Vision OCR, IAM token source)
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
| AI | Yandex Cloud — YandexGPT 5 Pro for text (`translate.go`, `programme.go`, `doverenost_clean.go`, GPT half of the scan parsers), Yandex Vision OCR for scan recognition (`ticket_parser.go`, `voucher_parser.go`, `passport_parser.go`) |
| Doc generation | python-docx + fillpdf (Python subprocess) |
| Auth | session cookie + argon2id passwords, row-level multi-tenancy via `org_id` |

---

## Environment Variables

```env
# Required
DATABASE_URL=postgres://fuji:fuji123@localhost:5435/fujitravel?sslmode=disable
APP_SECRET=<32-byte-base64 from `openssl rand -base64 32`>
YANDEX_FOLDER_ID=b1gv...                 # Yandex Cloud folder id
YANDEX_SA_KEY_JSON='{"id":"...","service_account_id":"...","private_key":"..."}'
# ↑ Full authorized_key.json contents on a single line. WRAP IN SINGLE QUOTES
#   — the value contains double quotes and `\n` escapes from the PEM private
#   key, and shell / .env parsers will mangle it without single-quote
#   wrapping.

# Optional
APP_ENV=production      # dev|production — controls Secure cookie flag
UPLOADS_DIR=./uploads
PORT=8080
DOCGEN_SCRIPT=../../docgen/generate.py
DOCGEN_PDF_TEMPLATE=./templates/anketa_template.pdf
AI_LOG_RETENTION_DAYS=30  # ai_call_logs auto-cleanup; 0 disables
YANDEX_CAPTCHA_SECRET=     # Yandex SmartCaptcha server key. When set, the public form requires a valid SmartCaptcha token before accepting submissions; when empty, captcha verification is skipped (local dev convenience).
VITE_YANDEX_CAPTCHA_SITE_KEY=  # Frontend build-time arg (Vite). The public site key embedded in the SmartCaptcha widget; paired with YANDEX_CAPTCHA_SECRET on the backend. Wired through Dockerfile.frontend by the frontend agent.
```

Backend **refuses to start** without `APP_SECRET`, `YANDEX_FOLDER_ID`, or
`YANDEX_SA_KEY_JSON` set — every AI call now goes through Yandex Cloud, so
missing credentials would yield a 500 at the first `/generate`. Failing
fast at boot makes that misconfiguration loud.

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
   - Backend translates free-text fields in one batched YandexGPT call (translate.go)
   - YandexGPT generates the programme from dates + hotels + flights (programme.go)
   - YandexGPT canonicalises доверенность address fields (doverenost_clean.go)
   - Go code (assembler.go) composes the final pass2.json deterministically
   - Python docgen builds .docx and .pdf files
5. "Сформировать финальные документы" generates group-level docs.
```

### AI Privacy

FujiTravel is a Russian legal entity → 152-ФЗ compliance is mandatory. The
guiding principle: **PII must never leave the Russian Federation.**

Every AI call now flows through **Yandex Cloud** — RU-resident processing,
no cross-border transfer. Because there is no foreign provider in the
pipeline, the 152-ФЗ trans-border restriction does not apply: PII (names,
passports, addresses) MAY now flow through Yandex services.

**What YandexGPT (`yandexgpt/latest`) sees:**

- `translate.go` — batch of Russian→English strings for non-PII fields:
  `place_of_birth_ru`, `issued_by_ru` (foreign passport authority),
  `occupation_ru`, `employer_ru`, `employer_address_ru`,
  `previous_visits_ru`, `nationality_ru`. Strings are de-duplicated across
  all tourists in the batch.
- `programme.go` — dates, flights, hotels, single contact phone, manager
  notes. (No tourist names / passport data are sent — those are not
  needed for the programme content even though they would be permitted.)
- `doverenost_clean.go` — batch of Russian free-text PII fields requiring
  canonical formatting (lower/upper-casing, dotted abbreviations, commas):
  `home_address_ru`, `reg_address_ru`, `internal_issued_by_ru`. Output
  stays in Russian. Dedup is identical to translate
  (`collectDoverenostFreeText` in `api/generate.go`).
- `ticket_parser.go` — text recognized by Vision OCR from a ticket scan,
  asked to extract structured flight JSON.
- `voucher_parser.go` — text recognized by Vision OCR from a hotel
  voucher scan, asked to extract a structured array of hotels.
- `passport_parser.go` — text recognized by Vision OCR from a passport
  scan, asked to extract MRZ-derived fields.

**What Yandex Vision OCR sees:**

- Raw PDF / JPEG / PNG bytes of ticket, voucher, and passport scans.
  Names and passport numbers reach the OCR pass — this is acceptable
  because Vision OCR is a Yandex Cloud service running inside Russia.

**What no AI sees at all (formatted locally on the server):**

- Phone numbers (other than the single guide contact phone in programme).
- The deterministic ICAO transliteration of addresses for the anketa PDF
  (`HomeAddress` = YandexGPT-cleaned `home_address_ru` →
  `translit.RuToLatICAO`). The transliteration step stays local because
  it is purely deterministic and there is no need to involve a service.

### AI Audit Log

Every AI provider call is persisted to the `ai_call_logs` table as an
observational record (request JSON, response text, status, duration,
model, **provider** — `yandex-gpt` or `yandex-vision`). Rows are scoped
to `org_id` + `group_id` + `generation_id` (one UUID per `/generate`,
`/finalize`, or scan-upload run). The log is wired at the per-provider
HTTP seams in `backend/internal/ai/yandex_adapter.go` and
`backend/internal/ai/yandex_ocr_adapter.go` so no high-level function can
forget to log — `ai.WithLogger(ctx, ...)` + `ai.WithGenerationID(ctx, ...)`
at the handler entry are enough.

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
