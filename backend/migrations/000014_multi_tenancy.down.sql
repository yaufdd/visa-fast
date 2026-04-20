DROP INDEX IF EXISTS idx_hotels_org_id;
DROP INDEX IF EXISTS idx_uploads_org_id;
DROP INDEX IF EXISTS idx_documents_org_id;
DROP INDEX IF EXISTS idx_group_hotels_org_id;
DROP INDEX IF EXISTS idx_tourist_submissions_org_id;
DROP INDEX IF EXISTS idx_tourists_org_id;
DROP INDEX IF EXISTS idx_subgroups_org_id;
DROP INDEX IF EXISTS idx_groups_org_id;

ALTER TABLE hotels              DROP COLUMN IF EXISTS org_id;
ALTER TABLE uploads             DROP COLUMN IF EXISTS org_id;
ALTER TABLE documents           DROP COLUMN IF EXISTS org_id;
ALTER TABLE group_hotels        DROP COLUMN IF EXISTS org_id;
ALTER TABLE tourist_submissions DROP COLUMN IF EXISTS org_id;
ALTER TABLE tourists            DROP COLUMN IF EXISTS org_id;
ALTER TABLE subgroups           DROP COLUMN IF EXISTS org_id;
ALTER TABLE groups              DROP COLUMN IF EXISTS org_id;

DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS organizations;
