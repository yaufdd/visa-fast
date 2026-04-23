-- ai_call_logs: every call to Anthropic made on behalf of an org, scoped
-- to a single generation run so a manager can inspect exactly what left the
-- server for one generate / finalize action.
--
-- request_json stores the full anthropicRequest body with image bytes
-- redacted ("[image redacted, N bytes]" placeholder) so the row stays
-- small while still showing what fields went out.
--
-- response_text stores the raw Claude text reply (often already JSON, but
-- we keep the bytes as received for audit reasons).
CREATE TABLE ai_call_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    group_id        UUID REFERENCES groups(id) ON DELETE SET NULL,
    subgroup_id     UUID REFERENCES subgroups(id) ON DELETE SET NULL,
    generation_id   UUID NOT NULL,
    function_name   TEXT NOT NULL,
    model           TEXT NOT NULL,
    request_json    JSONB NOT NULL,
    response_text   TEXT,
    status          TEXT NOT NULL CHECK (status IN ('success','error')),
    error_msg       TEXT,
    input_tokens    INTEGER,
    output_tokens   INTEGER,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ,
    duration_ms     INTEGER
);

CREATE INDEX idx_ai_call_logs_generation_id ON ai_call_logs(generation_id);
CREATE INDEX idx_ai_call_logs_org_group ON ai_call_logs(org_id, group_id, started_at DESC);
CREATE INDEX idx_ai_call_logs_started_at ON ai_call_logs(started_at DESC);
