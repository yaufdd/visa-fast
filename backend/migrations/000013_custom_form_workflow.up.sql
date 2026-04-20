-- Pool of form submissions (the "tourist list" the manager picks from)
CREATE TABLE tourist_submissions (
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

CREATE INDEX idx_submissions_status   ON tourist_submissions(status);
CREATE INDEX idx_submissions_created  ON tourist_submissions(created_at DESC);
CREATE INDEX idx_submissions_name_lat ON tourist_submissions ((payload ->> 'name_lat'));

-- Dedup guard: same foreign passport submitted twice on the same day while pending
CREATE UNIQUE INDEX idx_submissions_dedup ON tourist_submissions
  ((payload ->> 'passport_number'), ((created_at AT TIME ZONE 'UTC')::DATE))
  WHERE status = 'pending';

-- Drop legacy scan/sheet columns on tourists
ALTER TABLE tourists
  DROP COLUMN IF EXISTS raw_json,
  DROP COLUMN IF EXISTS matched_sheet_row,
  DROP COLUMN IF EXISTS match_confirmed;

-- Add new columns
ALTER TABLE tourists
  ADD COLUMN submission_id       UUID REFERENCES tourist_submissions(id) ON DELETE SET NULL,
  ADD COLUMN submission_snapshot JSONB,
  ADD COLUMN flight_data         JSONB,
  ADD COLUMN translations        JSONB;

-- Clean legacy passport-scan upload rows before tightening file_type constraint
DELETE FROM uploads WHERE file_type NOT IN ('ticket', 'voucher');

ALTER TABLE uploads
  ADD CONSTRAINT uploads_file_type_check
  CHECK (file_type IN ('ticket', 'voucher'));
