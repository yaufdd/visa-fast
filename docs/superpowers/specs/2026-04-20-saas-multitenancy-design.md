# SaaS Multi-Tenancy (Tier 1) — Design Spec

**Date:** 2026-04-20
**Status:** Draft (awaiting user review)
**Author:** Brainstormed together with Claude
**Related git branch:** `feature/saas-multitenancy` (to be created at
  implementation time, branched off `feature/custom-form-workflow`).
**Related specs:** `2026-04-20-custom-form-workflow-design.md`

---

## 1. Motivation

Current FujiTravel admin is single-tenant: no authentication, no user accounts,
no data isolation. The site is deployed to the internet but the URL has not
yet been shared with anyone. The product vision is a **SaaS platform for
travel agencies**: every agency gets its own account and sees only its own
groups, tourists, submissions, and documents.

This spec describes **Tier 1** of that transformation:

- Travel agencies self-register and log in.
- Every agency's data is isolated from every other agency's data.
- The public tourist form becomes agency-specific via a short slug in the URL.
- Hotel catalog is split into a shared (global) catalog plus each agency's
  private list.

Later tiers (deferred, not in this spec):

- Tier 2: invite additional users to an agency, per-user audit log, roles
  (owner / manager / viewer).
- Tier 3: billing, SSO, 2FA.

## 2. Out-of-Scope

- Email delivery (no password reset email, no verification email).
- Password recovery self-service. Recovery is a manual CLI step.
- Multiple users per organization (deferred to Tier 2).
- Role-based access control beyond a single `owner` role.
- Captcha on the registration form (IP rate-limit only for MVP).
- Postgres Row-Level Security (application-level scoping is the MVP approach).
- Schema-per-tenant or database-per-tenant approaches.

## 3. High-Level Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                   PUBLIC ROUTES (no auth)                        │
│                                                                  │
│   /register                     → create org + first user        │
│   /login                        → credentials                    │
│   /form/<org-slug>              → tourist form (agency-specific) │
│   /consent                      → agreement text                 │
│   POST /api/auth/register       → create org                     │
│   POST /api/auth/login          → set session cookie             │
│   POST /api/public/submissions/<slug> → accept tourist payload   │
│   GET  /api/public/org/<slug>   → org name for form header       │
│   GET  /api/consent/text                                         │
└──────────────────────────────────────────────────────────────────┘
                                │
                                │ (session cookie set)
                                ▼
