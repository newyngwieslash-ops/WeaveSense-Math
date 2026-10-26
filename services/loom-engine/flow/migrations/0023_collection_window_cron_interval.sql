-- Cron schedules have no interval_seconds (flow_schedules allows NULL since 0012);
-- freezing such a window must not fail the whole scheduler tick.
ALTER TABLE flow_collection_windows ALTER COLUMN interval_seconds DROP NOT NULL;
