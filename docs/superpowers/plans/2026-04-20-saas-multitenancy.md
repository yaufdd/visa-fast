# SaaS Multi-Tenancy (Tier 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convert the single-tenant admin panel into a multi-tenant SaaS for travel agencies: self-service org registration, session-cookie auth, row-level tenancy enforced by a repository pattern, shared+private hotel catalog, agency-specific `/form/<slug>` public URL.

**Architecture:** Row-level tenancy with application-enforced scoping. New `organizations` / `users` / `sessions` tables. `org_id` added to every owned table. Middleware places `org_id` in `context.Context`; every repository function takes it as a mandatory parameter. `/form/<slug>` resolves to an org_id; submissions land in that agency's pool.

**Tech Stack:** Go 1.25, chi router, pgx/v5, argon2id (golang.org/x/crypto/argon2), React+Vite, react-router-dom, PostgreSQL 16.

**Spec:** `docs/superpowers/specs/2026-04-20-saas-multitenancy-design.md`

**Branch strategy:** Work on `feature/saas-multitenancy` branched from `feature/custom-form-workflow` tip.

---

## Execution Order / Phases

Execute phases sequentially. Run `go build ./... && go test ./...` after each backend task, `npm run build` after each frontend task.

- **Phase A — DB Migration** (Task 1)
- **Phase B — Auth Primitives** (Tasks 2-4) — pure Go libs, TDD
- **Phase C — Auth Infra** (Tasks 5-7) — repositories for organizations / users / sessions
- **Phase D — Middleware** (Tasks 8-9)
- **Phase E — Auth Handlers** (Tasks 10-12)
- **Phase F — Entity Repositories + Handler Refactor** (Tasks 13-18)
- **Phase G — Public Slug Handlers** (Task 19)
- **Phase H — Route Rewiring** (Task 20)
- **Phase I — Frontend Auth** (Tasks 21-25)
- **Phase J — Frontend Slug + Admin Header** (Tasks 26-28)
- **Phase K — Integration Tests** (Task 29)
- **Phase L — Deploy + Docs** (Tasks 30-32)

Total: 32 tasks.

---

## Phase A — DB Migration

### Task 1: Migration `000014_multi_tenancy`

**Files:**
- Create: `backend/migrations/000014_multi_tenancy.up.sql`
- Create: `backend/migrations/000014_multi_tenancy.down.sql`

- [ ] **Step 1: Create branch**

```bash
cd /Users/yaufdd/Desktop/fujitravel-admin
git checkout feature/custom-form-workflow
git checkout -b feature/saas-multitenancy
```

- [ ] **Step 2: Write up migration**

Create `backend/migrations/000014_multi_tenancy.up.sql`:

```sql
-- ── New tables ───────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS organizations (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL,
  slug        TEXT UNIQUE NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  email         TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  display_name  TEXT,
  role          TEXT NOT NULL DEFAULT 'owner',
  is_active     BOOLEAN NOT NULL DEFAULT TRUE,
  last_login_at TIMESTAMPTZ,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_users_org_id ON users(org_id);

CREATE TABLE IF NOT EXISTS sessions (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token        TEXT UNIQUE NOT NULL,
  user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at   TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id    ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- ── Pre-launch cleanup of owned rows ────────────────────────────
-- The site has no production users. Wiping owned-table rows so we
-- can add org_id NOT NULL without a backfill. Hotels survive —
-- they become the global catalog (org_id stays NULL).
DELETE FROM documents;
DELETE FROM uploads;
DELETE FROM group_hotels;
DELETE FROM tourists;
DELETE FROM subgroups;
DELETE FROM groups;
DELETE FROM tourist_submissions;

-- ── Add org_id columns ───────────────────────────────────────────
ALTER TABLE groups
  ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL
  REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE subgroups
  ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL
  REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE tourists
  ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL
  REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE tourist_submissions
  ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL
  REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE group_hotels
  ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL
  REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE documents
  ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL
  REFERENCES organizations(id) ON DELETE CASCADE;
ALTER TABLE uploads
  ADD COLUMN IF NOT EXISTS org_id UUID NOT NULL
  REFERENCES organizations(id) ON DELETE CASCADE;

-- Hotels is special: nullable org_id = global, non-null = private
ALTER TABLE hotels
  ADD COLUMN IF NOT EXISTS org_id UUID
  REFERENCES organizations(id) ON DELETE CASCADE;

-- ── Indexes on org_id ────────────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_groups_org_id              ON groups(org_id);
CREATE INDEX IF NOT EXISTS idx_subgroups_org_id           ON subgroups(org_id);
CREATE INDEX IF NOT EXISTS idx_tourists_org_id            ON tourists(org_id);
CREATE INDEX IF NOT EXISTS idx_tourist_submissions_org_id ON tourist_submissions(org_id);
CREATE INDEX IF NOT EXISTS idx_group_hotels_org_id        ON group_hotels(org_id);
CREATE INDEX IF NOT EXISTS idx_documents_org_id           ON documents(org_id);
CREATE INDEX IF NOT EXISTS idx_uploads_org_id             ON uploads(org_id);
CREATE INDEX IF NOT EXISTS idx_hotels_org_id              ON hotels(org_id);
```

- [ ] **Step 3: Write down migration**

Create `backend/migrations/000014_multi_tenancy.down.sql`:

```sql
DROP INDEX IF EXISTS idx_hotels_org_id;
DROP INDEX IF EXISTS idx_uploads_org_id;
DROP INDEX IF EXISTS idx_documents_org_id;
DROP INDEX IF EXISTS idx_group_hotels_org_id;
DROP INDEX IF EXISTS idx_tourist_submissions_org_id;
DROP INDEX IF EXISTS idx_tourists_org_id;
DROP INDEX IF EXISTS idx_subgroups_org_id;
DROP INDEX IF EXISTS idx_groups_org_id;

ALTER TABLE hotels              DROP COLUMN IF EXISTS org_id;
ALTER TABLE uploads             DROP COLUMN IF EXISTS org_id;
ALTER TABLE documents           DROP COLUMN IF EXISTS org_id;
ALTER TABLE group_hotels        DROP COLUMN IF EXISTS org_id;
ALTER TABLE tourist_submissions DROP COLUMN IF EXISTS org_id;
ALTER TABLE tourists            DROP COLUMN IF EXISTS org_id;
ALTER TABLE subgroups           DROP COLUMN IF EXISTS org_id;
ALTER TABLE groups              DROP COLUMN IF EXISTS org_id;

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;
```

- [ ] **Step 4: Test on throwaway DB**

```bash
docker run --rm -d --name fuji-m14 -e POSTGRES_USER=fuji \
  -e POSTGRES_PASSWORD=fuji123 -e POSTGRES_DB=fujitravel \
  -p 5498:5432 postgres:16
sleep 3
export TEST_DB="postgres://fuji:fuji123@localhost:5498/fujitravel?sslmode=disable"
migrate -path backend/migrations -database "$TEST_DB" up
# Expected: all 14 migrations applied cleanly
migrate -path backend/migrations -database "$TEST_DB" down 1
# Expected: 14/d custom_form_workflow reversed (no — 14/d multi_tenancy)
migrate -path backend/migrations -database "$TEST_DB" up
# Expected: 14/u re-applied
docker stop fuji-m14
```

- [ ] **Step 5: Commit**

```bash
git add backend/migrations/000014_multi_tenancy.up.sql \
        backend/migrations/000014_multi_tenancy.down.sql
git commit -m "feat(db): migration 000014 — multi-tenancy tables + org_id columns"
```

---

## Phase B — Auth Primitives (TDD)

### Task 2: argon2id password hash

**Files:**
- Create: `backend/internal/auth/password.go`
- Create: `backend/internal/auth/password_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/auth/password_test.go`:

```go
package auth

import (
	"strings"
	"testing"
)

func TestHashPassword_CorrectPasswordVerifies(t *testing.T) {
	hash, err := HashPassword("super-secret-2026")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, err := VerifyPassword("super-secret-2026", hash)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Error("correct password did not verify")
	}
}

func TestHashPassword_WrongPasswordRejected(t *testing.T) {
	hash, _ := HashPassword("correct")
	ok, _ := VerifyPassword("wrong", hash)
	if ok {
		t.Error("wrong password accepted")
	}
}

func TestHashPassword_RehashesAreDifferent(t *testing.T) {
	h1, _ := HashPassword("same-password")
	h2, _ := HashPassword("same-password")
	if h1 == h2 {
		t.Error("two hashes of same password identical — salt not applied")
	}
}

func TestHashPassword_Format(t *testing.T) {
	h, _ := HashPassword("x")
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Errorf("hash does not start with argon2id marker: %s", h)
	}
}
```

- [ ] **Step 2: Verify tests fail**

`cd backend && go test ./internal/auth/...` — expect `undefined: HashPassword`.

- [ ] **Step 3: Implement**

Create `backend/internal/auth/password.go`:

```go
// Package auth provides password hashing (argon2id), session token
// generation, and org slug generation for the SaaS multi-tenant layer.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP 2024 recommendation for interactive logins).
const (
	argonTime    = 1
	argonMemory  = 64 * 1024 // 64 MB
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword returns an argon2id-encoded password hash.
// Format: "$argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>" (base64 std, unpadded).
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("argon2 salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword returns true if password matches the encoded argon2id hash.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("invalid argon2id hash format")
	}
	var memory uint32
	var time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, fmt.Errorf("parse argon2 params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode salt: %w", err)
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode hash: %w", err)
	}
	computed := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expected)))
	return subtle.ConstantTimeCompare(expected, computed) == 1, nil
}
```

- [ ] **Step 4: Add dependency**

```bash
cd backend && go get golang.org/x/crypto/argon2 && go mod tidy
```

- [ ] **Step 5: Run tests**

`go test ./internal/auth/... -v -run TestHashPassword` — all pass.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/auth/password.go backend/internal/auth/password_test.go \
        backend/go.mod backend/go.sum
git commit -m "feat(auth): argon2id password hashing"
```

---

### Task 3: Session token generator

**Files:**
- Create: `backend/internal/auth/token.go`
- Create: `backend/internal/auth/token_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/auth/token_test.go`:

```go
package auth

import (
	"regexp"
	"testing"
)

