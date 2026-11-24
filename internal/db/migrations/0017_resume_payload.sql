-- Levier - external resume payload for a waiting run.
-- Copyright 2026 The Levier Authors.

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS resume_json JSONB;
