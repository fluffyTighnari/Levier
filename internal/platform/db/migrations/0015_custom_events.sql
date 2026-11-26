-- Levier - custom Runtime event metadata.
-- Copyright 2026 The Levier Authors.

ALTER TABLE run_events
  ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS schema_version TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_run_events_custom_name
  ON run_events(run_id, name, ts_ms)
  WHERE name <> '';
