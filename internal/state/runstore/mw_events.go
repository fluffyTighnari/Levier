// Levier — control-plane run events.
//
// Copyright 2026 The Levier Authors.
package runstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// AppendMWEvent inserts one middleware-generated event.
//
// The event uses source="mw" and a seq in the reserved MW range. It does not
// advance runs.last_event_attempt/last_event_seq: those columns are the daemon
// stream watermark used for gap detection and replay.
func (s *Store) AppendMWEvent(ctx context.Context, event Event) (Event, error) {
	if s == nil || s.pool == nil {
		return Event{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if event.RunID == "" {
		return Event{}, fmt.Errorf("%w: run_id is required", ErrInvalidEvent)
	}
	if event.Type == "" {
		return Event{}, fmt.Errorf("%w: type is required", ErrInvalidEvent)
	}
	if len(event.Payload) == 0 {
		event.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(event.Payload) {
		return Event{}, fmt.Errorf("%w: payload is not valid JSON", ErrInvalidEvent)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Event{}, fmt.Errorf("append mw event: begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	var attempt int
	var leaseEpoch int64
	err = tx.QueryRow(ctx, `
		SELECT attempt, lease_epoch
		FROM runs
		WHERE run_id = $1
		FOR UPDATE
	`, event.RunID).Scan(&attempt, &leaseEpoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrRunNotFound
	}
	if err != nil {
		return Event{}, fmt.Errorf("append mw event: lock run: %w", err)
	}

	var seq int64
	if err := tx.QueryRow(ctx,
		`SELECT nextval('run_event_mw_seq') + $1`, MWSeqBase,
	).Scan(&seq); err != nil {
		return Event{}, fmt.Errorf("append mw event: allocate seq: %w", err)
	}

	event.Attempt = attempt
	event.LeaseEpoch = leaseEpoch
	event.Source = "mw"
	event.Seq = seq
	event.TSMs = time.Now().UnixMilli()

	if _, err := tx.Exec(ctx, `
		INSERT INTO run_events (run_id, attempt, seq, source, type, payload, ts_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (run_id, attempt, seq) DO NOTHING
	`, event.RunID, event.Attempt, event.Seq, event.Source,
		event.Type, event.Payload, event.TSMs); err != nil {
		return Event{}, fmt.Errorf("append mw event: insert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Event{}, fmt.Errorf("append mw event: commit: %w", err)
	}
	return event, nil
}
