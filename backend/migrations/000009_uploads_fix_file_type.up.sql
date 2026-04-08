ALTER TABLE uploads DROP CONSTRAINT IF EXISTS uploads_file_type_check;
ALTER TABLE uploads ADD CONSTRAINT uploads_file_type_check
    CHECK (file_type IN ('passport', 'foreign_passport', 'ticket', 'voucher', 'unknown', 'document'));
