-- Collection tables must enforce tenant policies for the non-superuser table owner.
ALTER TABLE flow_collection_windows FORCE ROW LEVEL SECURITY;
ALTER TABLE flow_monitor_hits FORCE ROW LEVEL SECURITY;
