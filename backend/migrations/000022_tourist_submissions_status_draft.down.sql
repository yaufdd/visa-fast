-- Restore the original four-state CHECK. We do NOT migrate any 'draft'
-- rows here on purpose: the constraint will simply reject them at apply
-- time, which is the desired loud failure if a downgrade is attempted with
-- live drafts present. Operators must triage drafts (delete / promote)
-- before downgrading.
ALTER TABLE tourist_submissions DROP CONSTRAINT IF EXISTS tourist_submissions_status_check;
ALTER TABLE tourist_submissions
  ADD CONSTRAINT tourist_submissions_status_check
  CHECK (status IN ('pending','attached','archived','deleted'));
