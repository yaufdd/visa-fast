-- Allow multiple ticket / voucher files per submission. Passports stay
-- unique (one внутренний + one загран per submission, replace-on-upload).
--
-- The original UNIQUE (submission_id, file_type) constraint is replaced
-- with a partial unique index that only covers the two passport file
-- types — so ON CONFLICT (submission_id, file_type) keeps working in
-- InsertOrReplaceSubmissionFile for passports while plain INSERT for
-- tickets/vouchers can stack rows.

ALTER TABLE submission_files
    DROP CONSTRAINT submission_files_submission_id_file_type_key;

CREATE UNIQUE INDEX submission_files_passport_uniq
    ON submission_files (submission_id, file_type)
 WHERE file_type IN ('passport_internal', 'passport_foreign');
