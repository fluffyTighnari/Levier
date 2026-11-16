// Middleware — internal/db
//
// RunTaskQueue schema constraint tests.
//
// Copyright 2026 The Middleware Authors.
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/fluffyTighnari/levier/internal/id"
)

func TestRunTaskConstraints(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	for _, runID := range []string{"run-1", "run-2", "run-3", "run-4"} {
		if _, err := pool.Exec(ctx,
			"INSERT INTO runs (run_id, app_id, status) VALUES ($1, 'app-1', 'queued')",
			runID,
		); err != nil {
			t.Fatalf("insert run %s: %v", runID, err)
		}
	}

	taskID := id.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO run_tasks (task_id, run_id, task_type, status, available_at)
		VALUES ($1, 'run-1', 'RUN_DISPATCH', 'PENDING', now())
	`, taskID); err != nil {
		t.Fatalf("insert pending task: %v", err)
	}

	// Active uniqueness: same run+type cannot have two PENDING/LEASED rows.
	_, err := pool.Exec(ctx, `
		INSERT INTO run_tasks (task_id, run_id, task_type, status, available_at)
		VALUES ($1, 'run-1', 'RUN_DISPATCH', 'PENDING', now())
	`, id.New())
	assertPgCode(t, err, "23505")

	// attempt must not exceed max_attempts.
	_, err = pool.Exec(ctx, `
		INSERT INTO run_tasks (task_id, run_id, task_type, status, available_at, attempt, max_attempts)
		VALUES ($1, 'run-2', 'RUN_TIMEOUT', 'PENDING', now(), 6, 5)
	`, id.New())
	assertPgCode(t, err, "23514")

	// LEASED requires owner, token, and expiry.
	_, err = pool.Exec(ctx, `
		INSERT INTO run_tasks (task_id, run_id, task_type, status, available_at, attempt, lease_token, lease_expires_at)
		VALUES ($1, 'run-3', 'RUN_RECONCILE', 'LEASED', now(), 1, 1, now() + interval '30 seconds')
	`, id.New())
	assertPgCode(t, err, "23514")

	leasedID := id.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO run_tasks (
			task_id, run_id, task_type, status, available_at, attempt,
			lease_owner, lease_token, lease_expires_at
		) VALUES ($1, 'run-4', 'RUN_RETRY', 'LEASED', now(), 1, 'worker-1', 1, now() + interval '30 seconds')
	`, leasedID); err != nil {
		t.Fatalf("insert leased task: %v", err)
	}

	// Non-LEASED rows must clear lease owner/expiry.
	_, err = pool.Exec(ctx, "UPDATE run_tasks SET status = 'DONE' WHERE task_id = $1", leasedID)
	assertPgCode(t, err, "23514")
}

func assertPgCode(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected PostgreSQL error %s, got nil", code)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != code {
		t.Fatalf("PostgreSQL code = %s, want %s (%v)", pgErr.Code, code, err)
	}
}
