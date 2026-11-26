-- Levier - resumable waiting run status.
-- Copyright 2026 The Levier Authors.

ALTER TABLE runs
  DROP CONSTRAINT IF EXISTS runs_status_check;

ALTER TABLE runs
  ADD CONSTRAINT runs_status_check CHECK (status IN (
    'queued', 'assigned', 'running', 'waiting',
    'completed', 'failed', 'cancelled', 'timeout'
  )) NOT VALID;
