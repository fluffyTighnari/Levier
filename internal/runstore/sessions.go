// Levier — runstore
//
// Split from store.go (P8R.3).
// Copyright 2026 The Levier Authors.
package runstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// EnsureSession creates a session if absent and validates its ownership.
func (s *Store) EnsureSession(ctx context.Context, sessionID, appID, tenantID string) (Session, error) {
	if s == nil || s.pool == nil {
		return Session{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if sessionID == "" || appID == "" {
		return Session{}, fmt.Errorf("%w: session_id and app_id are required", ErrInvalidRun)
	}
	if tenantID == "" {
		tenantID = DefaultTenantID
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (session_id, app_id, tenant_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, 'active', now(), now())
		ON CONFLICT (session_id) DO NOTHING
	`, sessionID, appID, tenantID); err != nil {
		return Session{}, fmt.Errorf("ensure session: %w", err)
	}
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	if session.AppID != appID || session.TenantID != tenantID {
		return Session{}, ErrSessionConflict
	}
	if session.Status == "expired" {
		return Session{}, ErrSessionClosed
	}
	return session, nil
}

const sessionColumns = `
	session_id, app_id, tenant_id, status, current_run_id, event_attempt,
	event_watermark, created_at, updated_at, expires_at`

// GetSession returns one longterm session.
func (s *Store) GetSession(ctx context.Context, sessionID string) (Session, error) {
	if s == nil || s.pool == nil {
		return Session{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if sessionID == "" {
		return Session{}, fmt.Errorf("runstore: session_id is required")
	}
	session, err := scanSession(s.pool.QueryRow(ctx, `
		SELECT `+sessionColumns+`
		FROM sessions
		WHERE session_id = $1
	`, sessionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	return session, nil
}

// ListSessions returns sessions owned by one app in a tenant.
func (s *Store) ListSessions(ctx context.Context, appID, tenantID string, limit int) ([]Session, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("runstore: postgres pool is nil")
	}
	if appID == "" {
		return nil, fmt.Errorf("%w: app_id is required", ErrInvalidRun)
	}
	if tenantID == "" {
		tenantID = DefaultTenantID
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+sessionColumns+`
		FROM sessions
		WHERE app_id = $1 AND tenant_id = $2
		ORDER BY updated_at DESC, session_id DESC
		LIMIT $3
	`, appID, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()
	sessions := make([]Session, 0, limit)
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, fmt.Errorf("list sessions: scan: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list sessions: rows: %w", err)
	}
	return sessions, nil
}

// CloseSession marks a session expired when it has no active runs.
func (s *Store) CloseSession(ctx context.Context, sessionID, appID, tenantID string) (Session, error) {
	if s == nil || s.pool == nil {
		return Session{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if sessionID == "" || appID == "" {
		return Session{}, fmt.Errorf("%w: session_id and app_id are required", ErrInvalidRun)
	}
	if tenantID == "" {
		tenantID = DefaultTenantID
	}
	session, err := scanSession(s.pool.QueryRow(ctx, `
		UPDATE sessions s
		SET status = 'expired',
		    expires_at = now(),
		    updated_at = now()
		WHERE s.session_id = $1
		  AND s.app_id = $2
		  AND s.tenant_id = $3
		  AND s.status <> 'expired'
		  AND NOT EXISTS (
		    SELECT 1
		    FROM runs r
		    WHERE r.session_id = s.session_id
		      AND r.status IN ('queued', 'assigned', 'running', 'waiting')
		  )
		RETURNING `+sessionColumns,
		sessionID, appID, tenantID,
	))
	if err == nil {
		return session, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Session{}, fmt.Errorf("close session: %w", err)
	}

	current, getErr := s.GetSession(ctx, sessionID)
	if errors.Is(getErr, ErrSessionNotFound) {
		return Session{}, ErrSessionNotFound
	}
	if getErr != nil {
		return Session{}, getErr
	}
	if current.AppID != appID || current.TenantID != tenantID {
		return Session{}, ErrSessionNotFound
	}
	if current.Status == "expired" {
		return current, nil
	}
	var active bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM runs
			WHERE session_id = $1
			  AND status IN ('queued', 'assigned', 'running', 'waiting')
		)
	`, sessionID).Scan(&active); err != nil {
		return Session{}, fmt.Errorf("close session: check active runs: %w", err)
	}
	if active {
		return Session{}, ErrSessionActive
	}
	return Session{}, fmt.Errorf("close session: concurrent update")
}

func scanSession(row pgx.Row) (Session, error) {
	var session Session
	var currentRunID pgtype.Text
	var expiresAt pgtype.Timestamptz
	if err := row.Scan(
		&session.SessionID,
		&session.AppID,
		&session.TenantID,
		&session.Status,
		&currentRunID,
		&session.EventAttempt,
		&session.EventWatermark,
		&session.CreatedAt,
		&session.UpdatedAt,
		&expiresAt,
	); err != nil {
		return Session{}, err
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
		SELECT e.run_id, e.attempt, e.seq, e.source, e.type, e.name, e.schema_version, e.payload, e.ts_ms
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
			&event.Name,
			&event.SchemaVersion,
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
