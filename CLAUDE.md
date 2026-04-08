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
│   │   ├── api/        — HTTP handlers
│   │   ├── sheets/     — Google Sheets client
│   │   ├── ai/         — Claude API (Pass 1 + Pass 2)
│   │   ├── matcher/    — fuzzy matching
│   │   ├── docgen/     — document generation (calls Python)
│   │   └── storage/    — file uploads
│   ├── migrations/     — golang-migrate SQL files
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── pages/
│   │   ├── components/
│   │   └── api/
│   └── package.json
├── uploads/            — uploaded client files (gitignored)
├── docker-compose.yml
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
| AI | Anthropic Claude API (claude-opus-4-6) |
| Sheets | Google Sheets API v4 |
| Doc generation | python-docx + fillpdf (Python subprocess) |

---

## Environment Variables

```env
DATABASE_URL=postgres://fuji:fuji123@localhost:5435/fujitravel?sslmode=disable
ANTHROPIC_API_KEY=...
GOOGLE_CREDENTIALS_PATH=/Users/yaufdd/Desktop/FUJIT TRAVEL/google-credentials.json
GOOGLE_SHEET_ID=1UH-MK_KsTPbghaAKrx6ous8jFACeL6uEQccsewLcrnM
UPLOADS_DIR=./uploads
PORT=8080
DOCGEN_SCRIPT=../../docgen/generate.py
```

> PostgreSQL runs on port **5435** (non-standard, 5432/5433 were occupied locally).

---

## Agents

| Agent | Role | Files it touches |
|---|---|---|
| db-agent | Schema + migrations + seed | migrations/*.sql |
| backend-agent | Go API + business logic | backend/ |
| frontend-agent | React UI | frontend/ |
| prompt-engineer | Claude API prompts | backend/internal/ai/ |
| code-reviewer | Review only, no edits | reads everything |

**Agents must not edit files outside their zone.**

---

## Data Flow

```
1. Create group
2. Add tourists from Google Sheets (select by name)
3. Per tourist: upload files (passport, ticket, voucher) → click Parse
   - Pass 1: Claude extracts structured data from documents (all fields in English)
   - Hotels auto-created from voucher if not in DB yet
4. Hotels auto-populated from vouchers into group_hotels (or add manually)
5. Click "Сгенерировать документы туристов":
   - Pass 2: Claude merges Pass 1 data + Sheets row → final JSON (pass2.json saved)
   - Python generates per-tourist docs: программа.docx, доверенность.docx, анкета.pdf
   - Download output.zip
6. After whole group is processed → click "Сформировать финальные документы":
   - Python generates group-level docs: для Инны в ВЦ.docx, заявка ВЦ.docx
   - Download final.zip
```

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

```bash
# Start DB
docker compose up -d db

# Run migrations
migrate -path backend/migrations -database $DATABASE_URL up

# Start backend
cd backend && go run cmd/server/main.go

# Start frontend
cd frontend && npm run dev
```
