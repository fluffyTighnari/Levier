// Middleware — runstore
//
// Split from store.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package runstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// MWSeqBase is the first seq reserved for middleware-generated events.
const MWSeqBase int64 = 1 << 62

// EventGapWindow is the maximum accepted out-of-order seq gap.
const EventGapWindow int64 = 16

// AppendEvent inserts one event idempotently and advances the run projection.
// It validates the run lease epoch and reports out-of-window sequence gaps.
func (s *Store) AppendEvent(ctx context.Context, event Event) (AppendResult, error) {
	if s == nil || s.pool == nil {
		return AppendResult{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	normalized, err := normalizeEvent(event)
	if err != nil {
		return AppendResult{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AppendResult{}, fmt.Errorf("append event: begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	var leaseEpoch int64
	var lastAttempt int
	var lastSeq int64
	err = tx.QueryRow(ctx, `
		SELECT lease_epoch, last_event_attempt, last_event_seq
		FROM runs
		WHERE run_id = $1
		FOR UPDATE
	`, normalized.RunID).Scan(&leaseEpoch, &lastAttempt, &lastSeq)
	if errors.Is(err, pgx.ErrNoRows) {
		return AppendResult{}, ErrRunNotFound
	}
	if err != nil {
		return AppendResult{}, fmt.Errorf("append event: lock run: %w", err)
	}
	if normalized.LeaseEpoch != leaseEpoch {
		return AppendResult{Stale: true}, nil
	}

	result := AppendResult{}
	if normalized.Attempt == lastAttempt && normalized.Seq > lastSeq+1 {
		result.GapSize = normalized.Seq - lastSeq - 1
		result.Gap = result.GapSize > EventGapWindow
	}

	tag, err := tx.Exec(ctx, `
		INSERT INTO run_events (run_id, attempt, seq, source, type, payload, ts_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (run_id, attempt, seq) DO NOTHING
	`, normalized.RunID, normalized.Attempt, normalized.Seq, normalized.Source,
		normalized.Type, normalized.Payload, normalized.TSMs)
	if err != nil {
		return AppendResult{}, fmt.Errorf("append event: insert: %w", err)
	}
	result.Inserted = tag.RowsAffected() > 0
	if result.Inserted &&
		(normalized.Attempt > lastAttempt ||
			(normalized.Attempt == lastAttempt && normalized.Seq > lastSeq)) {
		if _, err := tx.Exec(ctx, `
			UPDATE runs
			SET last_event_attempt = $2,
			    last_event_seq = $3,
			    updated_at = now()
			WHERE run_id = $1
		`, normalized.RunID, normalized.Attempt, normalized.Seq); err != nil {
			return AppendResult{}, fmt.Errorf("append event: update run: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return AppendResult{}, fmt.Errorf("append event: commit: %w", err)
	}
	return result, nil
}

func eventLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

// ListEvents returns persisted events after (afterAttempt, afterSeq).
func (s *Store) ListEvents(
	ctx context.Context,
	runID string,
	afterAttempt int32,
	afterSeq int64,
	limit int,
) ([]Event, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("runstore: postgres pool is nil")
	}
	if runID == "" {
		return nil, fmt.Errorf("%w: run_id is required", ErrInvalidEvent)
	}
	if afterAttempt < 0 || afterSeq < 0 {
		return nil, fmt.Errorf("%w: after values must be non-negative", ErrInvalidEvent)
	}
	limit = eventLimit(limit)
	rows, err := s.pool.Query(ctx, `
		SELECT run_id, attempt, seq, source, type, payload, ts_ms
		FROM run_events
		WHERE run_id = $1
		  AND (attempt > $2 OR (attempt = $2 AND seq > $3))
		ORDER BY attempt, seq
		LIMIT $4
	`, runID, afterAttempt, afterSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	events := make([]Event, 0, limit)
	for rows.Next() {
		var event Event
		if err := rows.Scan(
			&event.RunID,
			&event.Attempt,
			&event.Seq,
			&event.Source,
			&event.Type,
			&event.Payload,
			&event.TSMs,
		); err != nil {
			return nil, fmt.Errorf("list events: scan: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	return events, nil
}

func normalizeEvent(event Event) (Event, error) {
	if event.Ephemeral {
		return Event{}, ErrEphemeralEvent
	}
	if event.RunID == "" {
		return Event{}, fmt.Errorf("%w: run_id is required", ErrInvalidEvent)
	}
	if event.Attempt < 1 {
		return Event{}, fmt.Errorf("%w: attempt must be >= 1", ErrInvalidEvent)
	}
	if event.Seq < 1 {
		return Event{}, fmt.Errorf("%w: seq must be >= 1", ErrInvalidEvent)
	}
	if event.LeaseEpoch < 0 {
		return Event{}, fmt.Errorf("%w: lease_epoch must not be negative", ErrInvalidEvent)
	}
	if event.Source != "daemon" && event.Source != "mw" {
		return Event{}, fmt.Errorf("%w: source must be daemon or mw", ErrInvalidEvent)
	}
	if event.Source == "daemon" && event.Seq >= MWSeqBase {
		return Event{}, fmt.Errorf("%w: daemon seq must be below %d", ErrInvalidEvent, MWSeqBase)
	}
	if event.Source == "mw" && event.Seq < MWSeqBase {
		return Event{}, fmt.Errorf("%w: mw seq must be at least %d", ErrInvalidEvent, MWSeqBase)
	}
	if event.Type == "" {
		return Event{}, fmt.Errorf("%w: type is required", ErrInvalidEvent)
	}
	var err error
	event.Payload, err = normalizeJSON(event.Payload)
	if err != nil {
		return Event{}, fmt.Errorf("%w: payload: %v", ErrInvalidEvent, err)
	}
	if event.TSMs <= 0 {
		event.TSMs = time.Now().UnixMilli()
	}
	return event, nil
}

func normalizeJSON(value json.RawMessage) (json.RawMessage, error) {
	if len(value) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(value) {
		return nil, fmt.Errorf("invalid JSON")
	}
	return value, nil
}
