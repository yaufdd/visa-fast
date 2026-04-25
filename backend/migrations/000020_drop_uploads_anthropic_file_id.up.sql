-- Drop uploads.anthropic_file_id: every AI call now flows through Yandex
-- Cloud (Task 1.D1) and the Anthropic Files API is no longer used. The
-- column was write-never, read-never after Tasks 1.B2 / 1.C1 / 1.C2; this
-- migration removes the dead schema.
ALTER TABLE uploads DROP COLUMN IF EXISTS anthropic_file_id;
