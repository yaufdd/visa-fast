ALTER TABLE uploads ADD COLUMN parsed_at TIMESTAMPTZ;

-- Treat all historical uploads as already-parsed: the previous flow auto-parsed
-- every upload synchronously, so the "Распознать" button should not appear
-- for rows that were already run through the AI on creation.
UPDATE uploads SET parsed_at = created_at WHERE parsed_at IS NULL;
