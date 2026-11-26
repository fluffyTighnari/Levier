-- Levier - I3.0.1 capacity, internal addresses, and notification claiming.
-- Copyright 2026 The Levier Authors.

ALTER TABLE gateway_instances
  ADD COLUMN IF NOT EXISTS internal_address TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS runtime_capacity_leases (
  daemon_id       TEXT NOT NULL,
  reservation_id  TEXT NOT NULL,
  gateway_id      TEXT NOT NULL,
  expires_at      TIMESTAMPTZ NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (daemon_id, reservation_id)
);

CREATE INDEX IF NOT EXISTS idx_runtime_capacity_leases_expiry
  ON runtime_capacity_leases(expires_at);
CREATE INDEX IF NOT EXISTS idx_runtime_capacity_leases_gateway
  ON runtime_capacity_leases(gateway_id);

ALTER TABLE gateway_notifications
  ADD COLUMN IF NOT EXISTS dedupe_key TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS claimed_by TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS claimed_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE UNIQUE INDEX IF NOT EXISTS uq_gateway_notifications_dedupe
  ON gateway_notifications(dedupe_key)
  WHERE dedupe_key <> '' AND published_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_gateway_notifications_claim
  ON gateway_notifications(channel, next_attempt_at, claimed_until)
  WHERE published_at IS NULL;

ALTER TABLE tool_invocations
  ADD COLUMN IF NOT EXISTS owner_gateway_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS owner_connection_id TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS lease_epoch BIGINT NOT NULL DEFAULT 0 CHECK (lease_epoch >= 0),
  ADD COLUMN IF NOT EXISTS deadline_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_tool_invocations_owner
  ON tool_invocations(owner_gateway_id, owner_connection_id, status);
CREATE INDEX IF NOT EXISTS idx_tool_invocations_deadline
  ON tool_invocations(status, deadline_at)
  WHERE status = 'in_flight';
