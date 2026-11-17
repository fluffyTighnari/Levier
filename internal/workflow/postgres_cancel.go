// Levier — internal/workflow
//
// PostgreSQL run/task cancellation.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"context"
	"fmt"
)

const cancelRunSQL = `
	UPDATE run_tasks
	SET status = 'CANCELLED',
	    lease_owner = '',
	    lease_expires_at = NULL,
	    lease_token = CASE
	      WHEN status = 'LEASED' THEN lease_token + 1
	      ELSE lease_token
	    END,
	    last_error = $2,
	    updated_at = now()
	WHERE run_id = $1
	  AND status IN ('PENDING', 'LEASED')`

const cancelTaskSQL = `
	UPDATE run_tasks
	SET status = 'CANCELLED',
	    lease_owner = '',
	    lease_expires_at = NULL,
	    lease_token = CASE
	      WHEN status = 'LEASED' THEN lease_token + 1
	      ELSE lease_token
	    END,
	    last_error = $3,
	    updated_at = now()
	WHERE run_id = $1
	  AND task_type = $2
	  AND status IN ('PENDING', 'LEASED')`

// CancelRun cancels every active task for a run.
func (s *PostgresStore) CancelRun(ctx context.Context, runID, reason string) (int, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("workflow: postgres pool is nil")
	}
	if runID == "" {
		return 0, fmt.Errorf("%w: run_id is required", ErrInvalidTask)
	}
	if reason == "" {
		reason = "cancelled"
	}
	tag, err := s.pool.Exec(ctx, cancelRunSQL, runID, reason)
	if err != nil {
		return 0, fmt.Errorf("cancel run tasks: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// CancelTask cancels one active task type for a run.
func (s *PostgresStore) CancelTask(ctx context.Context, runID string, taskType TaskType, reason string) (int, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("workflow: postgres pool is nil")
	}
	if runID == "" {
		return 0, fmt.Errorf("%w: run_id is required", ErrInvalidTask)
	}
	if !taskType.Valid() {
		return 0, fmt.Errorf("%w: task type %q is invalid", ErrInvalidTask, taskType)
	}
	if reason == "" {
		reason = "cancelled"
	}
	tag, err := s.pool.Exec(ctx, cancelTaskSQL, runID, string(taskType), reason)
	if err != nil {
		return 0, fmt.Errorf("cancel task: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
