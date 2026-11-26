// Levier — internal/ops/maintenance
//
// Conservative terminal-data pruning.
//
// Copyright 2026 The Levier Authors.
package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const expireSessionsSQL = `
	UPDATE sessions s
	SET status = 'expired', updated_at = now()
	WHERE s.status IN ('active', 'paused')
	  AND s.expires_at IS NOT NULL
	  AND s.expires_at < $1
	  AND NOT EXISTS (
		SELECT 1
		FROM runs r
		WHERE (r.session_id = s.session_id OR (s.current_run_id <> '' AND r.run_id = s.current_run_id))
		  AND r.status IN ('queued', 'assigned', 'running', 'waiting')
	  )`

const deleteRunsSQL = `
	DELETE FROM runs
	WHERE run_id IN (
		SELECT run_id
		FROM runs
		WHERE status IN ('completed', 'failed', 'cancelled', 'timeout')
		  AND updated_at < $1
		ORDER BY updated_at
		LIMIT $2
	)`

const deleteSessionsSQL = `
	DELETE FROM sessions s
	WHERE s.session_id IN (
		SELECT s.session_id
		FROM sessions s
		WHERE s.status = 'expired'
		  AND s.expires_at IS NOT NULL
		  AND s.expires_at < $1
		  AND NOT EXISTS (
			SELECT 1
			FROM runs r
			WHERE (r.session_id = s.session_id OR (s.current_run_id <> '' AND r.run_id = s.current_run_id))
			  AND r.status IN ('queued', 'assigned', 'running', 'waiting')
		  )
		ORDER BY s.expires_at
		LIMIT $2
	)`

// Result describes a dry-run plan or the outcome of an executed prune.
type Result struct {
	DryRun           bool      `json:"dry_run"`
	Enabled          bool      `json:"enabled"`
	RunDays          int       `json:"run_days"`
	SessionDays      int       `json:"session_days"`
	BatchSize        int       `json:"batch_size"`
	RunCutoff        time.Time `json:"run_cutoff"`
	SessionCutoff    time.Time `json:"session_cutoff"`
	RunsEligible     int64     `json:"runs_eligible"`
	RunsDeleted      int64     `json:"runs_deleted"`
	SessionsEligible int64     `json:"sessions_eligible"`
	SessionsExpired  int64     `json:"sessions_expired"`
	SessionsDeleted  int64     `json:"sessions_deleted"`
	Batches          int       `json:"batches"`
}

// Prune returns candidates in dry-run mode. In execute mode it first marks
// expired sessions without active runs, then deletes eligible sessions and
// terminal runs in batches. A run delete also cascades to its events, tasks,
// and tool invocations.
func Prune(ctx context.Context, pool *pgxpool.Pool, policy Policy, execute bool) (Result, error) {
	if pool == nil {
		return Result{}, fmt.Errorf("maintenance: database pool is nil")
	}
	normalized, err := policy.Normalize()
	if err != nil {
		return Result{}, err
	}
	if execute && !normalized.Enabled {
		return Result{}, ErrRetentionDisabled
	}

	status, err := GetStatus(ctx, pool, normalized)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		DryRun:           !execute,
		Enabled:          normalized.Enabled,
		RunDays:          normalized.RunDays,
		SessionDays:      normalized.SessionDays,
		BatchSize:        normalized.BatchSize,
		RunCutoff:        status.RunCutoff,
		SessionCutoff:    status.SessionCutoff,
		RunsEligible:     status.RunsEligible,
		SessionsEligible: status.SessionsEligible,
	}
	if !execute {
		return result, nil
	}

	expired, err := pool.Exec(ctx, expireSessionsSQL, result.SessionCutoff)
	if err != nil {
		return Result{}, fmt.Errorf("expire sessions: %w", err)
	}
	result.SessionsExpired = expired.RowsAffected()

	for {
		tag, err := pool.Exec(ctx, deleteRunsSQL, result.RunCutoff, result.BatchSize)
		if err != nil {
			return Result{}, fmt.Errorf("delete terminal runs: %w", err)
		}
		deleted := tag.RowsAffected()
		if deleted == 0 {
			break
		}
		result.RunsDeleted += deleted
		result.Batches++
	}

	for {
		tag, err := pool.Exec(ctx, deleteSessionsSQL, result.SessionCutoff, result.BatchSize)
		if err != nil {
			return Result{}, fmt.Errorf("delete expired sessions: %w", err)
		}
		deleted := tag.RowsAffected()
		if deleted == 0 {
			break
		}
		result.SessionsDeleted += deleted
		result.Batches++
	}
	return result, nil
}
