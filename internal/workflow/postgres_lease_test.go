// Levier — internal/workflow
//
// PostgreSQL claim/lease/ack/nack tests.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestClaimExtendAck(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-lease")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-lease", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	task, lease, err := store.Claim(ctx, "worker-1", 30*time.Second)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if task.Status != StatusLeased || task.Attempt != 1 {
		t.Fatalf("claimed task status=%s attempt=%d", task.Status, task.Attempt)
	}
	if lease.TaskID != task.ID || lease.Owner != "worker-1" || lease.Token == 0 {
		t.Fatalf("lease = %+v", lease)
	}
	if lease.ExpiresAt.IsZero() {
		t.Fatal("lease expiry is zero")
	}

	if _, _, err := store.Claim(ctx, "worker-2", 30*time.Second); !errors.Is(err, ErrNoTask) {
		t.Fatalf("second Claim() error = %v, want ErrNoTask", err)
	}

	extended, err := store.Extend(ctx, lease, time.Minute)
	if err != nil {
		t.Fatalf("Extend() error = %v", err)
	}
	if extended.Token != lease.Token || !extended.ExpiresAt.After(lease.ExpiresAt) {
		t.Fatalf("extended lease = %+v", extended)
	}

	badLease := extended
	badLease.Token++
	if _, err := store.Extend(ctx, badLease, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale Extend() error = %v, want ErrLeaseLost", err)
	}

	if err := store.Ack(ctx, task, extended); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	assertTaskStatus(t, pool, task.ID, StatusDone, 1, "")
	if err := store.Ack(ctx, task, extended); !errors.Is(err, ErrTaskTerminal) {
		t.Fatalf("second Ack() error = %v, want ErrTaskTerminal", err)
	}
}

func TestNackRetryAndDead(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-nack")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-nack", TaskTimeout, time.Now().UTC(), 2)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	task, lease, err := store.Claim(ctx, "worker-1", 30*time.Second)
	if err != nil {
		t.Fatalf("first Claim() error = %v", err)
	}
	if err := store.Nack(ctx, task, lease, time.Now().Add(-time.Second), errors.New("temporary")); err != nil {
		t.Fatalf("first Nack() error = %v", err)
	}
	assertTaskStatus(t, pool, task.ID, StatusPending, 1, "temporary")

	task, lease, err = store.Claim(ctx, "worker-2", 30*time.Second)
	if err != nil {
		t.Fatalf("second Claim() error = %v", err)
	}
	if task.Attempt != 2 {
		t.Fatalf("second attempt = %d, want 2", task.Attempt)
	}
	if err := store.Nack(ctx, task, lease, time.Now(), errors.New("still failing")); err != nil {
		t.Fatalf("second Nack() error = %v", err)
	}
	assertTaskStatus(t, pool, task.ID, StatusDead, 2, "still failing")
}

func TestFail(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-fail")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-fail", TaskReconcile, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	task, lease, err := store.Claim(ctx, "worker-1", 30*time.Second)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if err := store.Fail(ctx, task, lease, errors.New("permanent")); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	assertTaskStatus(t, pool, task.ID, StatusDead, 1, "permanent")
}

func TestConcurrentClaimsAreUnique(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	store := NewPostgres(pool)

	const n = 8
	for i := 0; i < n; i++ {
		runID := "run-concurrent-" + idSuffix(i)
		insertRunForTest(t, pool, runID)
		if _, err := store.Enqueue(ctx, NewTask(runID, TaskDispatch, time.Now().UTC(), 0)); err != nil {
			t.Fatalf("Enqueue(%s) error = %v", runID, err)
		}
	}

	var wg sync.WaitGroup
	results := make(chan string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			task, _, err := store.Claim(ctx, "worker-"+idSuffix(n), 30*time.Second)
			if err != nil {
				t.Errorf("Claim() error = %v", err)
				return
			}
			results <- task.ID
		}(i)
	}
	wg.Wait()
	close(results)

	seen := make(map[string]struct{}, n)
	for taskID := range results {
		if _, ok := seen[taskID]; ok {
			t.Fatalf("duplicate claimed task %q", taskID)
		}
		seen[taskID] = struct{}{}
	}
	if len(seen) != n {
		t.Fatalf("unique claims = %d, want %d", len(seen), n)
	}
}

func TestClaimValidation(t *testing.T) {
	store := NewPostgres(nil)
	if _, _, err := store.Claim(context.Background(), "", time.Second); err == nil {
		t.Fatal("Claim() with nil pool/empty owner error = nil")
	}
	if err := validateLease(Lease{}); err == nil {
		t.Fatal("validateLease(empty) error = nil")
	}
}

func insertRunForTest(t testing.TB, pool *pgxpool.Pool, runID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), "INSERT INTO runs (run_id, app_id, status) VALUES ($1, 'app-1', 'queued')", runID)
	if err != nil {
		t.Fatalf("insert run %s: %v", runID, err)
	}
}

func assertTaskStatus(t *testing.T, pool *pgxpool.Pool, taskID string, want Status, wantAttempt int, wantError string) {
	t.Helper()
	var status string
	var attempt int
	var lastError string
	var owner string
	var expires *time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT status, attempt, last_error, lease_owner, lease_expires_at
		FROM run_tasks WHERE task_id = $1
	`, taskID).Scan(&status, &attempt, &lastError, &owner, &expires); err != nil {
		t.Fatalf("query task: %v", err)
	}
	if Status(status) != want {
		t.Fatalf("status = %s, want %s", status, want)
	}
	if attempt != wantAttempt {
		t.Fatalf("attempt = %d, want %d", attempt, wantAttempt)
	}
	if lastError != wantError {
		t.Fatalf("last_error = %q, want %q", lastError, wantError)
	}
	if owner != "" || expires != nil {
		t.Fatalf("lease not cleared: owner=%q expires=%v", owner, expires)
	}
}

func idSuffix(i int) string {
	return string(rune('a' + i))
}
