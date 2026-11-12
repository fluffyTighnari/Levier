-- Middleware — middleware-generated event sequence.
--
-- seq for source='mw' events lives in [2^62, 2^63). A database sequence keeps
-- allocation monotonic and unique without touching the daemon watermark.
--
-- Copyright 2026 The Middleware Authors.

CREATE SEQUENCE IF NOT EXISTS run_event_mw_seq
  AS BIGINT
  INCREMENT BY 1
  MINVALUE 1
  MAXVALUE 4611686018427387903
  START WITH 1
  CACHE 1;

-- A control-plane event may exist before the first daemon attempt (attempt=0).
ALTER TABLE run_events DROP CONSTRAINT IF EXISTS run_events_attempt_positive;
ALTER TABLE run_events
  ADD CONSTRAINT run_events_attempt_positive
  CHECK (attempt >= 1 OR (source = 'mw' AND attempt = 0))
  NOT VALID;
