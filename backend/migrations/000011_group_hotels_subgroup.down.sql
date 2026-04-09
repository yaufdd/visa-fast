DROP INDEX IF EXISTS idx_group_hotels_subgroup_id;
ALTER TABLE group_hotels DROP COLUMN IF EXISTS subgroup_id;
