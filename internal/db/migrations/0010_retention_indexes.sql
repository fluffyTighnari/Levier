-- Levier — lifecycle retention indexes.
-- Copyright 2026 The Levier Authors.

CREATE INDEX IF NOT EXISTS idx_runs_terminal_updated
  ON runs(updated_at)
  WHERE status IN ('completed', 'failed', 'cancelled', 'timeout');

CREATE INDEX IF NOT EXISTS idx_sessions_status_expires
  ON sessions(status, expires_at)
  WHERE expires_at IS NOT NULL;
