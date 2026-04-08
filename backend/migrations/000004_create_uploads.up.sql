CREATE TABLE uploads (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id    UUID NOT NULL REFERENCES groups  (id) ON DELETE CASCADE,
    tourist_id  UUID          REFERENCES tourists (id) ON DELETE SET NULL,
    file_type   TEXT NOT NULL CHECK (file_type IN ('passport', 'ticket', 'voucher')),
    file_path   TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_uploads_group_id   ON uploads (group_id);
CREATE INDEX idx_uploads_tourist_id ON uploads (tourist_id);
CREATE INDEX idx_uploads_file_type  ON uploads (file_type);
