// Middleware — runstore
//
// Split from store.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package runstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// GetSession returns one longterm session.
func (s *Store) GetSession(ctx context.Context, sessionID string) (Session, error) {
	if s == nil || s.pool == nil {
		return Session{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if sessionID == "" {
		return Session{}, fmt.Errorf("runstore: session_id is required")
	}
	var session Session
	var currentRunID pgtype.Text
	var expiresAt pgtype.Timestamptz
	err := s.pool.QueryRow(ctx, `
		SELECT session_id, app_id, status, current_run_id, event_attempt,
		       event_watermark, created_at, updated_at, expires_at
		FROM sessions
		WHERE session_id = $1
	`, sessionID).Scan(
		&session.SessionID,
		&session.AppID,
		&session.Status,
		&currentRunID,
		&session.EventAttempt,
		&session.EventWatermark,
		&session.CreatedAt,
		&session.UpdatedAt,
		&expiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	if currentRunID.Valid {
		session.CurrentRunID = currentRunID.String
	}
	if expiresAt.Valid {
		value := expiresAt.Time
		session.ExpiresAt = &value
	}
	return session, nil
}

// ListSessionEvents returns session events across runs in stable order.
func (s *Store) ListSessionEvents(
	ctx context.Context,
	sessionID string,
	afterAttempt int32,
	afterSeq int64,
	limit int,
) ([]Event, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("runstore: postgres pool is nil")
	}
	if sessionID == "" {
		return nil, fmt.Errorf("%w: session_id is required", ErrInvalidEvent)
	}
	if afterAttempt < 0 || afterSeq < 0 {
		return nil, fmt.Errorf("%w: after values must be non-negative", ErrInvalidEvent)
	}
	limit = eventLimit(limit)
	rows, err := s.pool.Query(ctx, `
		SELECT e.run_id, e.attempt, e.seq, e.source, e.type, e.payload, e.ts_ms
		FROM run_events e
		JOIN runs r ON r.run_id = e.run_id
		WHERE r.session_id = $1
		  AND (e.attempt > $2 OR (e.attempt = $2 AND e.seq > $3))
		ORDER BY r.created_at, r.run_id, e.attempt, e.seq
		LIMIT $4
	`, sessionID, afterAttempt, afterSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("list session events: %w", err)
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
			return nil, fmt.Errorf("list session events: scan: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list session events: %w", err)
	}
	return events, nil
}
