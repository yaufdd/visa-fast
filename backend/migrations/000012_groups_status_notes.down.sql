-- Revert status CHECK constraint back to the original set.
ALTER TABLE groups DROP CONSTRAINT IF EXISTS groups_status_check;

-- Map new values back to something legal for the old constraint.
UPDATE groups SET status = 'processing' WHERE status = 'in_progress';
UPDATE groups SET status = 'completed'  WHERE status IN ('docs_ready', 'submitted', 'visa_issued');

ALTER TABLE groups
    ADD CONSTRAINT groups_status_check
    CHECK (status IN ('draft', 'processing', 'ready', 'completed'));

ALTER TABLE groups DROP COLUMN IF EXISTS notes;
