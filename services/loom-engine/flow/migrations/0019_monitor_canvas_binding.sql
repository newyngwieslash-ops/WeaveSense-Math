-- Monitor owns the binding; schedules resolve it through the existing schedule_id.
ALTER TABLE flow_monitors ADD COLUMN canvas_id TEXT;
ALTER TABLE flow_monitors ADD COLUMN canvas_auto_run_grant JSONB;
ALTER TABLE flow_monitors ADD CONSTRAINT flow_monitor_canvas_id_nonempty CHECK (canvas_id IS NULL OR length(trim(canvas_id)) > 0);
ALTER TABLE flow_monitors ADD CONSTRAINT flow_monitor_canvas_grant_bound CHECK (canvas_auto_run_grant IS NULL OR (canvas_id IS NOT NULL AND jsonb_typeof(canvas_auto_run_grant) = 'object'));
