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
