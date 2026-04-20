-- Pool of form submissions (the "tourist list" the manager picks from)
CREATE TABLE IF NOT EXISTS tourist_submissions (
  id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  payload               JSONB NOT NULL,
  consent_accepted      BOOLEAN NOT NULL,
  consent_accepted_at   TIMESTAMPTZ NOT NULL,
  consent_version       TEXT NOT NULL,
  source                TEXT NOT NULL CHECK (source IN ('tourist', 'manager')),
  status                TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'attached', 'archived', 'deleted')),
  created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tourist_submissions_status   ON tourist_submissions(status);
CREATE INDEX IF NOT EXISTS idx_tourist_submissions_created  ON tourist_submissions(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tourist_submissions_name_lat ON tourist_submissions ((payload ->> 'name_lat'));

-- Dedup guard: same foreign passport submitted twice on the same day while pending
CREATE UNIQUE INDEX IF NOT EXISTS idx_tourist_submissions_dedup ON tourist_submissions
  ((payload ->> 'passport_number'), ((created_at AT TIME ZONE 'UTC')::DATE))
  WHERE status = 'pending';

-- Drop legacy scan/sheet columns on tourists
ALTER TABLE tourists
  DROP COLUMN IF EXISTS raw_json,
  DROP COLUMN IF EXISTS matched_sheet_row,
  DROP COLUMN IF EXISTS match_confirmed;

-- Add new columns
ALTER TABLE tourists
  ADD COLUMN IF NOT EXISTS submission_id       UUID REFERENCES tourist_submissions(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS submission_snapshot JSONB,
  ADD COLUMN IF NOT EXISTS flight_data         JSONB,
  ADD COLUMN IF NOT EXISTS translations        JSONB;

-- Pre-launch cleanup: drop legacy passport-scan upload rows before tightening the
-- file_type constraint. This is safe ONLY because the feature has not launched yet
-- and these rows are test data. Do NOT copy this DELETE into any post-launch
-- migration without a proper data-migration plan.
DELETE FROM uploads WHERE file_type NOT IN ('ticket', 'voucher');

ALTER TABLE uploads DROP CONSTRAINT IF EXISTS uploads_file_type_check;
ALTER TABLE uploads
  ADD CONSTRAINT uploads_file_type_check
  CHECK (file_type IN ('ticket', 'voucher'));
