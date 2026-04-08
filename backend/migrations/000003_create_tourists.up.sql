CREATE TABLE tourists (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id          UUID NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    raw_json          JSONB,
    matched_sheet_row JSONB,
    match_confirmed   BOOLEAN NOT NULL DEFAULT false,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_tourists_group_id        ON tourists (group_id);
CREATE INDEX idx_tourists_match_confirmed ON tourists (match_confirmed);
CREATE INDEX idx_tourists_created_at      ON tourists (created_at);
