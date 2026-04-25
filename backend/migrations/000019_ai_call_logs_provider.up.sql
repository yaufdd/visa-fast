-- ai_call_logs.provider: distinguish which AI provider made the call so the
-- audit log stays meaningful during the Anthropic → Yandex transition.
--
-- Existing rows predate Yandex integration and are all Anthropic, so we
-- backfill via DEFAULT 'anthropic' and then drop the default — every future
-- INSERT must specify the provider explicitly.
ALTER TABLE ai_call_logs ADD COLUMN provider TEXT NOT NULL DEFAULT 'anthropic';

ALTER TABLE ai_call_logs ALTER COLUMN provider DROP DEFAULT;

ALTER TABLE ai_call_logs
    ADD CONSTRAINT ai_call_logs_provider_check
    CHECK (provider IN ('anthropic', 'yandex-gpt', 'yandex-vision'));
