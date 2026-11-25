// Levier — internal/workflow
//
// PostgreSQL expired-lease reaper, DLQ, and stats tests.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestReapExpiredReturnsToPending(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-reap")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-reap", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	task, lease, err := store.Claim(ctx, "worker-1", 30*time.Second)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE run_tasks SET lease_expires_at = now() - interval '1 second'
		WHERE task_id = $1
	`, task.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	count, err := store.ReapExpired(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("ReapExpired() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("ReapExpired() count = %d, want 1", count)
	}
	assertTaskStatus(t, pool, task.ID, StatusPending, 1, "lease_expired")
	if err := store.Ack(ctx, task, lease); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("old Ack() error = %v, want ErrLeaseLost", err)
	}

	if _, _, err := store.Claim(ctx, "worker-2", 30*time.Second); err != nil {
		t.Fatalf("Claim after reap error = %v", err)
	}
}

func TestReapExpiredAtMaxGoesDead(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-reap-dead")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-reap-dead", TaskTimeout, time.Now().UTC(), 1)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	task, _, err := store.Claim(ctx, "worker-1", 30*time.Second)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE run_tasks SET lease_expires_at = now() - interval '1 second'
		WHERE task_id = $1
	`, task.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	if count, err := store.ReapExpired(ctx, time.Now().UTC(), 10); err != nil || count != 1 {
		t.Fatalf("ReapExpired() = (%d,%v)", count, err)
	}
	assertTaskStatus(t, pool, task.ID, StatusDead, 1, "lease_expired")
}

func TestReapExpiredLimit(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	store := NewPostgres(pool)

	for i := 0; i < 3; i++ {
		runID := fmt.Sprintf("run-reap-limit-%d", i)
		insertRunForTest(t, pool, runID)
		if _, err := store.Enqueue(ctx, NewTask(runID, TaskDispatch, time.Now().Add(-time.Second), 0)); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
		if _, _, err := store.Claim(ctx, fmt.Sprintf("worker-%d", i), 30*time.Second); err != nil {
			t.Fatalf("Claim() error = %v", err)
		}
	}
	if _, err := pool.Exec(ctx, "UPDATE run_tasks SET lease_expires_at = now() - interval '1 second'"); err != nil {
		t.Fatalf("expire leases: %v", err)
	}

	first, err := store.ReapExpired(ctx, time.Now().UTC(), 2)
	if err != nil || first != 2 {
		t.Fatalf("first ReapExpired() = (%d,%v), want 2", first, err)
	}
	second, err := store.ReapExpired(ctx, time.Now().UTC(), 2)
	if err != nil || second != 1 {
		t.Fatalf("second ReapExpired() = (%d,%v), want 1", second, err)
	}
}

func TestStatsAndListDead(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	store := NewPostgres(pool)

	insertRunForTest(t, pool, "run-stats-dispatch")
	insertRunForTest(t, pool, "run-stats-retry")
	insertRunForTest(t, pool, "run-stats-timeout")
	if _, err := store.Enqueue(ctx, NewTask("run-stats-dispatch", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue dispatch: %v", err)
	}
	if _, err := store.Enqueue(ctx, NewTask("run-stats-retry", TaskRetry, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue retry: %v", err)
	}
	if _, err := store.Enqueue(ctx, NewTask("run-stats-timeout", TaskTimeout, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue timeout: %v", err)
	}
	task, lease, err := store.Claim(ctx, "worker-1", 30*time.Second)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if err := store.Fail(ctx, task, lease, errors.New("dead")); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.Total != 3 {
		t.Fatalf("Total = %d, want 3", stats.Total)
	}
	if stats.ByStatus[StatusPending] != 2 || stats.ByStatus[StatusDead] != 1 {
		t.Fatalf("ByStatus = %+v", stats.ByStatus)
	}
	if stats.ByType[TaskDispatch] != 1 || stats.ByType[TaskRetry] != 1 || stats.ByType[TaskTimeout] != 1 {
		t.Fatalf("ByType = %+v", stats.ByType)
	}
	if stats.OldestPending.IsZero() {
		t.Fatal("OldestPending is zero")
	}

	dead, err := store.ListDead(ctx, 10)
	if err != nil {
		t.Fatalf("ListDead() error = %v", err)
	}
	if len(dead) != 1 || dead[0].ID != task.ID {
		t.Fatalf("ListDead() = %+v", dead)
	}
}
