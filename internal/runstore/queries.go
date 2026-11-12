// Middleware — runstore
//
// Split from runs.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package runstore

import (
	"context"
	"fmt"
)

// ListQueuedRuns returns runs waiting for a daemon.
func (s *Store) ListQueuedRuns(ctx context.Context) ([]Run, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("runstore: postgres pool is nil")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+runColumns+`
		FROM runs
		WHERE status = 'queued'
		ORDER BY created_at
		LIMIT 1000
	`)
	if err != nil {
		return nil, fmt.Errorf("list queued runs: %w", err)
	}
	defer rows.Close()
	runs := make([]Run, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("list queued runs: scan: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list queued runs: %w", err)
	}
	return runs, nil
}

// ListActiveRuns returns runs in assigned or running state.
func (s *Store) ListActiveRuns(ctx context.Context) ([]Run, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("runstore: postgres pool is nil")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+runColumns+`
		FROM runs
		WHERE status IN ('assigned', 'running')
		ORDER BY run_id
	`)
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
