-- Levier - soft tenant scope for apps, runs, and sessions.
-- Copyright 2026 The Levier Authors.

ALTER TABLE apps
  ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';

ALTER TABLE sessions
  ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT 'default';

-- App names are scoped per tenant. Existing Apps stay in the default tenant.
ALTER TABLE apps
  DROP CONSTRAINT IF EXISTS apps_name_key;

CREATE UNIQUE INDEX IF NOT EXISTS uq_apps_tenant_name
  ON apps(tenant_id, name);

CREATE INDEX IF NOT EXISTS idx_apps_tenant_status
  ON apps(tenant_id, status);

CREATE INDEX IF NOT EXISTS idx_runs_tenant_status
  ON runs(tenant_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_sessions_tenant_app_status
  ON sessions(tenant_id, app_id, status);
