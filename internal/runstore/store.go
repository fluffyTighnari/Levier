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

// MWSeqBase is the first seq reserved for middleware-generated events.
const MWSeqBase int64 = 1 << 62

// EventGapWindow is the maximum accepted out-of-order seq gap.
const EventGapWindow int64 = 16

const appColumns = `
	app_id, name, api_key_hash, multiagent, status, created_at, updated_at`

const runColumns = `
	run_id, app_id, idempotency_key, request_hash, runtime, strategy,
	params_json, input_json, status, attempt, session_id, model_json, run_timeout_ms,
	tool_timeout_ms, trace_id, assigned_daemon_id, lease_epoch, assigned_at,
	last_event_attempt, last_event_seq, created_at, updated_at`

const insertRunSQL = `
	INSERT INTO runs (
		run_id, app_id, idempotency_key, request_hash, runtime, strategy,
		params_json, input_json, status, attempt, session_id, model_json, run_timeout_ms,
		tool_timeout_ms, trace_id, assigned_daemon_id, lease_epoch, assigned_at,
		last_event_attempt, last_event_seq, created_at, updated_at
	) VALUES (
		$1, $2, $3, $4, $5, $6,
		$7, $8, $9, $10, NULLIF($11, ''), $12, $13,
		$14, $15, NULLIF($16, ''), $17, $18, $19,
		$20, $21, $22
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
			normalized.Input,
			normalized.Status,
			normalized.Attempt,
			normalized.SessionID,
			normalized.Model,
			normalized.RunTimeoutMS,
			normalized.ToolTimeoutMS,
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

// Assignment moves a run to assigned with CAS fencing.
type Assignment struct {
	RunID              string
	ExpectedStatus     string
	ExpectedAttempt    int
	ExpectedLeaseEpoch int64
	Attempt            int
	LeaseEpoch         int64
	DaemonID           string
}

// Transition applies a CAS status transition.
type Transition struct {
	RunID              string
	ExpectedStatus     string
	ToStatus           string
	ExpectedAttempt    int
	ExpectedLeaseEpoch int64
	AttemptDelta       int
}

// AssignRun moves a run to assigned and records the daemon/attempt fencing.
func (s *Store) AssignRun(ctx context.Context, assignment Assignment) (Run, error) {
	if s == nil || s.pool == nil {
		return Run{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if assignment.RunID == "" || assignment.DaemonID == "" ||
		assignment.ExpectedStatus == "" || assignment.ExpectedAttempt < 0 ||
		assignment.ExpectedLeaseEpoch < 0 || assignment.Attempt < 1 ||
		assignment.LeaseEpoch < 1 {
		return Run{}, fmt.Errorf("%w: invalid assignment", ErrInvalidRun)
	}
	if assignment.ExpectedStatus != StatusQueued && assignment.ExpectedStatus != StatusAssigned {
		return Run{}, fmt.Errorf("%w: cannot assign from %s", ErrRunNotAssignable, assignment.ExpectedStatus)
	}
	run, err := scanRun(s.pool.QueryRow(ctx, `
		UPDATE runs
		SET status = 'assigned',
		    attempt = $5,
		    lease_epoch = $6,
		    assigned_daemon_id = $7,
		    assigned_at = now(),
		    updated_at = now()
		WHERE run_id = $1
		  AND status = $2
		  AND attempt = $3
		  AND lease_epoch = $4
		  AND status IN ('queued', 'assigned')
		RETURNING `+runColumns,
		assignment.RunID, assignment.ExpectedStatus, assignment.ExpectedAttempt,
		assignment.ExpectedLeaseEpoch, assignment.Attempt, assignment.LeaseEpoch,
		assignment.DaemonID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := s.GetRun(ctx, assignment.RunID); errors.Is(getErr, ErrRunNotFound) {
			return Run{}, ErrRunNotFound
		}
		return Run{}, ErrRunNotAssignable
	}
	if err != nil {
		return Run{}, fmt.Errorf("assign run: %w", err)
	}
	return run, nil
}

// Transition applies a CAS status transition. Terminal runs never transition.
func (s *Store) Transition(ctx context.Context, transition Transition) (Run, error) {
	if s == nil || s.pool == nil {
		return Run{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if transition.RunID == "" || transition.ExpectedStatus == "" || transition.ToStatus == "" ||
		transition.ExpectedAttempt < 0 || transition.ExpectedLeaseEpoch < 0 ||
		transition.AttemptDelta < 0 {
		return Run{}, fmt.Errorf("%w: invalid transition", ErrInvalidRun)
	}
	if TerminalStatus(transition.ExpectedStatus) {
		return Run{}, fmt.Errorf("%w: current status is terminal", ErrRunNotAssignable)
	}
	if !canTransition(transition.ExpectedStatus, transition.ToStatus) {
		return Run{}, fmt.Errorf(
			"%w: %s -> %s",
			ErrRunNotAssignable,
			transition.ExpectedStatus,
			transition.ToStatus,
		)
	}
	run, err := scanRun(s.pool.QueryRow(ctx, `
		UPDATE runs
		SET status = $5,
		    attempt = attempt + $6,
		    updated_at = now()
		WHERE run_id = $1
		  AND status = $2
		  AND attempt = $3
		  AND lease_epoch = $4
		  AND status NOT IN ('completed', 'failed', 'cancelled', 'timeout')
		RETURNING `+runColumns,
		transition.RunID, transition.ExpectedStatus, transition.ExpectedAttempt,
		transition.ExpectedLeaseEpoch, transition.ToStatus, transition.AttemptDelta,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := s.GetRun(ctx, transition.RunID); errors.Is(getErr, ErrRunNotFound) {
			return Run{}, ErrRunNotFound
		}
		return Run{}, ErrRunNotAssignable
	}
	if err != nil {
		return Run{}, fmt.Errorf("transition run: %w", err)
	}
	return run, nil
}

// ListActiveRunsForDaemon returns runs currently assigned to a daemon.
func (s *Store) ListActiveRunsForDaemon(ctx context.Context, daemonID string) ([]Run, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("runstore: postgres pool is nil")
	}
	if daemonID == "" {
		return nil, fmt.Errorf("%w: daemon_id is required", ErrInvalidRun)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+runColumns+`
		FROM runs
		WHERE assigned_daemon_id = $1
		  AND status IN ('assigned', 'running')
		ORDER BY run_id
	`, daemonID)
	if err != nil {
		return nil, fmt.Errorf("list active runs: %w", err)
	}
	defer rows.Close()
	runs := make([]Run, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list active runs: scan: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list active runs: %w", err)
	}
	return runs, nil
}

// RequeueRun moves an assigned/running run back to queued with a new epoch.
func (s *Store) RequeueRun(ctx context.Context, runID string) (Run, error) {
	if s == nil || s.pool == nil {
		return Run{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	if runID == "" {
		return Run{}, fmt.Errorf("%w: run_id is required", ErrInvalidRun)
	}
	run, err := scanRun(s.pool.QueryRow(ctx, `
		UPDATE runs
		SET status = 'queued',
		    lease_epoch = lease_epoch + 1,
		    assigned_daemon_id = NULL,
		    assigned_at = NULL,
		    updated_at = now()
		WHERE run_id = $1
		  AND status IN ('assigned', 'running')
		RETURNING `+runColumns,
		runID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		if _, getErr := s.GetRun(ctx, runID); errors.Is(getErr, ErrRunNotFound) {
			return Run{}, ErrRunNotFound
		}
		return Run{}, ErrRunNotAssignable
	}
	if err != nil {
		return Run{}, fmt.Errorf("requeue run: %w", err)
	}
	return run, nil
}

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
	if run.ToolTimeoutMS <= 0 {
		run.ToolTimeoutMS = 60_000
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
	if len(run.Input) == 0 {
		run.Input = json.RawMessage(`[]`)
	}
	if !json.Valid(run.Input) {
		return Run{}, fmt.Errorf("%w: input is invalid JSON", ErrInvalidRun)
	}
	now := time.Now().UTC()
	run.CreatedAt = now
	run.UpdatedAt = now
	return run, nil
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
		&run.Input,
		&run.Status,
		&run.Attempt,
		&sessionID,
		&model,
		&run.RunTimeoutMS,
		&run.ToolTimeoutMS,
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
