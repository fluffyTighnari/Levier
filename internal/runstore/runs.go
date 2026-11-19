// Levier — runstore
//
// Split from runs.go (P8R.3).
// Copyright 2026 The Levier Authors.
package runstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const runColumns = `
	run_id, app_id, idempotency_key, request_hash, runtime, runtime_name,
	params_json, input_json, status, attempt, session_id, model_json, run_timeout_ms,
	tool_timeout_ms, trace_id, assigned_daemon_id, lease_epoch, assigned_at,
	last_event_attempt, last_event_seq, created_at, updated_at`

const insertRunSQL = `
	INSERT INTO runs (
		run_id, app_id, idempotency_key, request_hash, runtime, runtime_name,
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
	return createRun(ctx, s.pool, normalized)
}

// TxFunc runs inside a CreateRunWithTx transaction. The created or existing
// run is passed so callers can enqueue tasks for the right RunID.
type TxFunc func(ctx context.Context, tx pgx.Tx, run Run) error

// CreateRunWithTx inserts a run and invokes after in the same transaction.
// Returning an error from after rolls back the run insert.
func (s *Store) CreateRunWithTx(ctx context.Context, run Run, after TxFunc) (Run, error) {
	if s == nil || s.pool == nil {
		return Run{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	normalized, err := normalizeRun(run)
	if err != nil {
		return Run{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Run{}, fmt.Errorf("create run: begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	created, err := createRun(ctx, tx, normalized)
	if err != nil {
		return Run{}, err
	}
	if after != nil {
		if err := after(ctx, tx, created); err != nil {
			return Run{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Run{}, fmt.Errorf("create run: commit: %w", err)
	}
	return created, nil
}

func createRun(ctx context.Context, q querier, normalized Run) (Run, error) {
	for attempt := 0; attempt < 2; attempt++ {
		inserted, err := scanRun(q.QueryRow(ctx, insertRunSQL,
			normalized.RunID,
			normalized.AppID,
			normalized.IdempotencyKey,
			normalized.RequestHash,
			normalized.Runtime,
			normalized.RuntimeName,
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

		existing, lookupErr := getRunByIdempotency(ctx, q,
			normalized.AppID, normalized.IdempotencyKey)
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
		  AND NOT EXISTS (
		    SELECT 1
		    FROM run_events e
		    WHERE e.run_id = runs.run_id
		      AND e.attempt = runs.attempt
		      AND e.type = 'RUN_STATUS'
		      AND lower(coalesce(e.payload->>'status', '')) IN
		          ('completed', 'failed', 'cancelled', 'timeout')
		  )
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
	if run.RuntimeName == "" {
		return Run{}, fmt.Errorf("%w: runtime_name is required", ErrInvalidRun)
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
		&run.RuntimeName,
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
