-- submission_files: scans (passport, ticket, voucher) uploaded by a tourist
-- through the public form before the manager picks the submission into a
-- group. Each row pins one file to one tourist_submissions row, scoped to
-- the same org_id so multi-tenancy stays intact end-to-end.
--
-- The UNIQUE (submission_id, file_type) constraint enforces "one file per
-- type per submission": a tourist who re-uploads their foreign passport
-- replaces the previous attempt instead of accumulating duplicates, which
-- keeps the public form idempotent and the manager's view tidy.
--
-- file_path points at the on-disk location under UPLOADS_DIR; the DB row
-- carries the metadata (original_name, mime_type, size_bytes) needed to
-- serve the file back to the manager without re-reading it from disk.
CREATE TABLE submission_files (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    submission_id UUID NOT NULL REFERENCES tourist_submissions(id) ON DELETE CASCADE,
    file_type     TEXT NOT NULL CHECK (file_type IN ('passport_internal','passport_foreign','ticket','voucher')),
    file_path     TEXT NOT NULL,
    original_name TEXT NOT NULL,
    mime_type     TEXT NOT NULL,
    size_bytes    BIGINT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (submission_id, file_type)
);

CREATE INDEX submission_files_submission_idx ON submission_files (submission_id);
CREATE INDEX submission_files_org_idx ON submission_files (org_id);
