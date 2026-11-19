-- Levier - model provider catalog and secret references.
-- Copyright 2026 The Levier Authors.

CREATE TABLE IF NOT EXISTS model_providers (
  provider_id    TEXT PRIMARY KEY,
  name           TEXT NOT NULL UNIQUE,
  adapter        TEXT NOT NULL CHECK (adapter IN ('openai_compatible', 'anthropic')),
  base_url       TEXT NOT NULL DEFAULT '',
  default_model  TEXT NOT NULL DEFAULT '',
  models_json    TEXT NOT NULL DEFAULT '[]',
  secret_ref     TEXT NOT NULL DEFAULT '',
  enabled        BOOLEAN NOT NULL DEFAULT TRUE,
  is_default     BOOLEAN NOT NULL DEFAULT FALSE,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_model_providers_default
  ON model_providers(is_default)
  WHERE is_default;
