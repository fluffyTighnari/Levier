-- Levier - I3 connection leases and gateway registry.
-- Copyright 2026 The Levier Authors.

CREATE TABLE IF NOT EXISTS gateway_instances (
  gateway_id       TEXT PRIMARY KEY,
  epoch            BIGINT NOT NULL DEFAULT 1 CHECK (epoch > 0),
  roles            TEXT[] NOT NULL CHECK (cardinality(roles) > 0),
  status           TEXT NOT NULL DEFAULT 'online'
                     CHECK (status IN ('online', 'offline')),
  version          TEXT NOT NULL DEFAULT '',
  address          TEXT NOT NULL DEFAULT '',
  labels_json      JSONB NOT NULL DEFAULT '{}'::jsonb,
  started_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_expires_at TIMESTAMPTZ NOT NULL,
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_gateway_instances_expiry
  ON gateway_instances(status, lease_expires_at);

CREATE TABLE IF NOT EXISTS runtime_connections (
  daemon_id          TEXT PRIMARY KEY,
  gateway_id         TEXT NOT NULL,
  connection_epoch   BIGINT NOT NULL DEFAULT 1 CHECK (connection_epoch > 0),
  runtime_name       TEXT NOT NULL,
  runtime_type       TEXT NOT NULL,
  status             TEXT NOT NULL DEFAULT 'online'
                       CHECK (status IN ('online', 'offline')),
  version            TEXT NOT NULL DEFAULT '',
  max_concurrent_runs INTEGER NOT NULL DEFAULT 1 CHECK (max_concurrent_runs > 0),
  running_count      INTEGER NOT NULL DEFAULT 0 CHECK (running_count >= 0),
  reserved           INTEGER NOT NULL DEFAULT 0 CHECK (reserved >= 0),
  labels_json        JSONB NOT NULL DEFAULT '{}'::jsonb,
  capabilities_json  JSONB NOT NULL DEFAULT '{}'::jsonb,
  running_runs_json  JSONB NOT NULL DEFAULT '[]'::jsonb,
  connected_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_heartbeat_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_expires_at   TIMESTAMPTZ NOT NULL,
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_runtime_connections_gateway
  ON runtime_connections(gateway_id, status);
CREATE INDEX IF NOT EXISTS idx_runtime_connections_expiry
  ON runtime_connections(status, lease_expires_at);
CREATE INDEX IF NOT EXISTS idx_runtime_connections_runtime
  ON runtime_connections(runtime_name, runtime_type, status);

CREATE TABLE IF NOT EXISTS app_connections (
  connection_id     TEXT PRIMARY KEY,
  app_id            TEXT NOT NULL REFERENCES apps(app_id) ON DELETE CASCADE,
  tenant_id         TEXT NOT NULL,
  gateway_id        TEXT NOT NULL,
  connection_epoch  BIGINT NOT NULL DEFAULT 1 CHECK (connection_epoch > 0),
  protocol_version  INTEGER NOT NULL DEFAULT 1 CHECK (protocol_version > 0),
  capabilities_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  status            TEXT NOT NULL DEFAULT 'online'
                      CHECK (status IN ('online', 'offline')),
  connected_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_expires_at  TIMESTAMPTZ NOT NULL,
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_connections_app
  ON app_connections(app_id, tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_app_connections_gateway
  ON app_connections(gateway_id, status);
CREATE INDEX IF NOT EXISTS idx_app_connections_expiry
  ON app_connections(status, lease_expires_at);

CREATE TABLE IF NOT EXISTS app_execution_leases (
  app_id            TEXT PRIMARY KEY REFERENCES apps(app_id) ON DELETE CASCADE,
  connection_id     TEXT NOT NULL REFERENCES app_connections(connection_id) ON DELETE CASCADE,
  gateway_id        TEXT NOT NULL,
  lease_epoch       BIGINT NOT NULL DEFAULT 1 CHECK (lease_epoch > 0),
  expires_at        TIMESTAMPTZ NOT NULL,
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_execution_leases_expiry
  ON app_execution_leases(expires_at);
CREATE INDEX IF NOT EXISTS idx_app_execution_leases_gateway
  ON app_execution_leases(gateway_id);

CREATE TABLE IF NOT EXISTS gateway_notifications (
  notification_id BIGSERIAL PRIMARY KEY,
  channel         TEXT NOT NULL,
  payload_json    JSONB NOT NULL,
  attempts        INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_gateway_notifications_pending
  ON gateway_notifications(channel, next_attempt_at)
  WHERE published_at IS NULL;
