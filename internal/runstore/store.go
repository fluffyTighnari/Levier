// Middleware — internal/runstore
//
// PostgreSQL RunStore for apps, runs, and run events.
//
// Copyright 2026 The Middleware Authors.
package runstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const appColumns = `
	app_id, name, api_key_hash, multiagent, status, created_at, updated_at`

const runColumns = `
	run_id, app_id, idempotency_key, request_hash, runtime, strategy,
	params_json, status, attempt, session_id, model_json, run_timeout_ms,
	trace_id, assigned_daemon_id, lease_epoch, assigned_at,
	last_event_attempt, last_event_seq, created_at, updated_at`

const insertRunSQL = `
	INSERT INTO runs (
		run_id, app_id, idempotency_key, request_hash, runtime, strategy,
		params_json, status, attempt, session_id, model_json, run_timeout_ms,
		trace_id, assigned_daemon_id, lease_epoch, assigned_at,
		last_event_attempt, last_event_seq, created_at, updated_at
	) VALUES (
		$1, $2, $3, $4, $5, $6,
		$7, $8, $9, NULLIF($10, ''), $11, $12,
		$13, NULLIF($14, ''), $15, $16,
		$17, $18, $19, $20
	)
	ON CONFLICT (app_id, idempotency_key)
		WHERE idempotency_key <> ''
		DO NOTHING
	RETURNING ` + runColumns