func TestNewSessionToken_LengthAndAlphabet(t *testing.T) {
	tok, err := NewSessionToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 43 {
		t.Errorf("token too short: %d (expected >= 43)", len(tok))
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(tok) {
		t.Errorf("token contains non-urlsafe-base64 chars: %q", tok)
	}
}

func TestNewSessionToken_Unique(t *testing.T) {
	seen := make(map[string]bool, 1000)
	for i := 0; i < 1000; i++ {
		tok, err := NewSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		if seen[tok] {
			t.Fatalf("collision after %d iterations", i)
		}
		seen[tok] = true
	}
}
```

- [ ] **Step 2: Implement**

Create `backend/internal/auth/token.go`:

```go
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const sessionTokenBytes = 32

// NewSessionToken returns a URL-safe base64 string of 32 random bytes
// (43 characters without padding). Use for session cookies.
func NewSessionToken() (string, error) {
	b := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("session token rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
```

- [ ] **Step 3: Run tests**

`go test ./internal/auth/... -run TestNewSessionToken -v` — pass.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/auth/token.go backend/internal/auth/token_test.go
git commit -m "feat(auth): session token generator (32 random bytes, url-safe base64)"
```

---

### Task 4: Organization slug generator

**Files:**
- Create: `backend/internal/auth/slug.go`
- Create: `backend/internal/auth/slug_test.go`

- [ ] **Step 1: Tests**

Create `backend/internal/auth/slug_test.go`:

```go
package auth

import (
	"regexp"
	"testing"
)

func TestNewOrgSlug_LengthAndAlphabet(t *testing.T) {
	for i := 0; i < 100; i++ {
		s, err := NewOrgSlug()
		if err != nil {
			t.Fatal(err)
		}
		if len(s) != 7 {
			t.Errorf("slug length %d != 7: %q", len(s), s)
		}
		if !regexp.MustCompile(`^[A-Za-z0-9]+$`).MatchString(s) {
			t.Errorf("slug not base62: %q", s)
		}
	}
}

func TestNewOrgSlug_Unique(t *testing.T) {
	seen := make(map[string]bool, 10000)
	for i := 0; i < 10000; i++ {
		s, _ := NewOrgSlug()
		if seen[s] {
			t.Fatalf("collision after %d iterations: %q", i, s)
		}
		seen[s] = true
	}
}
```

- [ ] **Step 2: Implement**

Create `backend/internal/auth/slug.go`:

```go
package auth

import (
	"crypto/rand"
	"fmt"
)

const (
	slugAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	slugLength   = 7
)

// NewOrgSlug returns a 7-character random base62 string.
// 62^7 = ~3.5 trillion combinations — collisions are negligible, but
// callers should retry on unique-violation from the DB.
func NewOrgSlug() (string, error) {
	buf := make([]byte, slugLength)
	rnd := make([]byte, slugLength)
	if _, err := rand.Read(rnd); err != nil {
		return "", fmt.Errorf("slug rand: %w", err)
	}
	for i, b := range rnd {
		buf[i] = slugAlphabet[int(b)%len(slugAlphabet)]
	}
	return string(buf), nil
}
```

- [ ] **Step 3: Test + commit**

```bash
go test ./internal/auth/... -run TestNewOrgSlug -v
git add backend/internal/auth/slug.go backend/internal/auth/slug_test.go
git commit -m "feat(auth): org slug generator (7 chars base62)"
```

---

## Phase C — Auth Infrastructure Repositories

### Task 5: `db/organizations.go`

**Files:**
- Create: `backend/internal/db/organizations.go`

- [ ] **Step 1: Implement**

Create `backend/internal/db/organizations.go`:

```go
// Package db contains tenant-aware repository functions — every
// owned-entity function takes orgID as a mandatory parameter.
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateOrganization inserts a new org and returns the new ID.
// Caller must handle unique-violation on `slug` by retrying with a new slug.
func CreateOrganization(ctx context.Context, pool *pgxpool.Pool, name, slug string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`,
		name, slug,
	).Scan(&id)
	return id, err
}

// GetOrganizationBySlug is used by public slug-based form endpoints.
func GetOrganizationBySlug(ctx context.Context, pool *pgxpool.Pool, slug string) (*Organization, error) {
	var o Organization
	err := pool.QueryRow(ctx,
		`SELECT id, name, slug, created_at, updated_at
		   FROM organizations WHERE slug = $1`,
		slug,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetOrganizationByID is used by the /me endpoint.
func GetOrganizationByID(ctx context.Context, pool *pgxpool.Pool, id string) (*Organization, error) {
	var o Organization
	err := pool.QueryRow(ctx,
		`SELECT id, name, slug, created_at, updated_at
		   FROM organizations WHERE id = $1`,
		id,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add backend/internal/db/organizations.go
git commit -m "feat(db): organizations repository"
```

---

### Task 6: `db/users.go`

**Files:**
- Create: `backend/internal/db/users.go`

- [ ] **Step 1: Implement**

Create `backend/internal/db/users.go`:

```go
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	DisplayName  *string    `json:"display_name"`
	Role         string     `json:"role"`
	IsActive     bool       `json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// CreateUser inserts a new user row.
func CreateUser(ctx context.Context, pool *pgxpool.Pool, orgID, email, passwordHash string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO users (org_id, email, password_hash)
		 VALUES ($1, $2, $3) RETURNING id`,
		orgID, email, passwordHash,
	).Scan(&id)
	return id, err
}

// GetUserByEmail returns nil, nil when user does not exist (lookup miss).
func GetUserByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (*User, error) {
	var u User
	err := pool.QueryRow(ctx,
		`SELECT id, org_id, email, password_hash, display_name, role,
		        is_active, last_login_at, created_at, updated_at
		   FROM users WHERE email = $1`,
		email,
	).Scan(&u.ID, &u.OrgID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Role,
		&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID returns nil, nil on lookup miss.
func GetUserByID(ctx context.Context, pool *pgxpool.Pool, id string) (*User, error) {
	var u User
	err := pool.QueryRow(ctx,
		`SELECT id, org_id, email, password_hash, display_name, role,
		        is_active, last_login_at, created_at, updated_at
		   FROM users WHERE id = $1`,
		id,
	).Scan(&u.ID, &u.OrgID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.Role,
		&u.IsActive, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// TouchLastLogin sets last_login_at = NOW() for the given user id.
func TouchLastLogin(ctx context.Context, pool *pgxpool.Pool, userID string) error {
	_, err := pool.Exec(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = $1`, userID)
	return err
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add backend/internal/db/users.go
git commit -m "feat(db): users repository"
```

---

### Task 7: `db/sessions.go`

**Files:**
- Create: `backend/internal/db/sessions.go`

- [ ] **Step 1: Implement**

Create `backend/internal/db/sessions.go`:

```go
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionLookup is what middleware needs on every protected request:
// userID + orgID in one row (JOIN users).
type SessionLookup struct {
	UserID    string
	OrgID     string
	ExpiresAt time.Time
}

// CreateSession inserts a new session row with TTL days of lifetime.
func CreateSession(ctx context.Context, pool *pgxpool.Pool, userID, token string, ttl time.Duration) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO sessions (user_id, token, expires_at)
		 VALUES ($1, $2, NOW() + $3::interval)`,
		userID, token, ttl.String(),
	)
	return err
}

// LookupSession joins sessions + users to get user_id and org_id in one round-trip.
// Returns nil, nil if token does not exist, is expired, or user is inactive.
func LookupSession(ctx context.Context, pool *pgxpool.Pool, token string) (*SessionLookup, error) {
	var s SessionLookup
	err := pool.QueryRow(ctx,
		`SELECT s.user_id, u.org_id, s.expires_at
		   FROM sessions s
		   JOIN users u ON u.id = s.user_id
		  WHERE s.token = $1
		    AND s.expires_at > NOW()
		    AND u.is_active = TRUE`,
		token,
	).Scan(&s.UserID, &s.OrgID, &s.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// TouchSession updates last_seen_at and, when under threshold, extends expires_at.
func TouchSession(ctx context.Context, pool *pgxpool.Pool, token string, extend bool, ttl time.Duration) error {
	var q string
	var args []any
	if extend {
		q = `UPDATE sessions SET expires_at = NOW() + $2::interval, last_seen_at = NOW() WHERE token = $1`
		args = []any{token, ttl.String()}
	} else {
		q = `UPDATE sessions SET last_seen_at = NOW() WHERE token = $1`
		args = []any{token}
	}
	_, err := pool.Exec(ctx, q, args...)
	return err
}

// DeleteSession removes a session row (used by logout).
func DeleteSession(ctx context.Context, pool *pgxpool.Pool, token string) error {
	_, err := pool.Exec(ctx, `DELETE FROM sessions WHERE token = $1`, token)
	return err
}

// DeleteExpiredSessions is a cron helper — removes old rows.
func DeleteExpiredSessions(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add backend/internal/db/sessions.go
git commit -m "feat(db): sessions repository"
```

---

## Phase D — Middleware

### Task 8: `middleware/auth.go` (requireAuth)

**Files:**
- Create: `backend/internal/middleware/auth.go`

- [ ] **Step 1: Implement**

Create `backend/internal/middleware/auth.go`:

```go
// Package middleware holds HTTP middleware for the admin API.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/db"
)

type ctxKey int

const (
	ctxKeyUserID ctxKey = iota
	ctxKeyOrgID
)

const (
	sessionCookieName = "session"
	sessionTTL        = 30 * 24 * time.Hour
	sessionRefreshIf  = 15 * 24 * time.Hour
)

// UserID returns the authenticated user ID from context.
// Panics if called outside a request that went through RequireAuth.
func UserID(ctx context.Context) string {
	v, ok := ctx.Value(ctxKeyUserID).(string)
	if !ok {
		panic("middleware.UserID called without RequireAuth middleware")
	}
	return v
}

// OrgID returns the authenticated user's org ID from context.
// Panics if called outside a request that went through RequireAuth.
func OrgID(ctx context.Context) string {
	v, ok := ctx.Value(ctxKeyOrgID).(string)
	if !ok {
		panic("middleware.OrgID called without RequireAuth middleware")
	}
	return v
}

// RequireAuth reads the session cookie, looks up the session + user,
// places user_id and org_id in request context, and chains to the next
// handler. Unauthenticated requests get 401.
func RequireAuth(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || cookie.Value == "" {
				writeAuthError(w, 401, "not authenticated")
				return
			}

			sess, err := db.LookupSession(r.Context(), pool, cookie.Value)
			if err != nil {
				writeAuthError(w, 500, "db error")
				return
			}
			if sess == nil {
				writeAuthError(w, 401, "session expired")
				return
			}

			extend := time.Until(sess.ExpiresAt) < sessionRefreshIf
			if err := db.TouchSession(r.Context(), pool, cookie.Value, extend, sessionTTL); err != nil {
				// Non-fatal — continue anyway. Log it though.
				// log.Printf("touch session: %v", err)
			}

			ctx := context.WithValue(r.Context(), ctxKeyUserID, sess.UserID)
			ctx = context.WithValue(ctx, ctxKeyOrgID, sess.OrgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeAuthError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add backend/internal/middleware/auth.go
git commit -m "feat(middleware): requireAuth — session cookie + context injection"
```

---

### Task 9: `middleware/ratelimit.go`

**Files:**
- Create: `backend/internal/middleware/ratelimit.go`

- [ ] **Step 1: Implement**

Create `backend/internal/middleware/ratelimit.go`:

```go
package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

// rateEntry tracks attempts for a single key within a window.
type rateEntry struct {
	count int
	reset time.Time
}

// RateLimiter is an in-memory sliding-window-ish limiter keyed by IP.
// Intentionally simple: per-handler instance, no Redis, loses state on restart.
// Fine for MVP.
type RateLimiter struct {
	mu       sync.Mutex
	entries  map[string]*rateEntry
	limit    int
	window   time.Duration
}

// NewRateLimiter returns a limiter allowing `limit` requests per `window`.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*rateEntry),
		limit:   limit,
		window:  window,
	}
	go rl.gc()
	return rl
}

// Middleware is the chi-compatible http.Handler middleware.
func (rl *RateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !rl.allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "900") // 15 minutes
				w.WriteHeader(429)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":       "too many attempts",
					"retry_after": 900,
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (rl *RateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	e, ok := rl.entries[key]
	if !ok || now.After(e.reset) {
		rl.entries[key] = &rateEntry{count: 1, reset: now.Add(rl.window)}
		return true
	}
	if e.count >= rl.limit {
		return false
	}
	e.count++
	return true
}

// gc removes expired entries every minute.
func (rl *RateLimiter) gc() {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for range tick.C {
		rl.mu.Lock()
		now := time.Now()
		for k, e := range rl.entries {
			if now.After(e.reset) {
				delete(rl.entries, k)
			}
		}
		rl.mu.Unlock()
	}
}

func clientIP(r *http.Request) string {
	// chi's RealIP middleware already sets r.RemoteAddr to the client IP
	// when behind a proxy. Strip the port if present.
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add backend/internal/middleware/ratelimit.go
git commit -m "feat(middleware): in-memory IP rate limiter"
```

---

## Phase E — Auth Handlers

### Task 10: `POST /api/auth/register`

**Files:**
- Create: `backend/internal/api/handlers_auth.go`

- [ ] **Step 1: Implement Register handler**

Create `backend/internal/api/handlers_auth.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/auth"
	"fujitravel-admin/backend/internal/db"
	"fujitravel-admin/backend/internal/middleware"
)

type authUserResp struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName *string `json:"display_name"`
	Role        string  `json:"role"`
}

type authOrgResp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type authMeResp struct {
	User authUserResp `json:"user"`
	Org  authOrgResp  `json:"org"`
}

const sessionCookieName = "session"
const sessionTTL = 30 * 24 * time.Hour

// Register handles POST /api/auth/register.
// Body: {org_name, email, password}
func Register(pool *pgxpool.Pool) http.HandlerFunc {
	type req struct {
		OrgName  string `json:"org_name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, 400, "invalid JSON")
			return
		}
		body.OrgName = strings.TrimSpace(body.OrgName)
		body.Email = strings.ToLower(strings.TrimSpace(body.Email))

		if body.OrgName == "" {
			writeError(w, 400, "org_name required")
			return
		}
		if _, err := mail.ParseAddress(body.Email); err != nil {
			writeError(w, 400, "invalid email")
			return
		}
		if len(body.Password) < 8 {
			writeError(w, 400, "password too short (min 8)")
			return
		}

		hash, err := auth.HashPassword(body.Password)
		if err != nil {
			slog.Error("hash password", "err", err)
			writeError(w, 500, "internal error")
			return
		}

		// Retry slug generation on unique-violation
		var orgID string
		for attempt := 0; attempt < 5; attempt++ {
			slug, err := auth.NewOrgSlug()
			if err != nil {
				writeError(w, 500, "slug gen error")
				return
			}
			orgID, err = db.CreateOrganization(r.Context(), pool, body.OrgName, slug)
			if err == nil {
				break
			}
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				continue // slug collision — try again
			}
			slog.Error("create organization", "err", err)
			writeError(w, 500, "db error")
			return
		}
		if orgID == "" {
			writeError(w, 500, "could not allocate slug")
			return
		}

		userID, err := db.CreateUser(r.Context(), pool, orgID, body.Email, hash)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeError(w, 409, "email already registered")
				return
			}
			slog.Error("create user", "err", err)
			writeError(w, 500, "db error")
			return
		}

		token, err := auth.NewSessionToken()
		if err != nil {
			writeError(w, 500, "token gen error")
			return
		}
		if err := db.CreateSession(r.Context(), pool, userID, token, sessionTTL); err != nil {
			slog.Error("create session", "err", err)
			writeError(w, 500, "db error")
			return
		}
		setSessionCookie(w, token)

		// Respond with current user + org
		user, _ := db.GetUserByID(r.Context(), pool, userID)
		org, _ := db.GetOrganizationByID(r.Context(), pool, orgID)
		writeJSON(w, 201, authMeResp{
			User: authUserResp{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role},
			Org:  authOrgResp{ID: org.ID, Name: org.Name, Slug: org.Slug},
		})
	}
}

func setSessionCookie(w http.ResponseWriter, token string) {
	secure := os.Getenv("APP_ENV") == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func _unused_middleware() { _ = middleware.OrgID }
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add backend/internal/api/handlers_auth.go
git commit -m "feat(api): POST /api/auth/register"
```

---

### Task 11: `POST /api/auth/login`

**Files:**
- Modify: `backend/internal/api/handlers_auth.go`

- [ ] **Step 1: Append Login handler**

Append to `backend/internal/api/handlers_auth.go`:

```go
// Login handles POST /api/auth/login. Body: {email, password}.
func Login(pool *pgxpool.Pool) http.HandlerFunc {
	type req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, 400, "invalid JSON")
			return
		}
		body.Email = strings.ToLower(strings.TrimSpace(body.Email))

		user, err := db.GetUserByEmail(r.Context(), pool, body.Email)
		if err != nil {
			slog.Error("get user by email", "err", err)
			writeError(w, 500, "db error")
			return
		}
		if user == nil || !user.IsActive {
			writeError(w, 401, "invalid credentials")
			return
		}
		ok, err := auth.VerifyPassword(body.Password, user.PasswordHash)
		if err != nil || !ok {
			writeError(w, 401, "invalid credentials")
			return
		}

		token, err := auth.NewSessionToken()
		if err != nil {
			writeError(w, 500, "token gen")
			return
		}
		if err := db.CreateSession(r.Context(), pool, user.ID, token, sessionTTL); err != nil {
			writeError(w, 500, "create session")
			return
		}
		if err := db.TouchLastLogin(r.Context(), pool, user.ID); err != nil {
			// Non-fatal
		}
		setSessionCookie(w, token)

		org, _ := db.GetOrganizationByID(r.Context(), pool, user.OrgID)
		writeJSON(w, 200, authMeResp{
			User: authUserResp{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role},
			Org:  authOrgResp{ID: org.ID, Name: org.Name, Slug: org.Slug},
		})
	}
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add backend/internal/api/handlers_auth.go
git commit -m "feat(api): POST /api/auth/login"
```

---

### Task 12: Logout + Me

**Files:**
- Modify: `backend/internal/api/handlers_auth.go`

- [ ] **Step 1: Append handlers**

Append to `backend/internal/api/handlers_auth.go`:

```go
// Logout handles POST /api/auth/logout. Deletes current session.
func Logout(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookieName); err == nil {
			_ = db.DeleteSession(r.Context(), pool, cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   os.Getenv("APP_ENV") == "production",
			SameSite: http.SameSiteLaxMode,
		})
		writeJSON(w, 200, map[string]bool{"ok": true})
	}
}

// Me returns the currently authenticated user + org.
func Me(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.UserID(r.Context())
		orgID := middleware.OrgID(r.Context())
		user, err := db.GetUserByID(r.Context(), pool, userID)
		if err != nil || user == nil {
			writeError(w, 500, "user not found")
			return
		}
		org, err := db.GetOrganizationByID(r.Context(), pool, orgID)
		if err != nil || org == nil {
			writeError(w, 500, "org not found")
			return
		}
		writeJSON(w, 200, authMeResp{
			User: authUserResp{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role},
			Org:  authOrgResp{ID: org.ID, Name: org.Name, Slug: org.Slug},
		})
	}
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add backend/internal/api/handlers_auth.go
git commit -m "feat(api): POST /api/auth/logout, GET /api/auth/me"
```

---

## Phase F — Entity Repositories + Handler Refactor

**Pattern for each task:** create `backend/internal/db/<entity>.go` with repo functions that take `orgID` as mandatory parameter, then refactor `backend/internal/api/<entity>.go` to read `middleware.OrgID(r.Context())` and delegate SQL to the repo.

### Task 13: Groups

**Files:**
- Create: `backend/internal/db/groups.go`
- Modify: `backend/internal/api/groups.go`

- [ ] **Step 1: Create repository**

Create `backend/internal/db/groups.go`:

```go
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Group struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"-"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Notes     *string   `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ListGroups(ctx context.Context, pool *pgxpool.Pool, orgID string) ([]Group, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, org_id, name, status, notes, created_at, updated_at
		   FROM groups WHERE org_id = $1
		   ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.OrgID, &g.Name, &g.Status, &g.Notes, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func GetGroup(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (*Group, error) {
	var g Group
	err := pool.QueryRow(ctx,
		`SELECT id, org_id, name, status, notes, created_at, updated_at
		   FROM groups WHERE id = $1 AND org_id = $2`, id, orgID,
	).Scan(&g.ID, &g.OrgID, &g.Name, &g.Status, &g.Notes, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func CreateGroup(ctx context.Context, pool *pgxpool.Pool, orgID, name string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO groups (org_id, name) VALUES ($1, $2) RETURNING id`,
		orgID, name,
	).Scan(&id)
	return id, err
}

func DeleteGroup(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM groups WHERE id = $1 AND org_id = $2`, id, orgID)
	return tag.RowsAffected() > 0, err
}

func UpdateGroupStatus(ctx context.Context, pool *pgxpool.Pool, orgID, id, status string) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE groups SET status = $1, updated_at = NOW()
		  WHERE id = $2 AND org_id = $3`, status, id, orgID)
	return tag.RowsAffected() > 0, err
}

func UpdateGroupNotes(ctx context.Context, pool *pgxpool.Pool, orgID, id string, notes *string) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE groups SET notes = $1, updated_at = NOW()
		  WHERE id = $2 AND org_id = $3`, notes, id, orgID)
	return tag.RowsAffected() > 0, err
}
```

- [ ] **Step 2: Refactor api/groups.go handlers**

Open `backend/internal/api/groups.go`. Replace every SQL block with a call to the corresponding `db.*` function. Each handler now reads `orgID := middleware.OrgID(r.Context())` at the top and passes it through. If a lookup returns `nil, nil` → 404. If Update/Delete returns `false` → 404.

Example `ListGroups` handler after refactor:

```go
func ListGroups(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		orgID := middleware.OrgID(r.Context())
		groups, err := db.ListGroups(r.Context(), pool, orgID)
		if err != nil {
			slog.Error("list groups", "err", err)
			writeError(w, 500, "db")
			return
		}
		writeJSON(w, 200, groups)
	}
}
```

Apply the same pattern to `GetGroup`, `CreateGroup`, `DeleteGroup`, `UpdateGroupStatus`, `UpdateGroupNotes`. Remove all inline SQL from the file.

Add imports:
```go
import (
    "fujitravel-admin/backend/internal/db"
    "fujitravel-admin/backend/internal/middleware"
)
```

- [ ] **Step 3: Build + commit**

```bash
go build ./...
git add backend/internal/db/groups.go backend/internal/api/groups.go
git commit -m "feat(db+api): groups — repository + tenant-scoped handlers"
```

---

### Task 14: Subgroups

**Files:**
- Create: `backend/internal/db/subgroups.go`
- Modify: `backend/internal/api/subgroups.go`

- [ ] **Step 1: Create repository**

Create `backend/internal/db/subgroups.go`:

```go
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Subgroup struct {
	ID        string    `json:"id"`
	GroupID   string    `json:"group_id"`
	Name      string    `json:"name"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

func ListSubgroups(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string) ([]Subgroup, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, group_id, name, sort_order, created_at
		   FROM subgroups WHERE group_id = $1 AND org_id = $2
		   ORDER BY sort_order, created_at`, groupID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Subgroup{}
	for rows.Next() {
		var s Subgroup
		if err := rows.Scan(&s.ID, &s.GroupID, &s.Name, &s.SortOrder, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func CreateSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, groupID, name string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO subgroups (org_id, group_id, name)
		 SELECT $1, $2, $3 WHERE EXISTS (SELECT 1 FROM groups WHERE id = $2 AND org_id = $1)
		 RETURNING id`, orgID, groupID, name,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil // group not found or belongs to another org
	}
	return id, err
}

func UpdateSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, id, name string, sortOrder int) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE subgroups SET name = $1, sort_order = $2
		  WHERE id = $3 AND org_id = $4`, name, sortOrder, id, orgID)
	return tag.RowsAffected() > 0, err
}

func DeleteSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM subgroups WHERE id = $1 AND org_id = $2`, id, orgID)
	return tag.RowsAffected() > 0, err
}
```

- [ ] **Step 2: Refactor api/subgroups.go handlers**

Follow the same pattern: `orgID := middleware.OrgID(r.Context())`, delegate SQL to `db.*Subgroups`. Note the `CreateSubgroup` EXISTS check which protects against attaching a subgroup to a group owned by another org.

- [ ] **Step 3: Build + commit**

```bash
go build ./...
git add backend/internal/db/subgroups.go backend/internal/api/subgroups.go
git commit -m "feat(db+api): subgroups — repository + tenant-scoped handlers"
```

---

### Task 15: Tourists + flight_data

**Files:**
- Create: `backend/internal/db/tourists.go`
- Modify: `backend/internal/api/tourists.go`
- Modify: `backend/internal/api/flight_data.go`

- [ ] **Step 1: Create repository**

Create `backend/internal/db/tourists.go`:

```go
package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Tourist struct {
	ID                 string          `json:"id"`
	GroupID            string          `json:"group_id"`
	SubgroupID         *string         `json:"subgroup_id"`
	SubmissionID       *string         `json:"submission_id"`
	SubmissionSnapshot json.RawMessage `json:"submission_snapshot"`
	FlightData         json.RawMessage `json:"flight_data"`
	Translations       json.RawMessage `json:"translations"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

func ListTouristsByGroup(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string) ([]Tourist, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, group_id, subgroup_id, submission_id, submission_snapshot,
		        flight_data, translations, created_at, updated_at
		   FROM tourists
		  WHERE group_id = $1 AND org_id = $2
		  ORDER BY created_at`, groupID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tourist{}
	for rows.Next() {
		var t Tourist
		var snap, flight, tr []byte
		if err := rows.Scan(&t.ID, &t.GroupID, &t.SubgroupID, &t.SubmissionID,
			&snap, &flight, &tr, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		t.SubmissionSnapshot = snap
		t.FlightData = flight
		t.Translations = tr
		out = append(out, t)
	}
	return out, nil
}

func DeleteTourist(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tag, err := pool.Exec(ctx, `DELETE FROM tourists WHERE id = $1 AND org_id = $2`, id, orgID)
	return tag.RowsAffected() > 0, err
}

func AssignTouristSubgroup(ctx context.Context, pool *pgxpool.Pool, orgID, touristID string, subgroupID *string) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE tourists SET subgroup_id = $1, updated_at = NOW()
		  WHERE id = $2 AND org_id = $3`, subgroupID, touristID, orgID)
	return tag.RowsAffected() > 0, err
}

func UpdateFlightData(ctx context.Context, pool *pgxpool.Pool, orgID, touristID string, data []byte) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE tourists SET flight_data = $1, updated_at = NOW()
		  WHERE id = $2 AND org_id = $3`, data, touristID, orgID)
	return tag.RowsAffected() > 0, err
}

// AttachSubmissionToGroup — used by POST /api/submissions/:id/attach.
// Runs inside a transaction: checks submission is pending, inserts tourist, marks submission attached.
// All three operations scoped to orgID.
func AttachSubmissionToGroup(ctx context.Context, pool *pgxpool.Pool, orgID, submissionID, groupID string, subgroupID *string) (string, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var payload []byte
	var status string
	err = tx.QueryRow(ctx,
		`SELECT payload, status FROM tourist_submissions
		  WHERE id = $1 AND org_id = $2 FOR UPDATE`, submissionID, orgID,
	).Scan(&payload, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if status == "attached" {
		return "", ErrAlreadyAttached
	}

	// Validate group belongs to same org
	var ok bool
	err = tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM groups WHERE id = $1 AND org_id = $2)`,
		groupID, orgID,
	).Scan(&ok)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNotFound
	}

	var touristID string
	err = tx.QueryRow(ctx,
		`INSERT INTO tourists (org_id, group_id, subgroup_id, submission_id, submission_snapshot)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		orgID, groupID, subgroupID, submissionID, payload,
	).Scan(&touristID)
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE tourist_submissions SET status = 'attached', updated_at = NOW()
		  WHERE id = $1`, submissionID); err != nil {
		return "", err
	}
	return touristID, tx.Commit(ctx)
}

var (
	ErrNotFound       = errors.New("not found")
	ErrAlreadyAttached = errors.New("submission already attached")
)
```

- [ ] **Step 2: Refactor handlers**

In `backend/internal/api/tourists.go`: replace SQL with `db.ListTouristsByGroup`, `db.DeleteTourist`, `db.AssignTouristSubgroup`. Read orgID from middleware.

In `backend/internal/api/flight_data.go`: replace SQL in `UpdateFlightData` with `db.UpdateFlightData`.

- [ ] **Step 3: Build + commit**

```bash
go build ./...
git add backend/internal/db/tourists.go backend/internal/api/tourists.go backend/internal/api/flight_data.go
git commit -m "feat(db+api): tourists — repository + tenant-scoped handlers"
```

---

### Task 16: Submissions — split public and protected

**Files:**
- Create: `backend/internal/db/submissions.go`
- Modify: `backend/internal/api/submissions.go`

- [ ] **Step 1: Create repository**

Create `backend/internal/db/submissions.go`:

```go
package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

// CreateSubmissionForOrg is used by both the public slug endpoint and
// manager "create manually" flow. orgID comes from either slug resolve
// or the session.
func CreateSubmissionForOrg(ctx context.Context, pool *pgxpool.Pool, orgID string, payload []byte, consentVersion, source string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO tourist_submissions
		   (org_id, payload, consent_accepted, consent_accepted_at, consent_version, source)
		 VALUES ($1, $2, TRUE, NOW(), $3, $4) RETURNING id`,
		orgID, payload, consentVersion, source,
	).Scan(&id)
	return id, err
}

func ListSubmissions(ctx context.Context, pool *pgxpool.Pool, orgID, q, status string) ([]TouristSubmission, error) {
	args := []any{orgID}
	where := []string{"org_id = $1"}
	if q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, fmt.Sprintf("payload ->> 'name_lat' ILIKE $%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	sql := `SELECT id, payload, consent_accepted, consent_accepted_at, consent_version,
	               source, status, created_at, updated_at
	          FROM tourist_submissions
	         WHERE ` + strings.Join(where, " AND ") + `
	         ORDER BY created_at DESC LIMIT 500`

	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TouristSubmission{}
	for rows.Next() {
		var s TouristSubmission
		var payload []byte
		if err := rows.Scan(&s.ID, &payload, &s.ConsentAccepted, &s.ConsentAcceptedAt,
			&s.ConsentVersion, &s.Source, &s.Status, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Payload = payload
		out = append(out, s)
	}
	return out, nil
}

func GetSubmission(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (*TouristSubmission, error) {
	var s TouristSubmission
	var payload []byte
	err := pool.QueryRow(ctx,
		`SELECT id, payload, consent_accepted, consent_accepted_at, consent_version,
		        source, status, created_at, updated_at
		   FROM tourist_submissions WHERE id = $1 AND org_id = $2`, id, orgID,
	).Scan(&s.ID, &payload, &s.ConsentAccepted, &s.ConsentAcceptedAt,
		&s.ConsentVersion, &s.Source, &s.Status, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Payload = payload
	return &s, nil
}

func UpdateSubmission(ctx context.Context, pool *pgxpool.Pool, orgID, id string, payload []byte) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE tourist_submissions SET payload = $1, updated_at = NOW()
		  WHERE id = $2 AND org_id = $3`, payload, id, orgID)
	return tag.RowsAffected() > 0, err
}

func ArchiveSubmission(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE tourist_submissions SET status = 'archived', updated_at = NOW()
		  WHERE id = $1 AND org_id = $2 AND status != 'archived'`, id, orgID)
	return tag.RowsAffected() > 0, err
}

func EraseSubmission(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (bool, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if _, err := tx.Exec(ctx,
		`UPDATE tourists SET submission_snapshot = NULL, submission_id = NULL
		  WHERE submission_id = $1 AND org_id = $2`, id, orgID); err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM tourist_submissions WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	return true, tx.Commit(ctx)
}
```

- [ ] **Step 2: Refactor api/submissions.go**

Rewrite handlers to use repository functions. Key changes:
- Remove the current `CreateSubmission` public handler entirely (public endpoint moves to `handlers_public.go` in Task 19).
- Keep: `ListSubmissions`, `GetSubmission`, `UpdateSubmission`, `ArchiveSubmission`, `EraseSubmission`, `AttachSubmission`, `GetConsentText` — all now scope to `orgID := middleware.OrgID(r.Context())`.
- `AttachSubmission` now calls `db.AttachSubmissionToGroup` from tourists.go repo.

- [ ] **Step 3: Build + commit**

```bash
go build ./...
git add backend/internal/db/submissions.go backend/internal/api/submissions.go
git commit -m "feat(db+api): submissions — repository + tenant-scoped handlers (admin only)"
```

---

### Task 17: Hotels — shared + private catalog

**Files:**
- Create: `backend/internal/db/hotels.go`
- Modify: `backend/internal/api/hotels.go`

- [ ] **Step 1: Create repository**

Create `backend/internal/db/hotels.go`:

```go
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Hotel struct {
	ID        string    `json:"id"`
	NameEn    string    `json:"name_en"`
	NameRu    *string   `json:"name_ru"`
	City      *string   `json:"city"`
	Address   *string   `json:"address"`
	Phone     *string   `json:"phone"`
	IsGlobal  bool      `json:"is_global"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListHotels returns global hotels (org_id IS NULL) plus the calling
// org's private hotels. Private ones come first in the result.
func ListHotels(ctx context.Context, pool *pgxpool.Pool, orgID string) ([]Hotel, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, name_en, name_ru, city, address, phone,
		        (org_id IS NULL) AS is_global,
		        created_at, updated_at
		   FROM hotels
		  WHERE org_id IS NULL OR org_id = $1
		  ORDER BY (org_id IS NOT NULL) DESC, name_en`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hotel{}
	for rows.Next() {
		var h Hotel
		if err := rows.Scan(&h.ID, &h.NameEn, &h.NameRu, &h.City, &h.Address, &h.Phone,
			&h.IsGlobal, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

// GetHotel returns a hotel visible to the org (global or private).
func GetHotel(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (*Hotel, error) {
	var h Hotel
	err := pool.QueryRow(ctx,
		`SELECT id, name_en, name_ru, city, address, phone,
		        (org_id IS NULL) AS is_global,
		        created_at, updated_at
		   FROM hotels
		  WHERE id = $1 AND (org_id IS NULL OR org_id = $2)`, id, orgID,
	).Scan(&h.ID, &h.NameEn, &h.NameRu, &h.City, &h.Address, &h.Phone,
		&h.IsGlobal, &h.CreatedAt, &h.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// CreateHotel always creates as private to the calling org.
func CreateHotel(ctx context.Context, pool *pgxpool.Pool, orgID string, h Hotel) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO hotels (org_id, name_en, name_ru, city, address, phone)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		orgID, h.NameEn, h.NameRu, h.City, h.Address, h.Phone,
	).Scan(&id)
	return id, err
}

// UpdateHotel can only update PRIVATE hotels (org_id = $orgID). Global
// hotels (org_id IS NULL) are read-only for all orgs.
// Returns (false, nil) when the hotel does not exist or is global.
func UpdateHotel(ctx context.Context, pool *pgxpool.Pool, orgID, id string, h Hotel) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE hotels SET name_en = $1, name_ru = $2, city = $3,
		                   address = $4, phone = $5, updated_at = NOW()
		  WHERE id = $6 AND org_id = $7`,
		h.NameEn, h.NameRu, h.City, h.Address, h.Phone, id, orgID)
	return tag.RowsAffected() > 0, err
}
```

- [ ] **Step 2: Refactor api/hotels.go**

Replace all SQL with `db.ListHotels`, `db.GetHotel`, `db.CreateHotel`, `db.UpdateHotel`. `orgID` comes from middleware. Return 404 for attempts to update a global hotel (it's the same 404 as "doesn't exist" — that's intentional).

- [ ] **Step 3: Build + commit**

```bash
go build ./...
git add backend/internal/db/hotels.go backend/internal/api/hotels.go
git commit -m "feat(db+api): hotels — shared+private catalog with tenant scoping"
```

---

### Task 18: Group hotels + documents + uploads + generate

**Files:**
- Create: `backend/internal/db/group_hotels.go`
- Create: `backend/internal/db/documents.go`
- Create: `backend/internal/db/uploads.go`
- Modify: `backend/internal/api/uploads.go`
- Modify: `backend/internal/api/generate.go`

These three entities are closely tied together (uploads trigger parsers that create group_hotels, generate writes documents). Bundle into one task.

- [ ] **Step 1: Create `db/group_hotels.go`**

```go
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type GroupHotel struct {
	ID         string    `json:"id"`
	GroupID    string    `json:"group_id"`
	SubgroupID *string   `json:"subgroup_id"`
	HotelID    string    `json:"hotel_id"`
	CheckIn    time.Time `json:"check_in"`
	CheckOut   time.Time `json:"check_out"`
	RoomType   *string   `json:"room_type"`
	SortOrder  int       `json:"sort_order"`
}

func ListGroupHotels(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string) ([]GroupHotel, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, group_id, subgroup_id, hotel_id, check_in, check_out, room_type, sort_order
		   FROM group_hotels WHERE group_id = $1 AND org_id = $2
		   ORDER BY sort_order`, groupID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GroupHotel{}
	for rows.Next() {
		var gh GroupHotel
		if err := rows.Scan(&gh.ID, &gh.GroupID, &gh.SubgroupID, &gh.HotelID,
			&gh.CheckIn, &gh.CheckOut, &gh.RoomType, &gh.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, gh)
	}
	return out, nil
}

func UpsertGroupHotels(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string, hotels []GroupHotel) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx,
		`DELETE FROM group_hotels WHERE group_id = $1 AND org_id = $2`, groupID, orgID); err != nil {
		return err
	}
	for i, gh := range hotels {
		_, err = tx.Exec(ctx,
			`INSERT INTO group_hotels (org_id, group_id, subgroup_id, hotel_id, check_in, check_out, room_type, sort_order)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			orgID, groupID, gh.SubgroupID, gh.HotelID, gh.CheckIn, gh.CheckOut, gh.RoomType, i)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
```

- [ ] **Step 2: Create `db/documents.go`**

```go
package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Document struct {
	ID          string          `json:"id"`
	GroupID     string          `json:"group_id"`
	Pass2JSON   json.RawMessage `json:"pass2_json"`
	ZipPath     string          `json:"zip_path"`
	GeneratedAt time.Time       `json:"generated_at"`
	CreatedAt   time.Time       `json:"created_at"`
}

func CreateDocument(ctx context.Context, pool *pgxpool.Pool, orgID, groupID, zipPath string, pass2 []byte, generatedAt time.Time) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO documents (org_id, group_id, pass2_json, zip_path, generated_at)
		 VALUES ($1, $2, $3, $4, $5)`, orgID, groupID, pass2, zipPath, generatedAt)
	return err
}

func LatestDocumentForGroup(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string) (*Document, error) {
	var d Document
	var pass2 []byte
	err := pool.QueryRow(ctx,
		`SELECT id, group_id, pass2_json, zip_path, generated_at, created_at
		   FROM documents
		  WHERE group_id = $1 AND org_id = $2
		  ORDER BY generated_at DESC LIMIT 1`, groupID, orgID,
	).Scan(&d.ID, &d.GroupID, &pass2, &d.ZipPath, &d.GeneratedAt, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	d.Pass2JSON = pass2
	return &d, nil
}
```

- [ ] **Step 3: Create `db/uploads.go`**

```go
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Upload struct {
	ID              string    `json:"id"`
	GroupID         string    `json:"group_id"`
	TouristID       *string   `json:"tourist_id"`
	SubgroupID      *string   `json:"subgroup_id"`
	FileType        string    `json:"file_type"`
	FilePath        string    `json:"file_path"`
	AnthropicFileID *string   `json:"anthropic_file_id"`
	CreatedAt       time.Time `json:"created_at"`
}

func InsertUpload(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string, touristID *string, fileType, filePath string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO uploads (org_id, group_id, tourist_id, file_type, file_path)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		orgID, groupID, touristID, fileType, filePath,
	).Scan(&id)
	return id, err
}

func ListTouristUploads(ctx context.Context, pool *pgxpool.Pool, orgID, touristID string) ([]Upload, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, group_id, tourist_id, subgroup_id, file_type, file_path, anthropic_file_id, created_at
		   FROM uploads WHERE tourist_id = $1 AND org_id = $2
		   ORDER BY created_at DESC`, touristID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Upload{}
	for rows.Next() {
		var u Upload
		if err := rows.Scan(&u.ID, &u.GroupID, &u.TouristID, &u.SubgroupID, &u.FileType,
			&u.FilePath, &u.AnthropicFileID, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

func SetUploadAnthropicID(ctx context.Context, pool *pgxpool.Pool, orgID, uploadID, fileID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE uploads SET anthropic_file_id = $1 WHERE id = $2 AND org_id = $3`,
		fileID, uploadID, orgID)
	return err
}
```

- [ ] **Step 4: Refactor api/uploads.go**

In every handler that inserts or queries `uploads` / `hotels` / `group_hotels`: read `orgID` from middleware, pass it through. The ticket/voucher auto-parse code already runs after the upload is saved — just pass `orgID` into the hotel-creation logic (insert `org_id = orgID` in `hotels` for private hotels created from voucher parse).

- [ ] **Step 5: Refactor api/generate.go**

Replace all `db.Query` / `pool.Exec` calls in `GenerateDocuments`, `GenerateSubgroupDocuments`, and `FinalizeGroup` with repository calls. Specifically:
- `loadTouristsForGeneration` → uses `db.ListTouristsByGroup`
- `loadGroupHotels` → JOINs group_hotels + hotels; pass `orgID` through
- Document insert → `db.CreateDocument(ctx, pool, orgID, groupID, ...)`
- Document read (for finalize) → `db.LatestDocumentForGroup(ctx, pool, orgID, groupID)`

Every entry point reads `orgID := middleware.OrgID(r.Context())`.

- [ ] **Step 6: Build + commit**

```bash
go build ./...
git add backend/internal/db/group_hotels.go backend/internal/db/documents.go backend/internal/db/uploads.go \
        backend/internal/api/uploads.go backend/internal/api/generate.go
git commit -m "feat(db+api): group_hotels+documents+uploads+generate — tenant-scoped"
```

---

## Phase G — Public Slug Handlers

### Task 19: `/api/public/*`

**Files:**
- Create: `backend/internal/api/handlers_public.go`

- [ ] **Step 1: Implement**

Create `backend/internal/api/handlers_public.go`:

```go
package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"fujitravel-admin/backend/internal/consent"
	"fujitravel-admin/backend/internal/db"
)

// PublicOrg handles GET /api/public/org/:slug. Returns minimal org info
// (name only — do not leak id/email/created_at).
func PublicOrg(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		org, err := db.GetOrganizationBySlug(r.Context(), pool, slug)
		if err != nil {
			slog.Error("public org lookup", "err", err)
			writeError(w, 500, "db")
			return
		}
		if org == nil {
			writeError(w, 404, "form not found")
			return
		}
		writeJSON(w, 200, map[string]string{"name": org.Name})
	}
}

// PublicSubmit handles POST /api/public/submissions/:slug.
// Unauthenticated. Resolves slug → org_id and stores the submission.
func PublicSubmit(pool *pgxpool.Pool) http.HandlerFunc {
	type req struct {
		Payload         map[string]any `json:"payload"`
		ConsentAccepted bool           `json:"consent_accepted"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		org, err := db.GetOrganizationBySlug(r.Context(), pool, slug)
		if err != nil {
			writeError(w, 500, "db")
			return
		}
		if org == nil {
			writeError(w, 404, "form not found")
			return
		}

		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, 400, "invalid JSON")
			return
		}
		if !body.ConsentAccepted {
			writeError(w, 400, "consent not accepted")
			return
		}

		var missing []string
		for _, k := range requiredPayloadKeys {
			v, ok := body.Payload[k].(string)
			if !ok || v == "" {
				missing = append(missing, k)
			}
		}
		if len(missing) > 0 {
			writeErrorWithDetails(w, 400, "missing fields", map[string]any{"missing": missing})
			return
		}

		payloadBytes, _ := json.Marshal(body.Payload)
		agreement := consent.Current()

		id, err := db.CreateSubmissionForOrg(r.Context(), pool, org.ID, payloadBytes, agreement.Version, "tourist")
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeError(w, 409, "duplicate submission")
				return
			}
			slog.Error("create submission", "err", err)
			writeError(w, 500, "db")
			return
		}
		writeJSON(w, 201, map[string]string{"id": id})
	}
}
```

- [ ] **Step 2: Build + commit**

```bash
go build ./...
git add backend/internal/api/handlers_public.go
git commit -m "feat(api): public slug endpoints — GET /api/public/org/:slug + POST /api/public/submissions/:slug"
```

---

## Phase H — Route Wiring

### Task 20: Rewire `main.go`

**Files:**
- Modify: `backend/cmd/server/main.go`

- [ ] **Step 1: Rewrite routing**

Replace the `r.Route("/api", ...)` block with this split:

```go
import "fujitravel-admin/backend/internal/middleware"
// (also import time for rate-limit windows)

// Require APP_SECRET env (fail loud if missing — see Section 8.5)
appSecret := os.Getenv("APP_SECRET")
if appSecret == "" {
    slog.Error("APP_SECRET environment variable is required")
    os.Exit(1)
}

// Rate limiters
registerRL := middleware.NewRateLimiter(5, 15*time.Minute)
loginRL    := middleware.NewRateLimiter(10, 15*time.Minute)
publicRL   := middleware.NewRateLimiter(3, 10*time.Minute)

r.Route("/api", func(r chi.Router) {
    // Health check (public, for docker-healthcheck)
    r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
        if err := pool.Ping(req.Context()); err != nil {
            w.WriteHeader(503)
            return
        }
        w.WriteHeader(200)
    })

    // Public — auth
    r.With(registerRL.Middleware()).Post("/auth/register", api.Register(pool))
    r.With(loginRL.Middleware()).Post("/auth/login",       api.Login(pool))

    // Public — slug form
    r.Get("/public/org/{slug}",                                     api.PublicOrg(pool))
    r.With(publicRL.Middleware()).Post("/public/submissions/{slug}", api.PublicSubmit(pool))

    // Public — consent
    r.Get("/consent/text", api.GetConsentText())

    // Protected — everything else
    r.Group(func(r chi.Router) {
        r.Use(middleware.RequireAuth(pool))

        r.Post("/auth/logout", api.Logout(pool))
        r.Get("/auth/me",      api.Me(pool))

        // Hotels
        r.Get("/hotels",       api.ListHotels(pool))
        r.Post("/hotels",      api.CreateHotel(pool))
        r.Get("/hotels/{id}",  api.GetHotel(pool))
        r.Put("/hotels/{id}",  api.UpdateHotel(pool))

        // Groups
        r.Get("/groups",              api.ListGroups(pool))
        r.Post("/groups",             api.CreateGroup(pool))
        r.Get("/groups/{id}",         api.GetGroup(pool))
        r.Delete("/groups/{id}",      api.DeleteGroup(pool))
        r.Put("/groups/{id}/status",  api.UpdateGroupStatus(pool))
        r.Put("/groups/{id}/notes",   api.UpdateGroupNotes(pool))

        // Subgroups
        r.Get("/groups/{id}/subgroups",  api.ListSubgroups(pool, uploadsDir))
        r.Post("/groups/{id}/subgroups", api.CreateSubgroup(pool))
        r.Put("/subgroups/{id}",         api.UpdateSubgroup(pool))
        r.Delete("/subgroups/{id}",      api.DeleteSubgroup(pool))
        r.Put("/tourists/{id}/subgroup", api.AssignTouristSubgroup(pool))
        r.Get("/subgroups/{id}/hotels",  api.ListSubgroupHotels(pool))
        r.Post("/subgroups/{id}/hotels", api.UpsertSubgroupHotels(pool))
        r.Post("/subgroups/{id}/generate",
            api.GenerateSubgroupDocuments(pool, anthropicKey, uploadsDir, pythonScript))
        r.Get("/subgroups/{id}/download", api.DownloadSubgroupZIP(pool, uploadsDir))

        // Tourists
        r.Get("/groups/{id}/tourists",     api.ListTourists(pool))
        r.Delete("/tourists/{id}",         api.DeleteTourist(pool))
        r.Get("/tourists/{id}/uploads",    api.ListTouristUploads(pool))
        r.Post("/tourists/{id}/uploads",   api.UploadTouristFile(pool, uploadsDir, anthropicKey))
        r.Put("/tourists/{id}/flight_data", api.UpdateFlightData(pool))

        // Group hotels
        r.Get("/groups/{id}/hotels",  api.ListGroupHotels(pool))
        r.Post("/groups/{id}/hotels", api.UpsertGroupHotels(pool))

        // Submissions (admin)
        r.Get("/submissions",                  api.ListSubmissions(pool))
        r.Get("/submissions/{id}",             api.GetSubmission(pool))
        r.Put("/submissions/{id}",             api.UpdateSubmission(pool))
        r.Delete("/submissions/{id}",          api.ArchiveSubmission(pool))
        r.Delete("/submissions/{id}/erase",    api.EraseSubmission(pool))
        r.Post("/submissions/{id}/attach",     api.AttachSubmission(pool))
        // Manager create-manually uses the same protected POST /submissions
        r.Post("/submissions",                 api.CreateSubmissionByManager(pool))

        // Document generation
        r.Post("/groups/{id}/generate",
            api.GenerateDocuments(pool, anthropicKey, uploadsDir, pythonScript))
        r.Post("/groups/{id}/finalize",
            api.FinalizeGroup(pool, anthropicKey, uploadsDir, pythonScript))
        r.Get("/groups/{id}/documents",        api.GetDocuments(pool))
        r.Get("/groups/{id}/download",         api.DownloadZIP(pool))
        r.Get("/groups/{id}/download/final",   api.DownloadFinalZIP(uploadsDir))
        r.Get("/groups/{id}/final/status",     api.FinalStatus(uploadsDir))
    })
})
```

Add `CreateSubmissionByManager` in `backend/internal/api/submissions.go` — same as the old public `CreateSubmission` but reads `orgID` from `middleware.OrgID` and sets `source = "manager"`.

- [ ] **Step 2: Build + smoke-test**

```bash
go build ./...
# Smoke: start backend against throwaway DB
docker run --rm -d --name fuji-smoke -e POSTGRES_USER=fuji \
  -e POSTGRES_PASSWORD=fuji123 -e POSTGRES_DB=fujitravel \
  -p 5497:5432 postgres:16
sleep 3
export DATABASE_URL="postgres://fuji:fuji123@localhost:5497/fujitravel?sslmode=disable"
export ANTHROPIC_API_KEY="sk-ant-..."   # any value — we won't hit it
export APP_SECRET="$(openssl rand -base64 32)"
cd backend && go run cmd/server/main.go &
SERVER_PID=$!
sleep 2
curl -s http://localhost:8080/api/health  # expect 200
curl -s http://localhost:8080/api/groups  # expect 401 {"error":"not authenticated"}
curl -s -X POST http://localhost:8080/api/auth/register \
     -H 'Content-Type: application/json' \
     -d '{"org_name":"Test","email":"a@b.com","password":"password123"}' -c cookies.txt
curl -s -b cookies.txt http://localhost:8080/api/groups  # expect [] (200)
kill $SERVER_PID
docker stop fuji-smoke
```

- [ ] **Step 3: Commit**

```bash
git add backend/cmd/server/main.go backend/internal/api/submissions.go
git commit -m "refactor(server): wire public/protected route groups + rate limits"
```

---

## Phase I — Frontend Auth

### Task 21: `apiFetch` wrapper + auth client methods

**Files:**
- Modify: `frontend/src/api/client.js`

- [ ] **Step 1: Add apiFetch**

At the top of `frontend/src/api/client.js`, replace any existing raw-`fetch` patterns with an `apiFetch` wrapper:

```js
const API = '/api'

async function apiFetch(path, opts = {}) {
  const res = await fetch(API + path, {
    credentials: 'include',
    ...opts,
    headers: {
      ...(opts.body ? { 'Content-Type': 'application/json' } : {}),
      ...(opts.headers || {}),
    },
  })
  if (res.status === 401 && !path.startsWith('/auth/') && !path.startsWith('/public/')) {
    window.location.href = '/login'
    throw new Error('unauthenticated')
  }
  return res
}

async function errFromRes(res) {
  try {
    const data = await res.json()
    return new Error(data.error || 'request failed')
  } catch {
    return new Error(`${res.status} ${res.statusText}`)
  }
}
```

Refactor every existing exported function to use `apiFetch` instead of `fetch`. Rename prefix: paths no longer start with `/api`, but with `/groups`, `/submissions`, etc.

- [ ] **Step 2: Add auth functions**

Append to `frontend/src/api/client.js`:

```js
// ── Auth ──
export async function apiRegister(orgName, email, password) {
  const res = await apiFetch('/auth/register', {
    method: 'POST',
    body: JSON.stringify({ org_name: orgName, email, password }),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function apiLogin(email, password) {
  const res = await apiFetch('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function apiLogout() {
  const res = await apiFetch('/auth/logout', { method: 'POST' })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function apiMe() {
  const res = await apiFetch('/auth/me')
  if (res.status === 401) return null
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

// ── Public (no session) ──
export async function publicGetOrg(slug) {
  const res = await fetch(`${API}/public/org/${slug}`)
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}

export async function publicCreateSubmission(slug, payload, consentAccepted) {
  const res = await fetch(`${API}/public/submissions/${slug}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ payload, consent_accepted: consentAccepted }),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}
```

- [ ] **Step 3: Remove old `createSubmission`**

The old `createSubmission` (public, unauthenticated) is removed — it's replaced by `publicCreateSubmission(slug, ...)`. Callers inside the admin ("Create manually" flow) use a new `apiCreateSubmission` that hits the protected POST `/submissions`:

```js
export async function apiCreateSubmission(payload, consentAccepted) {
  const res = await apiFetch('/submissions', {
    method: 'POST',
    body: JSON.stringify({ payload, consent_accepted: consentAccepted, source: 'manager' }),
  })
  if (!res.ok) throw await errFromRes(res)
  return res.json()
}
```

Update any caller that used the old `createSubmission` with `'manager'` source.

- [ ] **Step 4: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/api/client.js
git commit -m "feat(frontend): apiFetch wrapper + auth/public API methods"
```

---

### Task 22: AuthContext + AuthProvider

**Files:**
- Create: `frontend/src/auth/AuthContext.jsx`
- Create: `frontend/src/auth/RequireAuth.jsx`

- [ ] **Step 1: AuthContext**

Create `frontend/src/auth/AuthContext.jsx`:

```jsx
import { createContext, useContext, useEffect, useState, useCallback } from 'react'
import { apiLogin, apiLogout, apiMe, apiRegister } from '../api/client'

const AuthContext = createContext(null)

export function AuthProvider({ children }) {
  const [state, setState] = useState({ loading: true, user: null, org: null })

  const refresh = useCallback(async () => {
    try {
      const data = await apiMe()
      if (!data) {
        setState({ loading: false, user: null, org: null })
      } else {
        setState({ loading: false, user: data.user, org: data.org })
      }
    } catch {
      setState({ loading: false, user: null, org: null })
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  const login = async (email, password) => {
    const data = await apiLogin(email, password)
    setState({ loading: false, user: data.user, org: data.org })
  }

  const register = async (orgName, email, password) => {
    const data = await apiRegister(orgName, email, password)
    setState({ loading: false, user: data.user, org: data.org })
  }

  const logout = async () => {
    await apiLogout()
    setState({ loading: false, user: null, org: null })
  }

  return (
    <AuthContext.Provider value={{ ...state, login, logout, register, refresh }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used inside <AuthProvider>')
  return ctx
}
```

- [ ] **Step 2: RequireAuth**

Create `frontend/src/auth/RequireAuth.jsx`:

```jsx
import { Navigate, useLocation } from 'react-router-dom'
import { useAuth } from './AuthContext'

export default function RequireAuth({ children }) {
  const { loading, user } = useAuth()
  const location = useLocation()
  if (loading) return <div className="auth-loading">Загрузка…</div>
  if (!user) return <Navigate to="/login" state={{ from: location }} replace />
  return children
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/auth
git commit -m "feat(frontend): AuthContext + RequireAuth wrapper"
```

---

### Task 23: LoginPage

**Files:**
- Create: `frontend/src/pages/LoginPage.jsx`

- [ ] **Step 1: Implement**

Create `frontend/src/pages/LoginPage.jsx`:

```jsx
import { useState } from 'react'
import { useLocation, useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

export default function LoginPage() {
  const { login } = useAuth()
  const nav = useNavigate()
  const location = useLocation()
  const from = location.state?.from?.pathname || '/groups'

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e) => {
    e.preventDefault()
    setErr(null)
    setBusy(true)
    try {
      await login(email.trim(), password)
      nav(from, { replace: true })
    } catch (e) {
      setErr(e.message || 'Неверный email или пароль')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="auth-page">
      <h1>Вход в систему</h1>
      <form onSubmit={submit} className="auth-form">
        <label>
          <span>Email</span>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)}
                 required autoFocus autoComplete="email" />
        </label>
        <label>
          <span>Пароль</span>
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)}
                 required autoComplete="current-password" />
        </label>
        {err && <div className="auth-error">{err}</div>}
        <button type="submit" disabled={busy}>{busy ? 'Проверка…' : 'Войти'}</button>
      </form>
      <p className="auth-link">Нет аккаунта? <Link to="/register">Зарегистрировать турфирму</Link></p>
      <p className="auth-hint">
        Забыли пароль? Свяжитесь с администратором: <a href="mailto:tour@fujitravel.ru">tour@fujitravel.ru</a>
      </p>
    </main>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/pages/LoginPage.jsx
git commit -m "feat(frontend): /login page"
```

---

### Task 24: RegisterPage

**Files:**
- Create: `frontend/src/pages/RegisterPage.jsx`

- [ ] **Step 1: Implement**

Create `frontend/src/pages/RegisterPage.jsx`:

```jsx
import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

export default function RegisterPage() {
  const { register } = useAuth()
  const nav = useNavigate()
  const [orgName, setOrgName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [err, setErr] = useState(null)
  const [busy, setBusy] = useState(false)

  const valid =
    orgName.trim().length > 0 &&
    /\S+@\S+\.\S+/.test(email) &&
    password.length >= 8 &&
    password === confirm

  const submit = async (e) => {
    e.preventDefault()
    if (!valid) return
    setErr(null)
    setBusy(true)
    try {
      await register(orgName.trim(), email.trim(), password)
      nav('/groups', { replace: true })
    } catch (e) {
      setErr(e.message || 'Ошибка регистрации')
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="auth-page">
      <h1>Регистрация турфирмы</h1>
      <form onSubmit={submit} className="auth-form">
        <label>
          <span>Название турфирмы</span>
          <input value={orgName} onChange={(e) => setOrgName(e.target.value)} required autoFocus />
        </label>
        <label>
          <span>Email</span>
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)}
                 required autoComplete="email" />
        </label>
        <label>
          <span>Пароль (минимум 8 символов)</span>
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)}
                 required minLength={8} autoComplete="new-password" />
        </label>
        <label>
          <span>Подтверждение пароля</span>
          <input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)}
                 required minLength={8} autoComplete="new-password" />
          {confirm && confirm !== password && <span className="auth-error">Пароли не совпадают</span>}
        </label>
        {err && <div className="auth-error">{err}</div>}
        <button type="submit" disabled={!valid || busy}>
          {busy ? 'Создаём аккаунт…' : 'Зарегистрировать'}
        </button>
      </form>
      <p className="auth-link">Уже есть аккаунт? <Link to="/login">Войти</Link></p>
    </main>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/pages/RegisterPage.jsx
git commit -m "feat(frontend): /register page"
```

---

### Task 25: App.jsx routing + AuthProvider

**Files:**
- Modify: `frontend/src/App.jsx`

- [ ] **Step 1: Wrap and add routes**

Open `frontend/src/App.jsx`. Wrap the whole router tree in `<AuthProvider>`. Add public auth routes (`/login`, `/register`) and wrap admin routes with `<RequireAuth>`.

Shape:

```jsx
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from './auth/AuthContext'
import RequireAuth from './auth/RequireAuth'
import LoginPage from './pages/LoginPage'
import RegisterPage from './pages/RegisterPage'
// ...existing imports

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <Routes>
          {/* Public */}
          <Route path="/login"        element={<LoginPage />} />
          <Route path="/register"     element={<RegisterPage />} />
          <Route path="/form/:slug"   element={<SubmissionFormPage />} />
          <Route path="/form/thanks"  element={<FormThanksPage />} />
          <Route path="/form"         element={<PublicFormFallbackPage />} />
          <Route path="/consent"      element={<ConsentPage />} />

          {/* Protected admin */}
          <Route path="/*" element={
            <RequireAuth>
              <AdminShell>
                <Routes>
                  <Route path="/groups"          element={<GroupsPage />} />
                  <Route path="/groups/:id"      element={<GroupDetailPage />} />
                  <Route path="/submissions"     element={<SubmissionsListPage />} />
                  <Route path="/submissions/:id" element={<SubmissionDetailPage />} />
                  <Route path="/hotels"          element={<HotelsPage />} />
                  <Route path="/hotels/:id"      element={<HotelEditPage />} />
                  <Route path="*"                element={<Navigate to="/groups" replace />} />
                </Routes>
              </AdminShell>
            </RequireAuth>
          } />
        </Routes>
      </BrowserRouter>
    </AuthProvider>
  )
}
```

Create `PublicFormFallbackPage.jsx`:

```jsx
export default function PublicFormFallbackPage() {
  return (
    <main className="auth-page">
      <h1>Для заполнения анкеты</h1>
      <p>Обратитесь к вашей турфирме за индивидуальной ссылкой вида <code>/form/&lt;код&gt;</code>.</p>
    </main>
  )
}
```

- [ ] **Step 2: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/App.jsx frontend/src/pages/PublicFormFallbackPage.jsx
git commit -m "feat(frontend): wire AuthProvider + public/protected routes"
```

---

## Phase J — Frontend Slug Form + Admin Header

### Task 26: SubmissionFormPage slug-aware

**Files:**
- Modify: `frontend/src/pages/SubmissionFormPage.jsx`

- [ ] **Step 1: Rewrite**

Replace `frontend/src/pages/SubmissionFormPage.jsx` contents:

```jsx
import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import SubmissionForm from '../components/SubmissionForm'
import { publicGetOrg, publicCreateSubmission } from '../api/client'

export default function SubmissionFormPage() {
  const { slug } = useParams()
  const nav = useNavigate()
  const [orgName, setOrgName] = useState(null)
  const [loadErr, setLoadErr] = useState(null)

  useEffect(() => {
    publicGetOrg(slug)
      .then((data) => setOrgName(data.name))
      .catch(() => setLoadErr('Ссылка недействительна или устарела. Обратитесь к менеджеру.'))
  }, [slug])

  const handleSubmit = async (payload, consent) => {
    await publicCreateSubmission(slug, payload, consent)
    nav('/form/thanks', { replace: true })
  }

  if (loadErr) {
    return (
      <main className="public-form">
        <h1>Ошибка</h1>
        <p>{loadErr}</p>
      </main>
    )
  }
  if (!orgName) return <main className="public-form"><h1>Загрузка…</h1></main>

  return (
    <main className="public-form">
      <h1>Анкета на визу в Японию</h1>
      <p className="lead">Для турфирмы: <strong>{orgName}</strong></p>
      <SubmissionForm onSubmit={handleSubmit} />
    </main>
  )
}
```

- [ ] **Step 2: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/pages/SubmissionFormPage.jsx
git commit -m "feat(frontend): SubmissionFormPage reads slug + uses public API"
```

---

### Task 27: AdminShell header with org name + logout

**Files:**
- Modify: `frontend/src/components/AdminShell.jsx` (or equivalent layout)
- Create: `frontend/src/components/CopyFormLinkButton.jsx`

- [ ] **Step 1: CopyFormLinkButton**

Create `frontend/src/components/CopyFormLinkButton.jsx`:

```jsx
import { useState } from 'react'
import { useAuth } from '../auth/AuthContext'

export default function CopyFormLinkButton() {
  const { org } = useAuth()
  const [copied, setCopied] = useState(false)
  const url = `${window.location.origin}/form/${org.slug}`

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      prompt('Скопируйте ссылку:', url)
    }
  }

  return (
    <button onClick={copy} className="btn-outline">
      {copied ? '✓ Скопировано' : '📎 Ссылка на анкету'}
    </button>
  )
}
```

- [ ] **Step 2: AdminShell update**

In `frontend/src/components/AdminShell.jsx` (or wherever the layout lives), add a header row that shows:
- Org name + slug
- User email
- Logout button
- CopyFormLinkButton

Example fragment:

```jsx
import { useAuth } from '../auth/AuthContext'
import CopyFormLinkButton from './CopyFormLinkButton'

function AdminShellHeader() {
  const { user, org, logout } = useAuth()
  return (
    <header className="admin-shell-header">
      <div className="org-info">
        <strong>{org.name}</strong>
        <span className="muted">/{org.slug}</span>
      </div>
      <div className="shell-actions">
        <CopyFormLinkButton />
        <span className="user-email">{user.email}</span>
        <button onClick={logout} className="btn-outline">Выйти</button>
      </div>
    </header>
  )
}

// Then render <AdminShellHeader /> at the top of AdminShell's JSX.
```

- [ ] **Step 3: Append CSS**

Append to `frontend/src/index.css`:

```css
/* ── Auth pages ─────────────────────────────────────────────── */
.auth-page {
  max-width: 420px;
  margin: 5rem auto;
  padding: 1.5rem;
  background: var(--card, rgba(255,255,255,0.02));
  border: 1px solid var(--border, rgba(255,255,255,0.12));
  border-radius: 10px;
}
.auth-form { display: flex; flex-direction: column; gap: 1rem; }
.auth-form label { display: flex; flex-direction: column; gap: 0.25rem; }
.auth-error { color: var(--danger, #e04a4a); font-size: 0.9rem; }
.auth-link, .auth-hint { text-align: center; margin-top: 1rem; }
.auth-loading { text-align: center; margin-top: 4rem; opacity: 0.6; }

/* ── Admin shell header ──────────────────────────────────────── */
.admin-shell-header {
  display: flex; align-items: center; justify-content: space-between;
  padding: 0.75rem 1.5rem;
  border-bottom: 1px solid var(--border, rgba(255,255,255,0.08));
  gap: 1rem;
}
.org-info { display: flex; align-items: baseline; gap: 0.5rem; }
.org-info .muted { opacity: 0.6; font-family: var(--font-mono); font-size: 0.85rem; }
.shell-actions { display: flex; align-items: center; gap: 1rem; }
.user-email { opacity: 0.8; font-size: 0.9rem; }
```

- [ ] **Step 4: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/components/AdminShell.jsx \
        frontend/src/components/CopyFormLinkButton.jsx \
        frontend/src/index.css
git commit -m "feat(frontend): AdminShell header with org + logout + copy-link"
```

---

### Task 28: Remove stale /form (no-slug) defaults in admin

**Files:**
- Modify: `frontend/src/pages/SubmissionDetailPage.jsx`

- [ ] **Step 1: Fix "Create manually" flow**

The admin-side "Create manually" submission must go through the **protected** endpoint, not the public one. Replace `createSubmission` call with `apiCreateSubmission`:

```jsx
import { apiCreateSubmission } from '../api/client'
// ...
const handleSubmit = async (payload, consent) => {
  if (isNew) {
    const data = await apiCreateSubmission(payload, consent)
    nav(`/submissions/${data.id}`)
  } else {
    await apiUpdateSubmission(id, payload)
    // ...
  }
}
```

Also any other component that used the old `createSubmission` export.

- [ ] **Step 2: Build + commit**

```bash
cd frontend && npm run build
git add frontend/src/pages/SubmissionDetailPage.jsx
git commit -m "fix(frontend): admin create-manually uses protected POST /submissions"
```

---

## Phase K — Integration Tests

### Task 29: Cross-org isolation integration test

**Files:**
- Create: `backend/integration_test.go`

- [ ] **Step 1: Write test**

Create `backend/integration_test.go`:

```go
//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	// NOTE: adjust import path if main.go wires router differently.
)

func registerOrg(t *testing.T, srv *httptest.Server, orgName, email, password string) *http.Client {
	t.Helper()
	jar, _ := http.CookieJarNew()
	client := &http.Client{Jar: jar}
	body, _ := json.Marshal(map[string]string{
		"org_name": orgName, "email": email, "password": password,
	})
	resp, err := client.Post(srv.URL+"/api/auth/register", "application/json", bytes.NewReader(body))
	if err != nil { t.Fatal(err) }
	if resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("register %s: %d — %s", orgName, resp.StatusCode, b)
	}
	return client
}

func TestCrossOrgIsolation(t *testing.T) {
	if os.Getenv("POSTGRES_HOST") == "" {
		t.Skip("integration test requires POSTGRES_HOST")
	}
	dsn := fmt.Sprintf("postgres://fuji:fuji123@%s:%s/fujitravel?sslmode=disable",
		os.Getenv("POSTGRES_HOST"), os.Getenv("POSTGRES_PORT"))
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil { t.Fatal(err) }
	defer pool.Close()

	// Start server against this DB
	srv := startTestServer(t, pool)
	defer srv.Close()

	c1 := registerOrg(t, srv, "Org One",  "o1@test.com", "password123")
	c2 := registerOrg(t, srv, "Org Two",  "o2@test.com", "password123")

	// Org1 creates a group
	body, _ := json.Marshal(map[string]string{"name": "SecretGroup"})
	resp, _ := c1.Post(srv.URL+"/api/groups", "application/json", bytes.NewReader(body))
	if resp.StatusCode != 201 { t.Fatalf("create group: %d", resp.StatusCode) }
	var g struct{ ID string `json:"id"` }
	_ = json.NewDecoder(resp.Body).Decode(&g)

	// Org2 list
	resp, _ = c2.Get(srv.URL + "/api/groups")
	var groups []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&groups)
	if len(groups) != 0 {
		t.Errorf("org2 sees org1's groups: %v", groups)
	}

	// Org2 direct access → 404
	resp, _ = c2.Get(srv.URL + "/api/groups/" + g.ID)
	if resp.StatusCode != 404 {
		t.Errorf("org2 GET org1 group: %d, want 404", resp.StatusCode)
	}

	// Org2 delete → 404
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/groups/"+g.ID, nil)
	resp, _ = c2.Do(req)
	if resp.StatusCode != 404 {
		t.Errorf("org2 DELETE org1 group: %d, want 404", resp.StatusCode)
	}

	// Org1 still sees it
	resp, _ = c1.Get(srv.URL + "/api/groups/" + g.ID)
	if resp.StatusCode != 200 {
		t.Errorf("org1 GET its own group: %d, want 200", resp.StatusCode)
	}
}
```

You'll need a `startTestServer` helper that wires the same chi router as `main.go`. The simplest approach is to extract router construction from `main.go` into `backend/internal/server/router.go` and import it from both `main.go` and the test.

- [ ] **Step 2: Run**

```bash
docker run --rm -d --name fuji-it -e POSTGRES_USER=fuji \
  -e POSTGRES_PASSWORD=fuji123 -e POSTGRES_DB=fujitravel \
  -p 5496:5432 postgres:16
sleep 3
export POSTGRES_HOST=localhost POSTGRES_PORT=5496 \
       DATABASE_URL="postgres://fuji:fuji123@localhost:5496/fujitravel?sslmode=disable" \
       APP_SECRET="$(openssl rand -base64 32)" \
       ANTHROPIC_API_KEY=dummy
cd backend && go test -tags=integration ./... -run TestCrossOrgIsolation -v
docker stop fuji-it
```

- [ ] **Step 3: Commit**

```bash
git add backend/integration_test.go backend/internal/server/router.go
git commit -m "test: integration — cross-org isolation coverage"
```

---

## Phase L — Deploy + Docs

### Task 30: Deployment env vars + CI

**Files:**
- Modify: `docker-compose.prod.yml`
- Modify: `.env.example`
- Modify: `.github/workflows/deploy.yml`

- [ ] **Step 1: Compose + .env.example**

Add `APP_SECRET` and `APP_ENV` to `backend` service in `docker-compose.prod.yml`:

```yaml
backend:
  # ...
  environment:
    APP_SECRET: ${APP_SECRET}
    APP_ENV: production
    # (plus existing DATABASE_URL etc)
```

In `.env.example`:

```env
# Required
DATABASE_URL=postgres://fuji:fuji123@db:5432/fujitravel?sslmode=disable
ANTHROPIC_API_KEY=sk-ant-...
APP_SECRET=generate-with-openssl-rand-base64-32

# Optional
APP_ENV=production
UPLOADS_DIR=./uploads
PORT=8081
DOCGEN_SCRIPT=../../docgen/generate.py
DOCGEN_PDF_TEMPLATE=./templates/anketa_template.pdf
```

- [ ] **Step 2: CI test step**

Edit `.github/workflows/deploy.yml`, add a test job before deploy:

```yaml
jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_USER: fuji
          POSTGRES_PASSWORD: fuji123
          POSTGRES_DB: fujitravel
        ports: ['5432:5432']
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.25' }
      - run: cd backend && go test ./...
      - run: cd backend && go test -tags=integration ./...
        env:
          POSTGRES_HOST: localhost
          POSTGRES_PORT: 5432
          DATABASE_URL: postgres://fuji:fuji123@localhost:5432/fujitravel?sslmode=disable
          APP_SECRET: test-secret-for-ci
          ANTHROPIC_API_KEY: dummy
  deploy:
    needs: test
    # ... existing deploy steps
```

- [ ] **Step 3: Commit**

```bash
git add docker-compose.prod.yml .env.example .github/workflows/deploy.yml
git commit -m "chore(deploy): APP_SECRET env + CI test gate"
```

---

### Task 31: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Rewrite relevant sections**

Update `CLAUDE.md`:

- **Stack** row for Auth: add `| Auth | session cookies + argon2id, row-level tenancy |`
- Add section **SaaS model**:
  - Each travel agency registers via `/register` → receives org + first user.
  - Tourists get `/form/<org-slug>` URL; submissions land only in that org's pool.
  - Hotels split: global catalog (org_id IS NULL) + private per-org (org_id set).
- **Environment Variables** — add `APP_SECRET`, `APP_ENV`.
- **Data Flow** — prepend: "0. Agency registers or logs in at /register or /login."
- Add reminder: every new API handler MUST read `middleware.OrgID(r.Context())` and delegate SQL to `internal/db/*.go` functions with `orgID` as a parameter.

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md for SaaS multi-tenancy"
```

---

### Task 32: Full-stack manual QA

**Files:**
- None (QA only).

- [ ] **Step 1: Start the stack**

```bash
cd /Users/yaufdd/Desktop/fujitravel-admin
docker compose -f docker-compose.prod.yml down
docker compose -f docker-compose.prod.yml up -d --build
make logs-backend | head -20
# Expect "migrations applied" and "server starting"
```

- [ ] **Step 2: Manual QA checklist**

- [ ] `/register` — create "Test Agency" with `qa1@test.com / password123` → redirect to `/groups`
- [ ] Header shows "Test Agency" + org slug + email + Logout button
- [ ] `/groups` is empty; `/hotels` shows the global catalog
- [ ] Logout → redirect to `/login`
- [ ] Login with same credentials → back in `/groups`
- [ ] Click "📎 Ссылка на анкету" → open copied URL in incognito → see "Для турфирмы: Test Agency" and the form
- [ ] Submit form as tourist → see `/form/thanks`
- [ ] In admin `/submissions` — see the submission
- [ ] Register a second org "Other Agency" with `qa2@test.com` in another incognito window
- [ ] As `qa2`, `/submissions` is empty — cannot see `qa1`'s submission
- [ ] As `qa2`, try to open `qa1`'s group ID → 404
- [ ] Add a private hotel as `qa1`; it is NOT visible to `qa2`
- [ ] Global (seeded) hotels ARE visible to both orgs
- [ ] As `qa1`: add tourist from pool → enter flight data manually → generate documents → zip downloads with correct ФИО

- [ ] **Step 3: Stop (no commit needed)**

No git changes expected from this task. If bugs found, fix with `fix(...)` commits.

---

## Post-Plan Self-Check

Spec coverage:
- §3 Architecture → implemented across Phases C–H.
- §4 DB schema → Task 1.
- §5 Auth flow → Tasks 10–12 + middleware 8.
- §6 Middleware/scoping → Tasks 8, 13–18.
- §7 Public slug form → Tasks 19, 26.
- §8 Errors/security → embedded throughout (401/404 mapping, cookie flags, rate limits).
- §9 Testing → Task 29 + manual QA Task 32.
- §10 Migration/deploy → Task 30.
- §11 File inventory → new files listed per task.

Placeholder scan: no "TBD" / "TODO" / "similar to Task N" phrases remaining.

Type consistency:
- Repository functions consistently named `<Verb><Entity>(ctx, pool, orgID, ...)`.
- Middleware helper `middleware.OrgID(ctx)` used consistently.
- Session cookie name `session` consistent.
- `sessionTTL` constant reused.

Done.
