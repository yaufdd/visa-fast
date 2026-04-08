CREATE TABLE documents (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id     UUID NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    pass2_json   JSONB,
    zip_path     TEXT,
    generated_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_documents_group_id    ON documents (group_id);
CREATE INDEX idx_documents_created_at  ON documents (created_at);
