-- Restore the original "one passport row per submission" constraint for
-- both passport types. Note: this will fail loudly if any submission has
-- accumulated more than one passport_internal row by the time of rollback —
-- that's intentional, the operator must clean up duplicates first.

DROP INDEX IF EXISTS submission_files_passport_foreign_uniq;

CREATE UNIQUE INDEX submission_files_passport_uniq
    ON submission_files (submission_id, file_type)
 WHERE file_type IN ('passport_internal', 'passport_foreign');
