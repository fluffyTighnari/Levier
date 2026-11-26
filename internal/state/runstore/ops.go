// Levier — internal/state/runstore
//
// Operator-facing run queries.
//
// Copyright 2026 The Levier Authors.
package runstore

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	defaultRunListLimit = 50
	maxRunListLimit     = 200
)

var validRunStatuses = map[string]struct{}{
	StatusQueued:    {},
	StatusAssigned:  {},
	StatusRunning:   {},
	StatusWaiting:   {},
	StatusCompleted: {},
	StatusFailed:    {},
	StatusCancelled: {},
	StatusTimeout:   {},
}

// ValidRunStatus reports whether status is a persisted run state.
func ValidRunStatus(status string) bool {
	_, ok := validRunStatuses[status]
	return ok
}

// RunFilter selects runs for the admin operations view.
type RunFilter struct {
	Statuses  []string
	TenantID  string
	AppID     string
	DaemonID  string
	SessionID string
	Limit     int
	Cursor    string
}

// RunPage is one stable page ordered by created_at descending.
type RunPage struct {
	Runs       []Run  `json:"runs"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type runCursor struct {
	CreatedAt time.Time `json:"created_at"`
	RunID     string    `json:"run_id"`
}

// ListRuns returns a filtered, cursor-paginated run list.
func (s *Store) ListRuns(ctx context.Context, filter RunFilter) (RunPage, error) {
	if s == nil || s.pool == nil {
		return RunPage{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	for _, status := range filter.Statuses {
		if _, ok := validRunStatuses[status]; !ok {
			return RunPage{}, fmt.Errorf("%w: invalid run status %q", ErrInvalidRun, status)
		}
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultRunListLimit
	}
	if limit > maxRunListLimit {
		limit = maxRunListLimit
	}

	where := []string{"TRUE"}
	args := make([]any, 0, 8)
	if len(filter.Statuses) > 0 {
		args = append(args, filter.Statuses)
		where = append(where, fmt.Sprintf("status = ANY($%d)", len(args)))
	}
	if value := strings.TrimSpace(filter.TenantID); value != "" {
		args = append(args, value)
		where = append(where, fmt.Sprintf("tenant_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.AppID); value != "" {
		args = append(args, value)
		where = append(where, fmt.Sprintf("app_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.DaemonID); value != "" {
		args = append(args, value)
		where = append(where, fmt.Sprintf("assigned_daemon_id = $%d", len(args)))
	}
	if value := strings.TrimSpace(filter.SessionID); value != "" {
		args = append(args, value)
		where = append(where, fmt.Sprintf("session_id = $%d", len(args)))
	}
	if filter.Cursor != "" {
		cursor, err := decodeRunCursor(filter.Cursor)
		if err != nil {
			return RunPage{}, err
		}
		args = append(args, cursor.CreatedAt, cursor.RunID)
		where = append(where,
			fmt.Sprintf("(created_at < $%d OR (created_at = $%d AND run_id < $%d))",
				len(args)-1, len(args)-1, len(args)),
		)
	}
	args = append(args, limit+1)

	query := `SELECT ` + runColumns + `
		FROM runs
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY created_at DESC, run_id DESC
		LIMIT $` + fmt.Sprint(len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return RunPage{}, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()

	runs := make([]Run, 0, limit)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return RunPage{}, fmt.Errorf("list runs: scan: %w", err)
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return RunPage{}, fmt.Errorf("list runs: rows: %w", err)
	}

	page := RunPage{Runs: runs}
	if len(runs) > limit {
		page.Runs = runs[:limit]
		page.NextCursor = encodeRunCursor(page.Runs[len(page.Runs)-1])
	}
	return page, nil
}

func encodeRunCursor(run Run) string {
	value, _ := json.Marshal(runCursor{CreatedAt: run.CreatedAt, RunID: run.RunID})
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeRunCursor(value string) (runCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return runCursor{}, fmt.Errorf("%w: invalid cursor", ErrInvalidRun)
	}
	var cursor runCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.RunID == "" || cursor.CreatedAt.IsZero() {
		return runCursor{}, fmt.Errorf("%w: invalid cursor", ErrInvalidRun)
	}
	return cursor, nil
}
