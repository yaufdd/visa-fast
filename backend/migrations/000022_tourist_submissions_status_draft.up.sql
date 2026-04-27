-- Allow 'draft' status for tourist_submissions rows.
--
-- A draft is a placeholder created by the public form *before* the tourist
-- finalises the submission, so they can attach passport / ticket / voucher
-- scans (linked via submission_files.submission_id) to a row that doesn't
-- yet have a complete payload. The finalize step (POST /api/public/submissions/{slug})
-- flips the row to 'pending'.
--
-- The dedup unique index (idx_tourist_submissions_dedup) only fires on
-- status='pending', so multiple drafts for the same passport number do not
-- conflict — only the finalize step competes for the dedup slot.
ALTER TABLE tourist_submissions DROP CONSTRAINT IF EXISTS tourist_submissions_status_check;
ALTER TABLE tourist_submissions
  ADD CONSTRAINT tourist_submissions_status_check
  CHECK (status IN ('draft','pending','attached','archived','deleted'));
