// Levier — internal/state/workflow
//
// Fault and recovery tests for the PostgreSQL task queue.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDelayedTaskNotClaimedEarly(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-delayed")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-delayed", TaskDispatch, time.Now().Add(time.Hour), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if _, _, err := store.Claim(ctx, "worker-1", 30*time.Second); !errors.Is(err, ErrNoTask) {
		t.Fatalf("early Claim() error = %v, want ErrNoTask", err)
	}

	if _, err := pool.Exec(ctx, "UPDATE run_tasks SET available_at = now() - interval '1 second' WHERE run_id = 'run-delayed'"); err != nil {
		t.Fatalf("move available_at: %v", err)
	}
	if _, _, err := store.Claim(ctx, "worker-1", 30*time.Second); err != nil {
		t.Fatalf("delayed Claim() error = %v", err)
	}
}

func TestCrashRecoveryAfterLeaseExpiry(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-crash")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-crash", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	oldTask, oldLease, err := store.Claim(ctx, "crashed-worker", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE run_tasks SET lease_expires_at = now() - interval '1 second' WHERE task_id = $1", oldTask.ID); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	reaped, err := store.ReapExpired(ctx, time.Now().UTC(), 10)
	if err != nil || reaped != 1 {
		t.Fatalf("ReapExpired() = (%d,%v), want 1", reaped, err)
	}
	if err := store.Ack(ctx, oldTask, oldLease); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("old Ack() error = %v, want ErrLeaseLost", err)
	}

	newTask, newLease, err := store.Claim(ctx, "recovery-worker", 30*time.Second)
	if err != nil {
		t.Fatalf("recovery Claim() error = %v", err)
	}
	if newTask.Attempt != 2 || newLease.Token != oldLease.Token+2 {
		t.Fatalf("recovery task attempt=%d token=%d, want attempt=2 token=%d", newTask.Attempt, newLease.Token, oldLease.Token+2)
	}
	if err := store.Ack(ctx, newTask, newLease); err != nil {
		t.Fatalf("recovery Ack() error = %v", err)
	}
	assertTaskStatus(t, pool, oldTask.ID, StatusDone, 2, "")
}

func TestPoolProcessesEachTaskOnce(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	store := NewPostgres(pool)
	const n = 20
	for i := 0; i < n; i++ {
		runID := fmt.Sprintf("run-once-%d", i)
		insertRunForTest(t, pool, runID)
		if _, err := store.Enqueue(ctx, NewTask(runID, TaskDispatch, time.Now().UTC(), 0)); err != nil {
			t.Fatalf("Enqueue(%s) error = %v", runID, err)
		}
	}

	var mu sync.Mutex
	seen := make(map[string]int, n)
	handler := HandlerFunc(func(ctx context.Context, task Task, lease Lease) error {
		mu.Lock()
		seen[task.ID]++
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		return nil
	})
	workerPool := NewPool(store, handler, 4, DefaultWorkerConfig("once"))
	workerPool.Start()
	defer workerPool.Stop()

	deadline := time.After(4 * time.Second)
	for {
		mu.Lock()
		count := len(seen)
		mu.Unlock()
		if count == n {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("processed %d tasks, want %d", count, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
	workerPool.Stop()

	mu.Lock()
	defer mu.Unlock()
	for taskID, count := range seen {
		if count != 1 {
			t.Fatalf("task %s handled %d times", taskID, count)
		}
	}
	if len(seen) != n {
		t.Fatalf("seen %d tasks, want %d", len(seen), n)
	}
}

func TestCancelledTaskIsNotExecuted(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-cancel-before-exec")
	store := NewPostgres(pool)
	if _, err := store.Enqueue(ctx, NewTask("run-cancel-before-exec", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if _, err := store.CancelRun(ctx, "run-cancel-before-exec", "user"); err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}

	called := false
	worker := NewWorker(store, HandlerFunc(func(context.Context, Task, Lease) error {
		called = true
		return nil
	}), DefaultWorkerConfig("cancel-worker"))
	processed, err := worker.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne() error = %v", err)
	}
	if processed || called {
		t.Fatalf("processed=%v called=%v, want both false", processed, called)
	}
}

func TestCancelNackRace(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	store := NewPostgres(pool)

	for i := 0; i < 5; i++ {
		runID := fmt.Sprintf("run-cancel-nack-%d", i)
		insertRunForTest(t, pool, runID)
		if _, err := store.Enqueue(ctx, NewTask(runID, TaskDispatch, time.Now().UTC(), 0)); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
		task, lease, err := store.Claim(ctx, fmt.Sprintf("worker-%d", i), 30*time.Second)
		if err != nil {
			t.Fatalf("Claim() error = %v", err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		var nackErr error
		var cancelCount int
		var cancelErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			nackErr = store.Nack(ctx, task, lease, time.Now(), errors.New("retry"))
		}()
		go func() {
			defer wg.Done()
			<-start
			cancelCount, cancelErr = store.CancelRun(ctx, runID, "race")
		}()
		close(start)
		wg.Wait()
		if cancelErr != nil {
			t.Fatalf("CancelRun() error = %v", cancelErr)
		}

		var status string
		if err := pool.QueryRow(ctx, "SELECT status FROM run_tasks WHERE task_id = $1", task.ID).Scan(&status); err != nil {
			t.Fatalf("query status: %v", err)
		}
		switch {
		case nackErr == nil && status == string(StatusPending) && cancelCount == 0:
			// Nack happened first and Cancel did not observe an active task.
		case nackErr == nil && status == string(StatusCancelled) && cancelCount == 1:
			// Nack returned to PENDING, then Cancel legitimately cancelled it.
		case errors.Is(nackErr, ErrLeaseLost) && status == string(StatusCancelled) && cancelCount == 1:
			// Cancel won the lease race; Nack observed the lost token.
		default:
			t.Fatalf("unexpected race outcome: nackErr=%v status=%s cancelCount=%d", nackErr, status, cancelCount)
		}
	}
}

func TestConnectionPoolExhaustion(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	store := NewPostgres(pool)

	maxConns := int(pool.Stat().MaxConns())
	held := make([]*pgxpool.Conn, 0, maxConns)
	for i := 0; i < maxConns; i++ {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("Acquire() error = %v", err)
		}
		held = append(held, conn)
	}
	defer func() {
		for _, conn := range held {
			conn.Release()
		}
	}()

	claimCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, _, err := store.Claim(claimCtx, "exhausted-worker", 30*time.Second); err == nil {
		t.Fatal("Claim() with exhausted pool error = nil")
	} else if errors.Is(err, ErrNoTask) {
		t.Fatal("exhausted pool returned ErrNoTask")
	}
}