┌──────────────────────────────────────────────────────────────────┐
│         PROTECTED ROUTES (session cookie + org scope)            │
│                                                                  │
│   /groups, /submissions, /hotels, /submissions/:id, ...          │
│   All existing admin endpoints                                   │
│                                                                  │
│   Middleware:                                                    │
│     requireAuth: cookie → SELECT sessions+users → ctx.user_id,   │
│                  ctx.org_id                                      │
│                                                                  │
│   Handlers: every SQL adds `WHERE org_id = ctx.org_id`           │
└──────────────────────────────────────────────────────────────────┘
```

### 3.1 Key Principles

- **Two route layers:** public (no auth) and protected (session + org). The
  public set is a short, explicit whitelist. Anything not in the list requires
  authentication.
- **Row-level tenancy, application-enforced:** middleware places `org_id` in
  `context.Context`; handlers must read it and filter SQL. A repository
  pattern (dedicated `internal/db/*.go` files) makes `orgID` a mandatory
  parameter so it cannot be forgotten.
- **Shared + private hotel catalog:** `hotels` gets a nullable `org_id`.
  `NULL` = global (visible to all), non-null = private to that agency.
- **Agency-specific public form via slug:** `/form/<slug>` resolves at the
  backend to an org_id; the submission lands in that agency's pool.
- **Session cookies in Postgres:** `sessions` table keyed by opaque random
  token. Cookie is `HttpOnly`, `SameSite=Lax`, `Secure` in production.
  Session TTL is 30 days with sliding renewal.

### 3.2 Unchanged Components

- Document generation pipeline (translate.go / programme.go / assembler.go /
  Python docgen) — zero changes.
- Word / PDF templates — zero changes.
- Consent text mechanism — zero changes.
- Deployment topology (Docker Compose with nginx + Go + Postgres).

## 4. Database Schema

### 4.1 New Tables

```sql
CREATE TABLE organizations (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL,
  slug        TEXT UNIQUE NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  email         TEXT NOT NULL UNIQUE,      -- globally unique
  password_hash TEXT NOT NULL,             -- argon2id encoded "salt$hash"
  display_name  TEXT,
  role          TEXT NOT NULL DEFAULT 'owner',
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  last_login_at TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_users_org_id ON users(org_id);

CREATE TABLE sessions (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token        TEXT UNIQUE NOT NULL,        -- 32 random bytes base64
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at   TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sessions_user_id    ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
```

### 4.2 Modified Existing Tables

All owned tables gain `org_id UUID NOT NULL REFERENCES organizations(id) ON
DELETE CASCADE`:

- `groups`
- `subgroups`
- `tourists`
- `tourist_submissions`
- `group_hotels`
- `documents`
- `uploads`

Each gets `CREATE INDEX idx_<table>_org_id ON <table>(org_id)`.

`hotels` is special — nullable:

```sql
ALTER TABLE hotels ADD COLUMN org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;
CREATE INDEX idx_hotels_org_id ON hotels(org_id);
```

- `org_id IS NULL` → global catalog (visible to all agencies, read-only).
- `org_id` set → private hotel (owned by that agency, editable by them).

### 4.3 Sample Query Patterns

Manager listing hotels:

```sql
SELECT id, name_en, city, address, phone, (org_id IS NOT NULL) AS is_private
  FROM hotels
 WHERE org_id IS NULL OR org_id = :my_org
 ORDER BY (org_id IS NOT NULL) DESC, name_en;   -- own hotels first, then global
```

Manager editing a private hotel:

```sql
UPDATE hotels SET name_en = :n, city = :c, ...
 WHERE id = :id AND org_id = :my_org;   -- cannot touch global rows
```

## 5. Auth Flow

### 5.1 Registration — `POST /api/auth/register`

Request body:

```json
{
  "org_name": "FujiTravel",
  "email": "manager@fujitravel.ru",
  "password": "super-secret-2026"
}
```

Server transaction:

1. Validate email format; check `users.email` not taken.
2. Validate `password.length >= 8`.
3. Generate a unique 7-character base62 slug (retry on unique-violation).
4. Hash password with argon2id (time=1, memory=64MB, threads=4).
5. `INSERT organizations (name, slug)`.
6. `INSERT users (org_id, email, password_hash, role='owner')`.
7. `INSERT sessions (user_id, token, expires_at = NOW() + 30 days)`.
8. Set response cookie `session=<token>; HttpOnly; SameSite=Lax; Secure;
   Max-Age=2592000; Path=/`.
9. `201 Created` with body `{ "user": {...}, "org": {...} }`.

Rate limit: 5 registrations per IP per 15 minutes → `429`.

### 5.2 Login — `POST /api/auth/login`

Request body:

```json
{ "email": "...", "password": "..." }
```

Server flow:

1. `SELECT id, password_hash, is_active FROM users WHERE email = $1`.
2. If no row, or password mismatch, or `is_active = FALSE` → `401 invalid
   credentials` (same text for all three cases — do not leak account state).
3. `INSERT sessions (user_id, token, expires_at = NOW() + 30 days)`.
4. `UPDATE users SET last_login_at = NOW()`.
5. Set session cookie, return `{ "user", "org" }`.

Rate limit: 10 login attempts per IP per 15 minutes → `429`.

### 5.3 Session Middleware — `requireAuth`

On every protected request:

```go
cookie, err := r.Cookie("session")
if err != nil { → 401 }

var userID, orgID string
var expires time.Time
err = pool.QueryRow(ctx,
    `SELECT s.user_id, u.org_id, s.expires_at
       FROM sessions s
       JOIN users u ON u.id = s.user_id
      WHERE s.token = $1 AND s.expires_at > NOW() AND u.is_active = true`,
    cookie.Value,
).Scan(&userID, &orgID, &expires)
if err == pgx.ErrNoRows { → 401 "session expired or invalid" }

// Sliding TTL: if under 15 days remain, extend to 30 days
if time.Until(expires) < 15*24*time.Hour {
    pool.Exec(ctx, `UPDATE sessions SET expires_at = NOW() + interval '30 days',
                                         last_seen_at = NOW()
                    WHERE token = $1`, cookie.Value)
} else {
    pool.Exec(ctx, `UPDATE sessions SET last_seen_at = NOW() WHERE token = $1`,
              cookie.Value)
}

ctx = context.WithValue(ctx, ctxKeyUserID, userID)
ctx = context.WithValue(ctx, ctxKeyOrgID, orgID)
```

### 5.4 Logout — `POST /api/auth/logout`

```
DELETE FROM sessions WHERE token = <cookie>;
Set-Cookie: session=; Max-Age=0
```

### 5.5 Current User — `GET /api/auth/me`

Returns `{ "user": {...}, "org": {...} }` for the currently authenticated
user. Used by the frontend on app load to determine whether to show the
login screen or the admin shell.

### 5.6 Password Hashing (argon2id)

```go
salt := crypto/rand.Read(16)
hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
stored := "$argon2id$v=19$m=65536,t=1,p=4$" +
          base64(salt) + "$" + base64(hash)
```

Verification parses parameters from the stored string and recomputes.

## 6. Middleware and Scoping

### 6.1 Router Structure

```go
r.Route("/api", func(r chi.Router) {
    // Public
    r.Post("/auth/register", handlers.Register(pool))
    r.Post("/auth/login",    handlers.Login(pool))
    r.Post("/public/submissions/{slug}", handlers.PublicSubmit(pool))
    r.Get("/public/org/{slug}",          handlers.PublicOrg(pool))
    r.Get("/consent/text",               handlers.GetConsentText())

    // Protected
    r.Group(func(r chi.Router) {
        r.Use(middleware.RequireAuth(pool))

        r.Post("/auth/logout", handlers.Logout(pool))
        r.Get("/auth/me",      handlers.Me())

        // All existing admin endpoints here (groups, submissions, tourists,
        // hotels, subgroups, documents, uploads, flight_data, generate).
    })
})
```

### 6.2 Context Helpers

```go
type ctxKey int
const (
    ctxKeyUserID ctxKey = iota
    ctxKeyOrgID
)

func OrgID(ctx context.Context) string {
    v, ok := ctx.Value(ctxKeyOrgID).(string)
    if !ok {
        panic("OrgID called without requireAuth middleware")
    }
    return v
}
```

The `panic` is intentional — it converts a routing misconfiguration into a
loud failure during development rather than a silent data leak.

### 6.3 Repository Pattern

SQL for owned entities moves into `backend/internal/db/<entity>.go` where
`orgID` is a mandatory parameter. This prevents forgetting the scope.

Handler:

```go
func ListGroups(pool *pgxpool.Pool) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        orgID := middleware.OrgID(r.Context())
        groups, err := db.ListGroups(r.Context(), pool, orgID)
        if err != nil { writeError(w, 500, "db"); return }
        writeJSON(w, 200, groups)
    }
}
```

Repository:

```go
func ListGroups(ctx context.Context, pool *pgxpool.Pool, orgID string) ([]Group, error) {
    rows, err := pool.Query(ctx,
        `SELECT id, name, status FROM groups
          WHERE org_id = $1
          ORDER BY created_at DESC`, orgID)
    // ...
}
```

### 6.4 Cross-Org Protection: 404 (not 403)

On `SELECT|UPDATE|DELETE WHERE id = :id AND org_id = :my_org`, if zero rows
are affected → `404 not found`. Using `404` rather than `403` prevents
enumeration of existing IDs across orgs.

## 7. Public Form with Slug

### 7.1 URL Structure

```
https://<domain>/form/<slug>                        — page
POST   https://<domain>/api/public/submissions/<slug> — submission endpoint
GET    https://<domain>/api/public/org/<slug>         — org name lookup
```

### 7.2 Frontend Changes

`SubmissionFormPage` reads `slug` from `useParams()`:

1. `getOrgBySlug(slug)` → if 404, show "link invalid" message.
2. Otherwise render form with `For: <orgName>` header.
3. On submit → `publicCreateSubmission(slug, payload, consent)` → navigate to
   `/form/thanks`.

### 7.3 Backend Submission Endpoint

`POST /api/public/submissions/:slug` (public, no auth):

1. `SELECT id FROM organizations WHERE slug = $1` → 404 if not found.
2. Validate payload (same `requiredPayloadKeys` as before, `consent_accepted`
   must be true).
3. `INSERT tourist_submissions (org_id, payload, consent_accepted,
   consent_accepted_at, consent_version, source='tourist')`.
4. `201 Created` with `{ "id": "..." }`.

Rate limit: 3 submissions per IP per 10 minutes → `429`.

### 7.4 Admin-Side Copy-Link Button

AdminShell header (or SubmissionsListPage toolbar) shows a button
`📎 Ссылка на анкету`; on click it copies
`window.location.origin + "/form/" + org.slug` to the clipboard with a
transient "Copied" feedback.

### 7.5 Legacy `/form` (no slug)

Shows a small stub page: "Contact your travel agency for a personal link."

## 8. Error Handling and Security

### 8.1 Auth Errors

| Condition | Status | Body |
| --- | --- | --- |
| No session cookie | 401 | `{"error":"not authenticated"}` |
| Session not found or expired | 401 | `{"error":"session expired"}` |
| User is_active = false | 401 | `{"error":"session expired"}` |
| Wrong email or password at login | 401 | `{"error":"invalid credentials"}` |
| Email already registered | 409 | `{"error":"email already registered"}` |
| Weak password | 400 | `{"error":"password too short"}` |
| Rate limit hit | 429 | `{"error":"too many attempts","retry_after":...}` |

### 8.2 Cross-Org Access

All protected endpoints return `404 not found` for resources belonging to
another org, never `403 forbidden`. This prevents cross-org ID enumeration.

### 8.3 Public Form Errors

| Condition | Status |
| --- | --- |
| Slug does not exist | 404 |
| Duplicate submission (same passport + same day, status=pending) | 409 |
| Missing required fields | 400 with `{"missing":[...]}` |
| Consent not accepted | 400 |
| Rate limit | 429 |

### 8.4 Security Hardening Summary

- `HttpOnly` cookies (XSS cannot read session).
- `SameSite=Lax` (CSRF from other domains blocked).
- `Secure` cookie flag enforced in production (`APP_ENV=production`).
- New random session token at every login (no session fixation).
- argon2id password hashes (~200ms per login, resistant to GPU brute force).
- IP-based rate limits on register / login / public submission.
- Parameterised SQL everywhere; slugs are query parameters, not interpolated.
- CSP header added: `default-src 'self'; script-src 'self'; ...`.
- Permission denied → `404`, not `403` (ID enumeration protection).

### 8.5 Migration and Startup Safety

On backend start:

1. Connect to Postgres; retry 5× with exponential backoff, then `exit(1)`.
2. Apply migrations; on failure `exit(1)` — never serve requests against an
   inconsistent schema.
3. Require `APP_SECRET` env var (32+ bytes base64); `exit(1)` if missing.
4. Start HTTP server.

`GET /api/health` returns 200 when DB is reachable, 503 otherwise — for
container health checks.

## 9. Testing Strategy

### 9.1 Unit Tests (Go)

- `internal/auth/argon2_test.go` — correct password → true, wrong → false,
  re-hash of same password gives different hashes.
- `internal/auth/token_test.go` — base64 length ≥ 43 chars, 1000 unique in a
  row.
- `internal/auth/slug_test.go` — length 7, base62 alphabet, no collisions in
  10 000 with empty DB.
- `internal/db/*_test.go` — one test per repository verifying that
  `ListEntity(orgA)` returns only orgA's rows when orgB has rows too.

### 9.2 Integration Tests

Against a real test Postgres (testcontainers-go or a dedicated `:5499`
container in CI):

- Register two orgs.
- In session of org1, create a group.
- In session of org2:
  - `GET /api/groups` → empty list.
  - `GET /api/groups/<org1-group-id>` → 404.
  - `DELETE /api/groups/<org1-group-id>` → 404.
- In session of org1: the group is still there.

Apply this shape to every protected entity: groups, submissions, tourists,
hotels (with proper handling of global-catalog rows), documents, uploads,
subgroups.

### 9.3 Public Form Tests

- Valid slug → 201.
- Invalid slug → 404.
- No session header needed (proven via explicit cookie-less request).
- Rate limit: four POSTs from same IP within 10 minutes → fourth is 429.

### 9.4 Manual QA Checklist

- [ ] `/register` → new org → redirected to `/groups`.
- [ ] Logout → redirected to `/login`.
- [ ] Correct login → back to `/groups`.
- [ ] Wrong login → inline error, form preserved.
- [ ] Refresh while on protected route → stays there.
- [ ] Delete session cookie → refresh → redirected to `/login`.
- [ ] Register second org; login as org2 → no org1 data visible.
- [ ] Copy form link button works; link in incognito shows org name and form;
      submission appears only in the correct org's `/submissions`.
- [ ] `/form/doesnotexist` → "link invalid" page.

### 9.5 CI Wiring

Extend `.github/workflows/deploy.yml` (or a separate `ci.yml`):

```yaml
- uses: actions/setup-go@v5
- name: unit tests
  run: cd backend && go test ./...
- name: integration tests
  run: cd backend && go test -tags=integration ./...
  services:
    postgres:
      image: postgres:16
      env: { POSTGRES_PASSWORD: fuji123, POSTGRES_DB: fujitravel }
      ports: ['5499:5432']
```

### 9.6 Not Automated

- Visual appearance of login/register pages.
- Cookie header verification (manual via DevTools).
- Load testing — rate limits + argon2id cost are sufficient for a 1–10 client
  footprint.

## 10. Migration and Deployment

### 10.1 Migration `000014_multi_tenancy`

`up.sql` (summary; exact SQL lives in the plan phase):

```sql
-- 1. New tables
CREATE TABLE organizations (...);
CREATE TABLE users (...);
CREATE TABLE sessions (...);

-- 2. Pre-launch cleanup of existing owned rows (safe: no production data yet)
DELETE FROM documents;
DELETE FROM uploads;
DELETE FROM group_hotels;
DELETE FROM tourists;
DELETE FROM subgroups;
DELETE FROM groups;
DELETE FROM tourist_submissions;

-- 3. Add org_id NOT NULL to now-empty owned tables
ALTER TABLE groups              ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE;
-- (repeat for subgroups, tourists, tourist_submissions, group_hotels, documents, uploads)

-- 4. Hotels — keep existing rows as global catalog (org_id NULL)
ALTER TABLE hotels ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES organizations(id) ON DELETE CASCADE;

-- 5. Indexes on org_id
```

`down.sql` drops indexes, then `org_id` columns, then `sessions`, `users`,
`organizations`. The DELETEd owned rows are NOT restored by `down` (use the
pre-migration backup if needed).

### 10.2 Deployment Steps

```
Step 1 — Backup:
  docker exec fujitravel-admin-db-1 pg_dump -U fuji fujitravel > backup_pre_tier1.sql

Step 2 — Stop backend and web (keep db running):
  docker compose -f docker-compose.prod.yml stop backend web

Step 3 — Pull + build:
  git pull origin main
  docker compose -f docker-compose.prod.yml build backend web

Step 4 — Start backend (auto-applies migration 000014):
  docker compose -f docker-compose.prod.yml up -d backend
  # Wait for "migrations applied" in logs.

Step 5 — Verify health:
  docker compose logs backend --tail=50
  curl http://localhost:3000/api/health

Step 6 — Start web:
  docker compose -f docker-compose.prod.yml up -d web

Step 7 — Create first organization:
  Open https://<domain>/register in a browser, fill the form.
```

Expected downtime: 2–5 minutes during build + start.

### 10.3 Environment Variables Added

```env
APP_SECRET=<32-byte base64>   # Required; no default
APP_ENV=production            # Controls cookie Secure flag; dev|production
```

Generate once:

```bash
openssl rand -base64 32
```

### 10.4 Cleanup Cron (Optional)

```
# Runs daily; removes expired sessions
DELETE FROM sessions WHERE expires_at < NOW();
```

Can be skipped in MVP — expired sessions are harmless beyond disk usage.

## 11. File-Level Change Summary

### New Files

- `backend/migrations/000014_multi_tenancy.{up,down}.sql`
- `backend/internal/auth/argon2.go` + `_test.go`
- `backend/internal/auth/token.go` + `_test.go`
- `backend/internal/auth/slug.go` + `_test.go`
- `backend/internal/auth/password.go` (wrapper around argon2)
- `backend/internal/middleware/auth.go` (requireAuth)
- `backend/internal/middleware/ratelimit.go`
- `backend/internal/api/handlers_auth.go` (register, login, logout, me)
- `backend/internal/api/handlers_public.go` (public/org, public/submit)
- `backend/internal/db/organizations.go`
- `backend/internal/db/users.go`
- `backend/internal/db/sessions.go`
- `backend/internal/db/groups.go` (repository extraction)
- `backend/internal/db/subgroups.go`
- `backend/internal/db/tourists.go`
- `backend/internal/db/submissions.go`
- `backend/internal/db/hotels.go`
- `backend/internal/db/documents.go`
- `backend/internal/db/uploads.go`
- `backend/internal/db/group_hotels.go`
- `backend/integration_test.go` (cross-org isolation coverage)
- `frontend/src/auth/AuthContext.jsx`
- `frontend/src/auth/RequireAuth.jsx`
- `frontend/src/pages/LoginPage.jsx`
- `frontend/src/pages/RegisterPage.jsx`
- `frontend/src/components/CopyFormLinkButton.jsx`

### Modified Files

- `backend/cmd/server/main.go` — route grouping, APP_SECRET env check
- `backend/internal/api/*.go` — rewrite handlers to read `OrgID` from context
  and delegate SQL to repositories
- `backend/internal/api/submissions.go` — split into protected (admin) and
  public (`public/submissions/:slug`) variants
- `frontend/src/App.jsx` — AuthProvider wrapper, new route definitions,
  RequireAuth wrapping admin shell
- `frontend/src/api/client.js` — `apiFetch` wrapper with `credentials:
  include` and 401 redirect; new auth + public endpoint functions; slug-
  aware `publicCreateSubmission`
- `frontend/src/pages/SubmissionFormPage.jsx` — consume slug from URL, show
  org name, use public endpoint
- `frontend/src/components/` admin layout — header with org/user/logout
- `CLAUDE.md` — note SaaS, auth, slug form, org scoping

### Deleted Files

None. All prior work is preserved.

## 12. Open Questions / Deferred

- Email delivery infrastructure (for Tier 2 password reset and invitations).
- Per-user audit log and multiple users per org (Tier 2).
- Captcha / bot protection beyond rate limit.
- Row-Level Security as defense-in-depth (optional Tier 1.5).
- Health check endpoint uptime monitoring external to the stack.
- Cookie domain configuration when frontend and backend run on different
  subdomains (currently both are `<domain>` via nginx).
