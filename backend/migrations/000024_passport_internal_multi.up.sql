-- Allow multiple internal-passport scans per submission. The Документы
-- step on the manager-side wizard now lets the manager upload several
-- pages (main + registration) and recognise them all. Foreign passport
-- still keeps the replace-on-upload behaviour (one row per submission)
-- so its ON CONFLICT path in InsertOrReplaceSubmissionFile keeps working.

DROP INDEX IF EXISTS submission_files_passport_uniq;

CREATE UNIQUE INDEX submission_files_passport_foreign_uniq
    ON submission_files (submission_id, file_type)
 WHERE file_type = 'passport_foreign';
