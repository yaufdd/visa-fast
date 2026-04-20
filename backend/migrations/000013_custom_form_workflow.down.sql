ALTER TABLE uploads DROP CONSTRAINT IF EXISTS uploads_file_type_check;

ALTER TABLE tourists
  DROP COLUMN IF EXISTS translations,
  DROP COLUMN IF EXISTS flight_data,
  DROP COLUMN IF EXISTS submission_snapshot,
  DROP COLUMN IF EXISTS submission_id;

ALTER TABLE tourists
  ADD COLUMN raw_json JSONB,
  ADD COLUMN matched_sheet_row JSONB,
  ADD COLUMN match_confirmed BOOLEAN DEFAULT FALSE;

DROP TABLE IF EXISTS tourist_submissions;
