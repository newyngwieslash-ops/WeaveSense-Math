-- Lossless industry output and fixed-payload idempotency for managed reports.
ALTER TABLE flow_reports ADD COLUMN IF NOT EXISTS structured_output JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE flow_reports ADD COLUMN IF NOT EXISTS request_hash TEXT;
