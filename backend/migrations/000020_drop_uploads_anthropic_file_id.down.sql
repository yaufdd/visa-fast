-- Re-add uploads.anthropic_file_id for rollback. Re-creates the same
-- nullable TEXT column shape the original 000008 migration introduced.
ALTER TABLE uploads ADD COLUMN IF NOT EXISTS anthropic_file_id TEXT;
