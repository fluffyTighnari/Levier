// Levier — internal/ops/maintenance
//
// Read-only retention status.
//
// Copyright 2026 The Levier Authors.
package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const runStatsSQL = `
	SELECT
		count(*),
		count(*) FILTER (WHERE status IN ('queued', 'assigned', 'running', 'waiting')),
		count(*) FILTER (WHERE status IN ('completed', 'failed', 'cancelled', 'timeout')),
		count(*) FILTER (
			WHERE status IN ('completed', 'failed', 'cancelled', 'timeout')
			  AND updated_at < $1
		),
		min(updated_at) FILTER (
			WHERE status IN ('completed', 'failed', 'cancelled', 'timeout')
		)
	FROM runs`

const sessionStatsSQL = `
	SELECT
		count(*),
		count(*) FILTER (WHERE s.status = 'active'),
		count(*) FILTER (WHERE s.status = 'paused'),
		count(*) FILTER (WHERE s.status = 'expired'),
		count(*) FILTER (
			WHERE s.expires_at IS NOT NULL
			  AND s.expires_at < $1
			  AND NOT EXISTS (
				SELECT 1
				FROM runs r
				WHERE (r.session_id = s.session_id OR (s.current_run_id <> '' AND r.run_id = s.current_run_id))
				  AND r.status IN ('queued', 'assigned', 'running', 'waiting')
			  )
		)
	FROM sessions s`

// Status is a read-only view of the data affected by retention.
type Status struct {
	RunsTotal           int64            `json:"runs_total"`
	RunsActive          int64            `json:"runs_active"`
	RunsTerminal        int64            `json:"runs_terminal"`
	RunsEligible        int64            `json:"runs_eligible"`
	OldestTerminalAt    *time.Time       `json:"oldest_terminal_at,omitempty"`
	EventsTotal         int64            `json:"events_total"`
	TasksByStatus       map[string]int64 `json:"tasks_by_status"`
	InvocationsByStatus map[string]int64 `json:"invocations_by_status"`
	SessionsTotal       int64            `json:"sessions_total"`
	SessionsActive      int64            `json:"sessions_active"`
	SessionsPaused      int64            `json:"sessions_paused"`
	SessionsExpired     int64            `json:"sessions_expired"`
	SessionsEligible    int64            `json:"sessions_eligible"`
	RunCutoff           time.Time        `json:"run_cutoff"`
	SessionCutoff       time.Time        `json:"session_cutoff"`
}

// GetStatus queries counts without modifying data.
func GetStatus(ctx context.Context, pool *pgxpool.Pool, policy Policy) (Status, error) {
	if pool == nil {
		return Status{}, fmt.Errorf("maintenance: database pool is nil")
	}
	normalized, err := policy.Normalize()
	if err != nil {
		return Status{}, err
	}
	now := time.Now()
	status := Status{
		TasksByStatus:       make(map[string]int64),
		InvocationsByStatus: make(map[string]int64),
		RunCutoff:           normalized.runCutoff(now),
		SessionCutoff:       normalized.sessionCutoff(now),
	}

	var oldestTerminal pgtype.Timestamptz
	if err := pool.QueryRow(ctx, runStatsSQL, status.RunCutoff).Scan(
		&status.RunsTotal,
		&status.RunsActive,
		&status.RunsTerminal,
		&status.RunsEligible,
		&oldestTerminal,
	); err != nil {
		return Status{}, fmt.Errorf("query run retention status: %w", err)
	}
	if oldestTerminal.Valid {
		value := oldestTerminal.Time
		status.OldestTerminalAt = &value
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM run_events").Scan(&status.EventsTotal); err != nil {
		return Status{}, fmt.Errorf("query run events count: %w", err)
	}
	if err := queryStatusCounts(ctx, pool, "run_tasks", status.TasksByStatus); err != nil {
		return Status{}, err
	}
	if err := queryStatusCounts(ctx, pool, "tool_invocations", status.InvocationsByStatus); err != nil {
		return Status{}, err
	}
	if err := pool.QueryRow(ctx, sessionStatsSQL, status.SessionCutoff).Scan(
		&status.SessionsTotal,
		&status.SessionsActive,
		&status.SessionsPaused,
		&status.SessionsExpired,
		&status.SessionsEligible,
	); err != nil {
		return Status{}, fmt.Errorf("query session retention status: %w", err)
	}
	return status, nil
}

func queryStatusCounts(ctx context.Context, pool *pgxpool.Pool, table string, target map[string]int64) error {
	// table is a package-controlled constant, never user input.
	rows, err := pool.Query(ctx, "SELECT status, count(*) FROM "+table+" GROUP BY status")
	if err != nil {
		return fmt.Errorf("query %s status: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return fmt.Errorf("scan %s status: %w", table, err)
		}
		target[status] = count
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read %s status: %w", table, err)
	}
	return nil
}
