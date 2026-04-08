ALTER TABLE uploads ADD COLUMN IF NOT EXISTS anthropic_file_id TEXT;
ALTER TABLE uploads ALTER COLUMN file_type DROP NOT NULL;
ALTER TABLE uploads ALTER COLUMN file_type SET DEFAULT 'document';
