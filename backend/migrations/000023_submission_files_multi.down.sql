-- Restore the original "one file per (submission, type)" rule. If
-- multiple ticket / voucher rows accumulated under the new schema we keep
-- only the most recent (highest created_at) per submission+type so the
-- UNIQUE rebuild succeeds; older rows are dropped (the on-disk files are
-- left alone — ops can sweep <uploadsDir>/<org>/submissions/<id>/ if
-- needed).

DELETE FROM submission_files sf
 USING (
     SELECT id
       FROM (
           SELECT id,
                  ROW_NUMBER() OVER (
                      PARTITION BY submission_id, file_type
                      ORDER BY created_at DESC, id DESC
                  ) AS rn
             FROM submission_files
            WHERE file_type IN ('ticket','voucher')
       ) ranked
      WHERE rn > 1
 ) dup
 WHERE sf.id = dup.id;

DROP INDEX IF EXISTS submission_files_passport_uniq;

ALTER TABLE submission_files
    ADD CONSTRAINT submission_files_submission_id_file_type_key
    UNIQUE (submission_id, file_type);
