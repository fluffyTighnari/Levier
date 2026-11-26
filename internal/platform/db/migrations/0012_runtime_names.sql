-- Levier — replace run strategy with App-facing runtime name.
-- Copyright 2026 The Levier Authors.

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS runtime_name TEXT NOT NULL DEFAULT '';

UPDATE runs
SET runtime_name = strategy
WHERE runtime_name = '' AND strategy <> '';

UPDATE runs
SET runtime_name = 'legacy'
WHERE runtime_name = '';

CREATE INDEX IF NOT EXISTS idx_runs_runtime_name_status
  ON runs(runtime_name, status);

ALTER TABLE runs
  DROP COLUMN IF EXISTS strategy;
