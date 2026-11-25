// Levier — internal/workflow
//
// PostgreSQL cancellation and race tests.
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
)

func TestCancelRunPending(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-cancel-pending")
	store := NewPostgres(pool)

	first, err := store.Enqueue(ctx, NewTask("run-cancel-pending", TaskDispatch, time.Now().UTC(), 0))
	if err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	count, err := store.CancelRun(ctx, "run-cancel-pending", "user")
	if err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CancelRun() count = %d, want 1", count)
	}
	assertTaskStatus(t, pool, first.ID, StatusCancelled, 0, "user")

	// CANCELLED is terminal, so a fresh active task is allowed.
	second, err := store.Enqueue(ctx, NewTask("run-cancel-pending", TaskDispatch, time.Now().UTC(), 0))
	if err != nil {
		t.Fatalf("Enqueue after cancel error = %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("enqueue after cancel returned cancelled task")
	}
	if count := countTasks(t, pool, "run-cancel-pending"); count != 2 {
		t.Fatalf("task count = %d, want 2", count)
	}
}

func TestCancelRunLeasedInvalidatesLease(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-cancel-leased")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-cancel-leased", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	task, lease, err := store.Claim(ctx, "worker-1", 30*time.Second)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	count, err := store.CancelRun(ctx, "run-cancel-leased", "user")
	if err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CancelRun() count = %d, want 1", count)
	}
	assertTaskStatus(t, pool, task.ID, StatusCancelled, 1, "user")

	if _, err := store.Extend(ctx, lease, time.Minute); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("Extend() error = %v, want ErrLeaseLost", err)
	}
	if err := store.Ack(ctx, task, lease); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("Ack() error = %v, want ErrLeaseLost", err)
	}
	if err := store.Nack(ctx, task, lease, time.Now(), errors.New("late")); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("Nack() error = %v, want ErrLeaseLost", err)
	}
	if err := store.Fail(ctx, task, lease, errors.New("late")); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("Fail() error = %v, want ErrLeaseLost", err)
	}
}

func TestCancelTaskScoped(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-cancel-scoped")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-cancel-scoped", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue dispatch error = %v", err)
	}
	if _, err := store.Enqueue(ctx, NewTask("run-cancel-scoped", TaskTimeout, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue timeout error = %v", err)
	}

	count, err := store.CancelTask(ctx, "run-cancel-scoped", TaskDispatch, "user")
	if err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("CancelTask() count = %d, want 1", count)
	}

	var status string
	if err := pool.QueryRow(ctx, `
		SELECT status FROM run_tasks
		WHERE run_id = 'run-cancel-scoped' AND task_type = 'RUN_TIMEOUT'
	`).Scan(&status); err != nil {
		t.Fatalf("query timeout task: %v", err)
	}
	if Status(status) != StatusPending {
		t.Fatalf("timeout status = %s, want PENDING", status)
	}

	if _, err := store.Enqueue(ctx, NewTask("run-cancel-scoped", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue dispatch after cancel error = %v", err)
	}
}

func TestAckBeforeCancel(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-ack-before")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-ack-before", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	task, lease, err := store.Claim(ctx, "worker-1", 30*time.Second)
	if err != nil {
		t.Fatalf("Claim() error = %v", err)
	}
	if err := store.Ack(ctx, task, lease); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	count, err := store.CancelRun(ctx, "run-ack-before", "user")
	if err != nil {
		t.Fatalf("CancelRun() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("CancelRun() count = %d, want 0", count)
	}
	assertTaskStatus(t, pool, task.ID, StatusDone, 1, "")
}

func TestCancelAckRace(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	store := NewPostgres(pool)

	for i := 0; i < 5; i++ {
		runID := fmt.Sprintf("run-race-%d", i)
		insertRunForTest(t, pool, runID)
		if _, err := store.Enqueue(ctx, NewTask(runID, TaskDispatch, time.Now().Add(-time.Second), 0)); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
		task, lease, err := store.Claim(ctx, fmt.Sprintf("worker-%d", i), 30*time.Second)
		if err != nil {
			t.Fatalf("Claim() error = %v", err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		var ackErr error
		var cancelCount int
		var cancelErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			ackErr = store.Ack(ctx, task, lease)
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
		case ackErr == nil:
			if status != string(StatusDone) || cancelCount != 0 {
				t.Fatalf("ack won but status=%s cancelCount=%d", status, cancelCount)
			}
		case errors.Is(ackErr, ErrLeaseLost):
			if status != string(StatusCancelled) || cancelCount != 1 {
				t.Fatalf("cancel won but status=%s cancelCount=%d ackErr=%v", status, cancelCount, ackErr)
			}
		default:
			t.Fatalf("unexpected Ack() error = %v", ackErr)
		}
	}
}

func TestCancelValidation(t *testing.T) {
	store := NewPostgres(nil)
	if _, err := store.CancelRun(context.Background(), "", "user"); err == nil {
		t.Fatal("CancelRun() with empty run_id error = nil")
	}
	if _, err := store.CancelTask(context.Background(), "run-1", TaskType("NOPE"), "user"); err == nil {
		t.Fatal("CancelTask() with invalid type error = nil")
	}
}
