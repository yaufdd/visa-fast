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
│   │   ├── api/        — HTTP handlers (groups, subgroups, hotels, submissions, tourists, flight_data, generate, uploads)
│   │   ├── ai/         — Claude API (translate.go, programme.go, assembler.go, ticket_parser.go, voucher_parser.go)
│   │   ├── consent/    — consent / form-submission helpers
│   │   ├── docgen/     — document generation (calls Python)
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

---

## Environment Variables

```env
# Local dev
DATABASE_URL=postgres://fuji:fuji123@localhost:5435/fujitravel?sslmode=disable
ANTHROPIC_API_KEY=...
UPLOADS_DIR=./uploads
PORT=8080
DOCGEN_SCRIPT=../../docgen/generate.py
DOCGEN_PDF_TEMPLATE=./templates/anketa_template.pdf
```

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

## Data Flow

```
1. Tourist (or manager) submits form at /form → POST /api/submissions → tourist_submissions row.
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
Passport data, full names, and full addresses never reach the AI. The AI only sees:
- anonymised free-text fragments (mini-translate via `translate.go`)
- dates, hotels, and flights (programme generation via `programme.go`)

Scan parsers (`ticket_parser.go`, `voucher_parser.go`) receive only the uploaded
image/PDF of the ticket/voucher — no linked tourist PII.

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

Required env vars: `DATABASE_URL`, `ANTHROPIC_API_KEY`. Optional: `UPLOADS_DIR`,
`PORT`, `DOCGEN_SCRIPT`, `DOCGEN_PDF_TEMPLATE`. No Google credentials are needed.

```bash
# Start DB
docker compose up -d db

# Run migrations
migrate -path backend/migrations -database $DATABASE_URL up

# Start backend
cd backend && go run cmd/server/main.go

# Start frontend (Vite HMR — no rebuild needed for .jsx/.css changes)
cd frontend && npm run dev
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
