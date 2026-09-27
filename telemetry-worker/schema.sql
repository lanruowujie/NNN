CREATE TABLE IF NOT EXISTS telemetry_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  installation_id TEXT NOT NULL,
  event TEXT NOT NULL,
  app_version TEXT NOT NULL,
  os TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS telemetry_events_installation_id ON telemetry_events(installation_id);
CREATE INDEX IF NOT EXISTS telemetry_events_created_at ON telemetry_events(created_at);
CREATE INDEX IF NOT EXISTS telemetry_events_event ON telemetry_events(event);
