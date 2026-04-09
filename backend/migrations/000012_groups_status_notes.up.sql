-- Add notes column.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';

-- Drop the old CHECK first so we can rewrite legacy status values.
ALTER TABLE groups DROP CONSTRAINT IF EXISTS groups_status_check;

-- Backfill legacy status values.
UPDATE groups SET status = 'in_progress' WHERE status = 'processing';
UPDATE groups SET status = 'docs_ready'  WHERE status IN ('completed', 'ready');

-- Install the new canonical CHECK.
ALTER TABLE groups
    ADD CONSTRAINT groups_status_check
    CHECK (status IN ('draft', 'in_progress', 'docs_ready', 'submitted', 'visa_issued'));
