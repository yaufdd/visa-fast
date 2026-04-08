CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE groups (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    status     TEXT NOT NULL DEFAULT 'draft'
                   CHECK (status IN ('draft', 'processing', 'ready', 'completed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_groups_status     ON groups (status);
CREATE INDEX idx_groups_created_at ON groups (created_at);
