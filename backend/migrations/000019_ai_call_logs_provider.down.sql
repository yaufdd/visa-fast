ALTER TABLE ai_call_logs DROP CONSTRAINT IF EXISTS ai_call_logs_provider_check;
ALTER TABLE ai_call_logs DROP COLUMN IF EXISTS provider;
