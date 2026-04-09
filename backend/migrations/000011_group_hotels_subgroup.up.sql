ALTER TABLE group_hotels
    ADD COLUMN subgroup_id UUID REFERENCES subgroups (id) ON DELETE CASCADE;

CREATE INDEX idx_group_hotels_subgroup_id ON group_hotels (subgroup_id);
