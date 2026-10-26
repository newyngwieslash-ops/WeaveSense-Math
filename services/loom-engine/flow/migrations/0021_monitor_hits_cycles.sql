-- Freeze collection membership at admission, independently of later monitor edits.
CREATE TABLE flow_collection_windows (
  tenant_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  operation_id TEXT NOT NULL,
  schedule_id TEXT NOT NULL,
  scheduled_at TIMESTAMPTZ NOT NULL,
  expected_source_ids TEXT[] NOT NULL,
  monitor_id TEXT,
  monitor_version INTEGER,
  keywords TEXT[] NOT NULL DEFAULT '{}',
  canvas_id TEXT,
  canvas_auto_run_grant JSONB,
  interval_seconds INTEGER NOT NULL,
  timezone TEXT NOT NULL,
  PRIMARY KEY (tenant_id, workspace_id, operation_id),
  UNIQUE (tenant_id, workspace_id, schedule_id, scheduled_at)
);
CREATE TABLE flow_monitor_hits (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  monitor_id TEXT NOT NULL,
  monitor_version INTEGER NOT NULL,
  schedule_id TEXT NOT NULL,
  operation_id TEXT NOT NULL,
  job_id TEXT NOT NULL,
  source_id TEXT NOT NULL,
  document_id TEXT NOT NULL,
  revision INTEGER NOT NULL CHECK (revision > 0),
  matched_at TIMESTAMPTZ NOT NULL,
  scheduled_at TIMESTAMPTZ NOT NULL,
  UNIQUE (tenant_id, workspace_id, monitor_id, operation_id, document_id, revision)
);
CREATE INDEX flow_monitor_hits_page ON flow_monitor_hits (tenant_id, workspace_id, monitor_id, matched_at, id);
ALTER TABLE flow_collection_windows ENABLE ROW LEVEL SECURITY;
CREATE POLICY flow_collection_windows_tenant ON flow_collection_windows USING (coalesce(current_setting('app.tenant_id', true), '') = '' OR tenant_id = current_setting('app.tenant_id', true));
ALTER TABLE flow_monitor_hits ENABLE ROW LEVEL SECURITY;
CREATE POLICY flow_monitor_hits_tenant ON flow_monitor_hits USING (coalesce(current_setting('app.tenant_id', true), '') = '' OR tenant_id = current_setting('app.tenant_id', true));
