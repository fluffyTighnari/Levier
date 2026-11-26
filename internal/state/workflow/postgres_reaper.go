// Levier — internal/state/workflow
//
// PostgreSQL expired-lease reaper and dead-letter queries.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"context"
	"fmt"
	"time"
)

const reapExpiredSQL = `
	WITH expired AS (
		SELECT task_id
		FROM run_tasks
		WHERE status = 'LEASED'
		  AND lease_expires_at <= $1
		ORDER BY lease_expires_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT $2
	)
	UPDATE run_tasks t
	SET status = CASE
	      WHEN t.attempt >= t.max_attempts THEN 'DEAD'
	      ELSE 'PENDING'
	    END,
	    available_at = CASE
	      WHEN t.attempt >= t.max_attempts THEN t.available_at
	      ELSE $1
	    END,
	    lease_owner = '',
	    lease_expires_at = NULL,
	    lease_token = t.lease_token + 1,
	    last_error = 'lease_expired',
	    updated_at = now()
	FROM expired e
	WHERE t.task_id = e.task_id`

const deadTaskSQL = `
	SELECT ` + taskColumns + `
	FROM run_tasks
	WHERE status = 'DEAD'
	ORDER BY updated_at DESC
	LIMIT $1`

// ReapExpired returns expired leases to PENDING or DEAD.
func (s *PostgresStore) ReapExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("workflow: postgres pool is nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if limit <= 0 {
		limit = 100
	}
	tag, err := s.pool.Exec(ctx, reapExpiredSQL, now, limit)
	if err != nil {
		return 0, fmt.Errorf("reap expired tasks: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// ListDead returns dead-letter tasks ordered by most recently updated.
func (s *PostgresStore) ListDead(ctx context.Context, limit int) ([]Task, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("workflow: postgres pool is nil")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, deadTaskSQL, limit)
	if err != nil {
		return nil, fmt.Errorf("list dead tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]Task, 0, limit)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("scan dead task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan dead tasks: %w", err)
	}
	return tasks, nil
}

// Stats returns queue depth and oldest pending age.
func (s *PostgresStore) Stats(ctx context.Context) (Stats, error) {
	if s == nil || s.pool == nil {
		return Stats{}, fmt.Errorf("workflow: postgres pool is nil")
	}

	stats := Stats{
		ByStatus:            make(map[Status]int64, 5),
		ByType:              make(map[TaskType]int64, 4),
		ByTypeStatus:        make(map[TaskType]map[Status]int64, 4),
		OldestPendingByType: make(map[TaskType]time.Duration, 4),
	}
	for _, status := range []Status{StatusPending, StatusLeased, StatusDone, StatusDead, StatusCancelled} {
		stats.ByStatus[status] = 0
	}
	for _, taskType := range []TaskType{TaskDispatch, TaskRetry, TaskTimeout, TaskReconcile} {
		stats.ByType[taskType] = 0
	}

	rows, err := s.pool.Query(ctx, "SELECT status, count(*) FROM run_tasks GROUP BY status")
	if err != nil {
		return Stats{}, fmt.Errorf("count tasks by status: %w", err)
	}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return Stats{}, fmt.Errorf("scan status count: %w", err)
		}
		stats.ByStatus[Status(status)] = count
		stats.Total += count
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Stats{}, fmt.Errorf("scan status counts: %w", err)
	}
	rows.Close()

	rows, err = s.pool.Query(ctx, "SELECT task_type, count(*) FROM run_tasks GROUP BY task_type")
	if err != nil {
		return Stats{}, fmt.Errorf("count tasks by type: %w", err)
	}
	for rows.Next() {
		var taskType string
		var count int64
		if err := rows.Scan(&taskType, &count); err != nil {
			rows.Close()
			return Stats{}, fmt.Errorf("scan type count: %w", err)
		}
		stats.ByType[TaskType(taskType)] = count
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Stats{}, fmt.Errorf("scan type counts: %w", err)
	}
	rows.Close()

	rows, err = s.pool.Query(ctx, "SELECT task_type, status, count(*) FROM run_tasks GROUP BY task_type, status")
	if err != nil {
		return Stats{}, fmt.Errorf("count tasks by type/status: %w", err)
	}
	for rows.Next() {
		var taskType, status string
		var count int64
		if err := rows.Scan(&taskType, &status, &count); err != nil {
			rows.Close()
			return Stats{}, fmt.Errorf("scan type/status count: %w", err)
		}
		if stats.ByTypeStatus[TaskType(taskType)] == nil {
			stats.ByTypeStatus[TaskType(taskType)] = make(map[Status]int64, 5)
		}
		stats.ByTypeStatus[TaskType(taskType)][Status(status)] = count
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Stats{}, fmt.Errorf("scan type/status counts: %w", err)
	}
	rows.Close()

	rows, err = s.pool.Query(ctx, `
		SELECT task_type, min(available_at)
		FROM run_tasks
		WHERE status = 'PENDING'
		GROUP BY task_type
	`)
	if err != nil {
		return Stats{}, fmt.Errorf("oldest pending by type: %w", err)
	}
	now := time.Now()
	for rows.Next() {
		var taskType string
		var oldest time.Time
		if err := rows.Scan(&taskType, &oldest); err != nil {
			rows.Close()
			return Stats{}, fmt.Errorf("scan oldest pending by type: %w", err)
		}
		if !oldest.IsZero() {
			stats.OldestPendingByType[TaskType(taskType)] = now.Sub(oldest)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Stats{}, fmt.Errorf("scan oldest pending by type: %w", err)
	}
	rows.Close()

	var oldest *time.Time
	if err := s.pool.QueryRow(
		ctx,
		"SELECT min(available_at) FROM run_tasks WHERE status = 'PENDING'",
	).Scan(&oldest); err != nil {
		return Stats{}, fmt.Errorf("oldest pending task: %w", err)
	}
	if oldest != nil {
		stats.OldestPending = *oldest
		if age := time.Since(*oldest); age > 0 {
			stats.OldestPendingAge = age
		}
	}
	return stats, nil
}
