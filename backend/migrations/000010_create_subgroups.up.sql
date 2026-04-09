CREATE TABLE subgroups (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   UUID NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    sort_order INT  NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_subgroups_group_id ON subgroups(group_id);

ALTER TABLE tourists ADD COLUMN subgroup_id UUID REFERENCES subgroups(id) ON DELETE SET NULL;
CREATE INDEX idx_tourists_subgroup_id ON tourists(subgroup_id);

ALTER TABLE uploads ADD COLUMN subgroup_id UUID REFERENCES subgroups(id) ON DELETE SET NULL;
CREATE INDEX idx_uploads_subgroup_id ON uploads(subgroup_id);
