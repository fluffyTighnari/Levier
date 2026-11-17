-- Levier — P4-33 Control Run input and tool timeout.
-- Copyright 2026 The Levier Authors.

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS input_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS tool_timeout_ms INTEGER NOT NULL DEFAULT 60000;
