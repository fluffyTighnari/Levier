-- Middleware — P2-15 run_tasks constraints and indexes.
-- Copyright 2026 The Middleware Authors.

ALTER TABLE run_tasks
  ADD CONSTRAINT run_tasks_attempt_check
    CHECK (attempt >= 0 AND attempt <= max_attempts),
  ADD CONSTRAINT run_tasks_max_attempts_check
    CHECK (max_attempts > 0),
  ADD CONSTRAINT run_tasks_lease_check
    CHECK (
      (status = 'LEASED'
        AND lease_owner <> ''
        AND lease_token > 0
        AND lease_expires_at IS NOT NULL)
      OR
      (status <> 'LEASED'
        AND lease_owner = ''
        AND lease_expires_at IS NULL)
    );

CREATE INDEX IF NOT EXISTS idx_run_tasks_run
  ON run_tasks(run_id, status);

CREATE INDEX IF NOT EXISTS idx_run_tasks_dead
  ON run_tasks(status, updated_at)
  WHERE status = 'DEAD';
