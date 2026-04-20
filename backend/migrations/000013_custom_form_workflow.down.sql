ALTER TABLE uploads DROP CONSTRAINT IF EXISTS uploads_file_type_check;

-- Restore the original uploads.file_type constraint from migration 000009 so that
-- `down` leaves the schema identical to the pre-000013 state.
ALTER TABLE uploads ADD CONSTRAINT uploads_file_type_check
    CHECK (file_type IN ('passport', 'foreign_passport', 'ticket', 'voucher', 'unknown', 'document'));

ALTER TABLE tourists
  DROP COLUMN IF EXISTS translations,
  DROP COLUMN IF EXISTS flight_data,
  DROP COLUMN IF EXISTS submission_snapshot,
  DROP COLUMN IF EXISTS submission_id;

ALTER TABLE tourists
  ADD COLUMN IF NOT EXISTS raw_json JSONB,
  ADD COLUMN IF NOT EXISTS matched_sheet_row JSONB,
  ADD COLUMN IF NOT EXISTS match_confirmed BOOLEAN DEFAULT FALSE;

DROP TABLE IF EXISTS tourist_submissions;