// Store persists apps, runs, and events in PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// New returns a PostgreSQL-backed RunStore.
func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateApp upserts an app by app_id.
func (s *Store) CreateApp(ctx context.Context, app App) (App, error) {
	if s == nil || s.pool == nil {
		return App{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if app.AppID == "" || app.Name == "" || app.APIKeyHash == "" {
		return App{}, fmt.Errorf("runstore: app_id, name, and api_key_hash are required")
	}
	if app.Status == "" {
		app.Status = "active"
	}
	now := time.Now().UTC()
	row := s.pool.QueryRow(ctx, `
		INSERT INTO apps (app_id, name, api_key_hash, multiagent, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (app_id) DO UPDATE SET
			name = EXCLUDED.name,
			api_key_hash = EXCLUDED.api_key_hash,
			multiagent = EXCLUDED.multiagent,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at
		RETURNING `+appColumns,
		app.AppID, app.Name, app.APIKeyHash, app.Multiagent, app.Status, now, now,
	)
	created, err := scanApp(row)
	if err != nil {
		return App{}, fmt.Errorf("create app: %w", err)
	}
	return created, nil
}

// GetApp returns one app by app_id.
func (s *Store) GetApp(ctx context.Context, appID string) (App, error) {
	if s == nil || s.pool == nil {
		return App{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	app, err := scanApp(s.pool.QueryRow(ctx,
		"SELECT "+appColumns+" FROM apps WHERE app_id = $1",
		appID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return App{}, ErrAppNotFound
	}
	if err != nil {
		return App{}, fmt.Errorf("get app: %w", err)
	}
	return app, nil
}

// CreateRun inserts a run or returns the existing run for the same
// (app_id, idempotency_key).
func (s *Store) CreateRun(ctx context.Context, run Run) (Run, error) {
	if s == nil || s.pool == nil {
		return Run{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	normalized, err := normalizeRun(run)
	if err != nil {
		return Run{}, err
	}

	for attempt := 0; attempt < 2; attempt++ {
		inserted, err := scanRun(s.pool.QueryRow(ctx, insertRunSQL,
			normalized.RunID,
			normalized.AppID,
			normalized.IdempotencyKey,
			normalized.RequestHash,
			normalized.Runtime,
			normalized.Strategy,
			normalized.Params,
			normalized.Status,
			normalized.Attempt,
			normalized.SessionID,
			normalized.Model,
			normalized.RunTimeoutMS,
			normalized.TraceID,
			normalized.AssignedDaemonID,
			normalized.LeaseEpoch,
			normalized.AssignedAt,
			normalized.LastEventAttempt,
			normalized.LastEventSeq,
			normalized.CreatedAt,
			normalized.UpdatedAt,
		))
		if err == nil {
			return inserted, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Run{}, fmt.Errorf("create run: %w", err)
		}

		existing, lookupErr := s.getRunByIdempotency(
			ctx,
			normalized.AppID,
			normalized.IdempotencyKey,
		)
		if lookupErr == nil {
			if existing.RequestHash != normalized.RequestHash {
				return Run{}, ErrIdempotencyConflict
			}
			return existing, nil
		}
		if !errors.Is(lookupErr, ErrRunNotFound) {
			return Run{}, lookupErr
		}
	}
	return Run{}, fmt.Errorf("create run: idempotency race")
}

// GetRun returns one run by run_id.
func (s *Store) GetRun(ctx context.Context, runID string) (Run, error) {
	if s == nil || s.pool == nil {
		return Run{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	run, err := scanRun(s.pool.QueryRow(ctx,
		"SELECT "+runColumns+" FROM runs WHERE run_id = $1",
		runID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrRunNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("get run: %w", err)
	}
	return run, nil
}

// AppendEvent inserts one event idempotently and advances the run projection.
// It returns false when the event already exists.
func (s *Store) AppendEvent(ctx context.Context, event Event) (bool, error) {
	if s == nil || s.pool == nil {
		return false, fmt.Errorf("runstore: postgres pool is nil")
	}
	normalized, err := normalizeEvent(event)
	if err != nil {
		return false, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("append event: begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	tag, err := tx.Exec(ctx, `
		INSERT INTO run_events (run_id, attempt, seq, source, type, payload, ts_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (run_id, attempt, seq) DO NOTHING
	`, normalized.RunID, normalized.Attempt, normalized.Seq, normalized.Source,
		normalized.Type, normalized.Payload, normalized.TSMs)
	if err != nil {
		return false, fmt.Errorf("append event: insert: %w", err)
	}
	inserted := tag.RowsAffected() > 0
	if inserted {
		if _, err := tx.Exec(ctx, `
			UPDATE runs
			SET last_event_attempt = $2,
			    last_event_seq = $3,
			    updated_at = now()
			WHERE run_id = $1
			  AND (last_event_attempt < $2
			       OR (last_event_attempt = $2 AND last_event_seq < $3))
		`, normalized.RunID, normalized.Attempt, normalized.Seq); err != nil {
			return false, fmt.Errorf("append event: update run: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("append event: commit: %w", err)
	}
	return inserted, nil
}

func (s *Store) getRunByIdempotency(ctx context.Context, appID, key string) (Run, error) {
	run, err := scanRun(s.pool.QueryRow(ctx,
		"SELECT "+runColumns+" FROM runs WHERE app_id = $1 AND idempotency_key = $2",
		appID, key,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, ErrRunNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("get run by idempotency: %w", err)
	}
	return run, nil
}

func normalizeRun(run Run) (Run, error) {
	if run.RunID == "" {
		return Run{}, fmt.Errorf("%w: run_id is required", ErrInvalidRun)
	}
	if run.AppID == "" {
		return Run{}, fmt.Errorf("%w: app_id is required", ErrInvalidRun)
	}
	if run.Status == "" {
		run.Status = "queued"
	}
	if run.Runtime == "" {
		run.Runtime = "normal"
	}
	if run.Strategy == "" {
		run.Strategy = "react"
	}
	if run.RunTimeoutMS <= 0 {
		run.RunTimeoutMS = 1_800_000
	}
	if run.TraceID == "" {
		run.TraceID = run.RunID
	}
	if run.Attempt < 0 || run.LeaseEpoch < 0 || run.LastEventAttempt < 0 || run.LastEventSeq < 0 {
		return Run{}, fmt.Errorf("%w: counters must be non-negative", ErrInvalidRun)
	}
	var err error
	run.Params, err = normalizeJSON(run.Params)
	if err != nil {
		return Run{}, fmt.Errorf("%w: params: %v", ErrInvalidRun, err)
	}
	run.Model, err = normalizeJSON(run.Model)
	if err != nil {
		return Run{}, fmt.Errorf("%w: model: %v", ErrInvalidRun, err)
	}
	now := time.Now().UTC()
	run.CreatedAt = now
	run.UpdatedAt = now
	return run, nil
}

func normalizeEvent(event Event) (Event, error) {
	if event.RunID == "" {
		return Event{}, fmt.Errorf("%w: run_id is required", ErrInvalidEvent)
	}
	if event.Attempt < 1 {
		return Event{}, fmt.Errorf("%w: attempt must be >= 1", ErrInvalidEvent)
	}
	if event.Seq < 1 {
		return Event{}, fmt.Errorf("%w: seq must be >= 1", ErrInvalidEvent)
	}
	if event.Source != "daemon" && event.Source != "mw" {
		return Event{}, fmt.Errorf("%w: source must be daemon or mw", ErrInvalidEvent)
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

func scanApp(row pgx.Row) (App, error) {
	var app App
	if err := row.Scan(
		&app.AppID,
		&app.Name,
		&app.APIKeyHash,
		&app.Multiagent,
		&app.Status,
		&app.CreatedAt,
		&app.UpdatedAt,
	); err != nil {
		return App{}, err
	}
	return app, nil
}

func scanRun(row pgx.Row) (Run, error) {
	var run Run
	var params, model []byte
	var sessionID, assignedDaemon pgtype.Text
	var assignedAt pgtype.Timestamptz
	if err := row.Scan(
		&run.RunID,
		&run.AppID,
		&run.IdempotencyKey,
		&run.RequestHash,
		&run.Runtime,
		&run.Strategy,
		&params,
		&run.Status,
		&run.Attempt,
		&sessionID,
		&model,
		&run.RunTimeoutMS,
		&run.TraceID,
		&assignedDaemon,
		&run.LeaseEpoch,
		&assignedAt,
		&run.LastEventAttempt,
		&run.LastEventSeq,
		&run.CreatedAt,
		&run.UpdatedAt,
	); err != nil {
		return Run{}, err
	}
	run.Params = json.RawMessage(params)
	run.Model = json.RawMessage(model)
	if sessionID.Valid {
		run.SessionID = sessionID.String
	}
	if assignedDaemon.Valid {
		run.AssignedDaemonID = assignedDaemon.String
	}
	if assignedAt.Valid {
		value := assignedAt.Time
		run.AssignedAt = &value
	}
	return run, nil
}
