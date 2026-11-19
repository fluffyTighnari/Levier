-- Levier — operator run/task list indexes.
-- Copyright 2026 The Levier Authors.

CREATE INDEX IF NOT EXISTS idx_runs_ops_created
  ON runs(created_at DESC, run_id DESC);

CREATE INDEX IF NOT EXISTS idx_runs_ops_app_created
  ON runs(app_id, created_at DESC, run_id DESC);

CREATE INDEX IF NOT EXISTS idx_runs_ops_daemon_created
  ON runs(assigned_daemon_id, created_at DESC, run_id DESC)
  WHERE assigned_daemon_id IS NOT NULL AND assigned_daemon_id <> '';

CREATE INDEX IF NOT EXISTS idx_run_tasks_ops_updated
  ON run_tasks(updated_at DESC, task_id DESC);
