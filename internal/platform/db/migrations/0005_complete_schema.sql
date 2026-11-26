-- Levier — P5-37 complete RunStore, registry, and session schema.
-- Copyright 2026 The Levier Authors.

CREATE TABLE IF NOT EXISTS tool_defs (
  tool_id      TEXT PRIMARY KEY,
  scope        TEXT NOT NULL CHECK (scope IN ('local', 'global')),
  app_id       TEXT REFERENCES apps(app_id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  description  TEXT NOT NULL DEFAULT '',
  schema_json  TEXT NOT NULL CHECK (schema_json <> ''),
  executor_ref TEXT,
  timeout_ms   INTEGER NOT NULL DEFAULT 60000 CHECK (timeout_ms > 0),
  acl_json     TEXT,
  version      INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (
    (scope = 'local' AND app_id IS NOT NULL)
    OR
    (scope = 'global' AND app_id IS NULL AND executor_ref IS NOT NULL)
  )
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tool_defs_local
  ON tool_defs(app_id, name)
  WHERE scope = 'local';

CREATE UNIQUE INDEX IF NOT EXISTS uq_tool_defs_global
  ON tool_defs(name)
  WHERE scope = 'global';

CREATE INDEX IF NOT EXISTS idx_tool_defs_scope
  ON tool_defs(scope, name);

CREATE TABLE IF NOT EXISTS mcp_server_defs (
  mcp_id             TEXT PRIMARY KEY,
  name               TEXT NOT NULL UNIQUE,
  transport          TEXT NOT NULL CHECK (transport IN ('stdio', 'sse', 'streamable_http')),
  config_json        TEXT NOT NULL CHECK (config_json <> ''),
  allowed_tools_json TEXT,
  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
  session_id      TEXT PRIMARY KEY,
  app_id          TEXT NOT NULL REFERENCES apps(app_id) ON DELETE CASCADE,
  status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'paused', 'expired')),
  current_run_id  TEXT,
  checkpoint_blob BYTEA,
  event_attempt   INTEGER NOT NULL DEFAULT 0 CHECK (event_attempt >= 0),
  event_watermark BIGINT NOT NULL DEFAULT 0 CHECK (event_watermark >= 0),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sessions_app_status
  ON sessions(app_id, status);

CREATE INDEX IF NOT EXISTS idx_sessions_expires
  ON sessions(expires_at)
  WHERE expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS tool_invocations (
  invocation_id TEXT PRIMARY KEY,
  run_id        TEXT NOT NULL REFERENCES runs(run_id) ON DELETE CASCADE,
  tool_name     TEXT NOT NULL,
  status        TEXT NOT NULL CHECK (status IN ('in_flight', 'completed', 'failed')),
  result_json   TEXT,
  error_json    TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_tool_invocations_run
  ON tool_invocations(run_id, created_at);

CREATE INDEX IF NOT EXISTS idx_tool_invocations_status
  ON tool_invocations(status, created_at)
  WHERE status = 'in_flight';

-- Run state and event constraints for the remaining P5 lifecycle work.
ALTER TABLE runs
  ADD CONSTRAINT runs_attempt_nonnegative CHECK (attempt >= 0) NOT VALID,
  ADD CONSTRAINT runs_lease_epoch_nonnegative CHECK (lease_epoch >= 0) NOT VALID,
  ADD CONSTRAINT runs_run_timeout_positive CHECK (run_timeout_ms > 0) NOT VALID,
  ADD CONSTRAINT runs_tool_timeout_positive CHECK (tool_timeout_ms > 0) NOT VALID,
  ADD CONSTRAINT runs_status_check CHECK (status IN (
    'queued', 'assigned', 'running', 'completed', 'failed', 'cancelled', 'timeout'
  )) NOT VALID;

ALTER TABLE run_events
  ADD CONSTRAINT run_events_attempt_positive CHECK (attempt >= 1) NOT VALID,
  ADD CONSTRAINT run_events_seq_positive CHECK (seq >= 1) NOT VALID;

CREATE INDEX IF NOT EXISTS idx_runs_app_status
  ON runs(app_id, status);

CREATE INDEX IF NOT EXISTS idx_run_events_type
  ON run_events(run_id, type, ts_ms);
