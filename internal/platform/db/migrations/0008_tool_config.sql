-- Levier — P7-54 global executor configuration.
-- Copyright 2026 The Levier Authors.

ALTER TABLE tool_defs
  ADD COLUMN IF NOT EXISTS config_json TEXT;
