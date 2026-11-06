-- Middleware — P4-31 minimal RunStore schema.
-- Copyright 2026 The Middleware Authors.

CREATE TABLE IF NOT EXISTS apps (
  app_id        TEXT PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,
  api_key_hash  TEXT NOT NULL,
  multiagent    BOOLEAN NOT NULL DEFAULT FALSE,
  status        TEXT NOT NULL DEFAULT 'active',
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS daemons (
  daemon_id        TEXT PRIMARY KEY,
  runtime_type     TEXT NOT NULL CHECK (runtime_type IN ('normal', 'sandbox', 'longterm')),
  max_concurrency  INTEGER NOT NULL DEFAULT 4,
  labels_json      JSONB NOT NULL DEFAULT '{}'::jsonb,
  status           TEXT NOT NULL DEFAULT 'offline'
                     CHECK (status IN ('online', 'offline', 'draining')),
  last_heartbeat_at TIMESTAMPTZ
);

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS request_hash TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS runtime TEXT NOT NULL DEFAULT 'normal',
  ADD COLUMN IF NOT EXISTS strategy TEXT NOT NULL DEFAULT 'react',
  ADD COLUMN IF NOT EXISTS params_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS model_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS run_timeout_ms INTEGER NOT NULL DEFAULT 1800000,
  ADD COLUMN IF NOT EXISTS trace_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS session_id TEXT,
  ADD COLUMN IF NOT EXISTS assigned_daemon_id TEXT,
  ADD COLUMN IF NOT EXISTS lease_epoch BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS assigned_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_event_attempt INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_event_seq BIGINT NOT NULL DEFAULT 0;

CREATE UNIQUE INDEX IF NOT EXISTS uq_runs_idempotency
  ON runs(app_id, idempotency_key)
  WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_runs_status
  ON runs(status);

CREATE INDEX IF NOT EXISTS idx_runs_daemon
  ON runs(assigned_daemon_id, status);

CREATE INDEX IF NOT EXISTS idx_runs_session
  ON runs(session_id)
  WHERE session_id IS NOT NULL AND session_id <> '';

CREATE INDEX IF NOT EXISTS idx_run_events_run_ts
  ON run_events(run_id, ts_ms);
