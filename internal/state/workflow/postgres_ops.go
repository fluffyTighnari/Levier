// Levier — internal/state/workflow
//
// Operator-facing task queries.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"context"
	"fmt"
	"strings"
)

const (
	defaultTaskListLimit = 100
	maxTaskListLimit     = 500
)

// TaskFilter selects tasks for the admin operations view.
type TaskFilter struct {
	Statuses []Status
	Types    []TaskType
	RunID    string
	Limit    int
	Offset   int
}

// ListTasks returns task metadata without exposing payloads.
func (s *PostgresStore) ListTasks(ctx context.Context, filter TaskFilter) ([]Task, error) {
	if s == nil || s.pool == nil {
		return nil, fmt.Errorf("workflow: postgres pool is nil")
	}
	for _, status := range filter.Statuses {
		if !status.Valid() {
			return nil, fmt.Errorf("%w: invalid task status %q", ErrInvalidTask, status)
		}
	}
	for _, taskType := range filter.Types {
		if !taskType.Valid() {
			return nil, fmt.Errorf("%w: invalid task type %q", ErrInvalidTask, taskType)
		}
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultTaskListLimit
	}
	if limit > maxTaskListLimit {
		limit = maxTaskListLimit
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	where := []string{"TRUE"}
	args := make([]any, 0, 5)
	if len(filter.Statuses) > 0 {
		values := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			values = append(values, string(status))
		}
		args = append(args, values)
		where = append(where, fmt.Sprintf("status = ANY($%d)", len(args)))
	}
	if len(filter.Types) > 0 {
		values := make([]string, 0, len(filter.Types))
		for _, taskType := range filter.Types {
			values = append(values, string(taskType))
		}
		args = append(args, values)
		where = append(where, fmt.Sprintf("task_type = ANY($%d)", len(args)))
	}
	if value := strings.TrimSpace(filter.RunID); value != "" {
		args = append(args, value)
		where = append(where, fmt.Sprintf("run_id = $%d", len(args)))
	}
	args = append(args, limit, offset)

	query := `SELECT ` + taskColumns + `
		FROM run_tasks
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY updated_at DESC, task_id DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]Task, 0, limit)
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("list tasks: scan: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list tasks: rows: %w", err)
	}
	return tasks, nil
}
