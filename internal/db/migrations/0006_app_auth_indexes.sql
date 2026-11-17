-- Levier — P5-41 App authentication indexes.
-- Copyright 2026 The Levier Authors.

CREATE UNIQUE INDEX IF NOT EXISTS uq_apps_api_key_hash
  ON apps(api_key_hash);

CREATE INDEX IF NOT EXISTS idx_apps_status
  ON apps(status);
