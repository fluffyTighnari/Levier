-- Middleware — P7-54 global executor configuration.
-- Copyright 2026 The Middleware Authors.

ALTER TABLE tool_defs
  ADD COLUMN IF NOT EXISTS config_json TEXT;
