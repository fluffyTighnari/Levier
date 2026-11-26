-- Levier — P0-07 initial PostgreSQL schema.
-- Copyright 2026 The Levier Authors.

CREATE TABLE IF NOT EXISTS runs (
  run_id      TEXT PRIMARY KEY,
  app_id      TEXT NOT NULL,
  status      TEXT NOT NULL,
  attempt     INTEGER NOT NULL DEFAULT 0,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS run_events (
  run_id       TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
  attempt      INTEGER NOT NULL,
  seq          BIGINT NOT NULL,
  source       TEXT NOT NULL CHECK (source IN ('daemon', 'mw')),
  type         TEXT NOT NULL,
  payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
  ts_ms        BIGINT NOT NULL,
  PRIMARY KEY (run_id, attempt, seq)
);

CREATE TABLE IF NOT EXISTS run_tasks (
  task_id          TEXT PRIMARY KEY,
  run_id           TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
  task_type        TEXT NOT NULL CHECK (task_type IN
                     ('RUN_DISPATCH', 'RUN_RETRY', 'RUN_TIMEOUT', 'RUN_RECONCILE')),
  status           TEXT NOT NULL CHECK (status IN
                     ('PENDING', 'LEASED', 'DONE', 'DEAD', 'CANCELLED')),
  available_at     TIMESTAMPTZ NOT NULL,
  attempt          INTEGER NOT NULL DEFAULT 0,
  max_attempts     INTEGER NOT NULL DEFAULT 5,
  lease_owner      TEXT NOT NULL DEFAULT '',
  lease_token      BIGINT NOT NULL DEFAULT 0,
  lease_expires_at TIMESTAMPTZ,
  last_error       TEXT NOT NULL DEFAULT '',
  payload          JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_run_tasks_active
  ON run_tasks(run_id, task_type)
  WHERE status IN ('PENDING', 'LEASED');

CREATE INDEX IF NOT EXISTS idx_run_tasks_ready
  ON run_tasks(status, available_at, created_at);

CREATE INDEX IF NOT EXISTS idx_run_tasks_lease
  ON run_tasks(status, lease_expires_at)
  WHERE status = 'LEASED';
