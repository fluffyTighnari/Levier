// Middleware — internal/workflow
//
// PostgreSQL enqueue and idempotency tests.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/levier/internal/db"
	"github.com/fluffyTighnari/levier/internal/id"
)

func testStorePool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("MIDDLEWARE_TEST_POSTGRES") != "1" {
		t.Skip("MIDDLEWARE_TEST_POSTGRES is not set to 1")
	}
	dsn := os.Getenv("MIDDLEWARE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIDDLEWARE_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	schema := strings.ToLower("mw_test_" + id.New())
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("NewWithConfig() error = %v", err)
	}
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		pool.Close()
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE")
		pool.Close()
	})
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return pool
}

func TestEnqueueIdempotent(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "INSERT INTO runs (run_id, app_id, status) VALUES ('run-1', 'app-1', 'queued')"); err != nil {
		t.Fatalf("insert run: %v", err)
	}

	store := NewPostgres(pool)
	first := NewTask("run-1", TaskDispatch, time.Now().UTC(), 0)
	first.Payload = map[string]any{"reason": "initial"}

	gotFirst, err := store.Enqueue(ctx, first)
	if err != nil {
		t.Fatalf("first Enqueue() error = %v", err)
	}
	if gotFirst.ID != first.ID {
		t.Fatalf("first ID = %q, want %q", gotFirst.ID, first.ID)
	}
	if gotFirst.Payload["reason"] != "initial" {
		t.Fatalf("payload = %+v", gotFirst.Payload)
	}

	second := NewTask("run-1", TaskDispatch, time.Now().UTC(), 0)
	gotSecond, err := store.Enqueue(ctx, second)
	if err != nil {
		t.Fatalf("second Enqueue() error = %v", err)
	}
	if gotSecond.ID != first.ID {
		t.Fatalf("second ID = %q, want existing %q", gotSecond.ID, first.ID)
	}
	if count := countTasks(t, pool, "run-1"); count != 1 {
		t.Fatalf("task count = %d, want 1", count)
	}

	// A terminal task no longer blocks a new active task.
	if _, err := pool.Exec(ctx, `
		UPDATE run_tasks
		SET status = 'DONE', lease_owner = '', lease_expires_at = NULL
		WHERE task_id = $1
	`, first.ID); err != nil {
		t.Fatalf("mark first done: %v", err)
	}
	third := NewTask("run-1", TaskDispatch, time.Now().UTC(), 0)
	gotThird, err := store.Enqueue(ctx, third)
	if err != nil {
		t.Fatalf("third Enqueue() error = %v", err)
	}
	if gotThird.ID == first.ID {
		t.Fatal("third enqueue returned terminal task")
	}
	if count := countTasks(t, pool, "run-1"); count != 2 {
		t.Fatalf("task count = %d, want 2", count)
	}
}

func TestEnqueueWithRunTx(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	store := NewPostgres(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, "INSERT INTO runs (run_id, app_id, status) VALUES ('run-tx', 'app-1', 'queued')"); err != nil {
		t.Fatalf("insert run in tx: %v", err)
	}
	task, err := store.EnqueueTx(ctx, tx, NewTask("run-tx", TaskTimeout, time.Now().UTC(), 0))
	if err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}
	if task.RunID != "run-tx" {
		t.Fatalf("RunID = %q", task.RunID)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if count := countTasks(t, pool, "run-tx"); count != 1 {
		t.Fatalf("task count = %d, want 1", count)
	}
}

func TestEnqueueTxRollback(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	store := NewPostgres(pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin() error = %v", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO runs (run_id, app_id, status) VALUES ('run-rollback', 'app-1', 'queued')"); err != nil {
		t.Fatalf("insert run in tx: %v", err)
	}
	if _, err := store.EnqueueTx(ctx, tx, NewTask("run-rollback", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("EnqueueTx() error = %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	var runs, tasks int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM runs WHERE run_id = 'run-rollback'").Scan(&runs); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM run_tasks WHERE run_id = 'run-rollback'").Scan(&tasks); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if runs != 0 || tasks != 0 {
		t.Fatalf("rollback left runs=%d tasks=%d", runs, tasks)
	}
}

func TestNormalizeForEnqueue(t *testing.T) {
	valid := NewTask("run-1", TaskRetry, time.Now().UTC(), 0)
	normalized, err := normalizeForEnqueue(valid)
	if err != nil {
		t.Fatalf("normalizeForEnqueue() error = %v", err)
	}
	if normalized.ID == "" || normalized.Status != StatusPending {
		t.Fatalf("normalized = %+v", normalized)
	}

	tests := []struct {
		name   string
		mutate func(*Task)
		want   string
	}{
		{"missing run", func(task *Task) { task.RunID = "" }, "run_id"},
		{"not pending", func(task *Task) { task.Status = StatusDone }, "PENDING"},
		{"attempt set", func(task *Task) { task.Attempt = 1 }, "attempt must be 0"},
		{"lease set", func(task *Task) { task.LeaseOwner = "w" }, "must not carry a lease"},
		{"bad type", func(task *Task) { task.Type = "NOPE" }, "task.type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := NewTask("run-1", TaskRetry, time.Now().UTC(), 0)
			tt.mutate(&task)
			if _, err := normalizeForEnqueue(task); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("normalizeForEnqueue() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func countTasks(t *testing.T, pool *pgxpool.Pool, runID string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM run_tasks WHERE run_id = $1", runID).Scan(&count); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	return count
}
