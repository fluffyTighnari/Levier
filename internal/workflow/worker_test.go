// Middleware — internal/workflow
//
// Worker and pool tests.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestWorkerProcessOneAck(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-worker-ack")
	if _, err := NewPostgres(pool).Enqueue(ctx, NewTask("run-worker-ack", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	called := false
	worker := NewWorker(NewPostgres(pool), HandlerFunc(func(ctx context.Context, task Task, lease Lease) error {
		called = true
		return nil
	}), DefaultWorkerConfig("worker-ack"))

	processed, err := worker.ProcessOne(ctx)
	if err != nil {
		t.Fatalf("ProcessOne() error = %v", err)
	}
	if !processed || !called {
		t.Fatalf("processed=%v called=%v", processed, called)
	}
	var status string
	if err := pool.QueryRow(ctx, "SELECT status FROM run_tasks WHERE run_id = 'run-worker-ack'").Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if Status(status) != StatusDone {
		t.Fatalf("status = %s, want DONE", status)
	}
}

func TestWorkerProcessOneNoTask(t *testing.T) {
	pool := testStorePool(t)
	worker := NewWorker(NewPostgres(pool), HandlerFunc(func(context.Context, Task, Lease) error {
		t.Fatal("handler should not be called")
		return nil
	}), DefaultWorkerConfig("worker-idle"))

	processed, err := worker.ProcessOne(context.Background())
	if err != nil {
		t.Fatalf("ProcessOne() error = %v", err)
	}
	if processed {
		t.Fatal("processed = true, want false")
	}
}

func TestWorkerRetryAndPermanent(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-worker-retry")
	insertRunForTest(t, pool, "run-worker-dead")
	store := NewPostgres(pool)

	if _, err := store.Enqueue(ctx, NewTask("run-worker-retry", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue retry: %v", err)
	}
	if _, err := store.Enqueue(ctx, NewTask("run-worker-dead", TaskTimeout, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue dead: %v", err)
	}

	retryWorker := NewWorker(store, HandlerFunc(func(context.Context, Task, Lease) error {
		return errors.New("temporary")
	}), DefaultWorkerConfig("worker-retry"))
	if _, err := retryWorker.ProcessOne(ctx); err != nil {
		t.Fatalf("retry ProcessOne() error = %v", err)
	}
	assertTaskStatus(t, pool, taskIDByRun(t, pool, "run-worker-retry"), StatusPending, 1, "temporary")

	deadCalled := make(chan Task, 1)
	deadWorker := NewWorker(store, HandlerFunc(func(context.Context, Task, Lease) error {
		return Permanent(errors.New("bad input"))
	}), DefaultWorkerConfig("worker-dead"))
	deadWorker.SetDeadLetterHandler(DeadLetterFunc(func(ctx context.Context, task Task, cause error) error {
		deadCalled <- task
		return nil
	}))
	if _, err := deadWorker.ProcessOne(ctx); err != nil {
		t.Fatalf("dead ProcessOne() error = %v", err)
	}
	deadTask := taskIDByRun(t, pool, "run-worker-dead")
	assertTaskStatus(t, pool, deadTask, StatusDead, 1, "bad input (permanent)")
	select {
	case task := <-deadCalled:
		if task.ID != deadTask {
			t.Fatalf("dead callback task = %s, want %s", task.ID, deadTask)
		}
	default:
		t.Fatal("dead callback not called")
	}
}

func TestWorkerPanicIsRecovered(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-worker-panic")
	store := NewPostgres(pool)
	if _, err := store.Enqueue(ctx, NewTask("run-worker-panic", TaskReconcile, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	worker := NewWorker(store, HandlerFunc(func(context.Context, Task, Lease) error {
		panic("boom")
	}), DefaultWorkerConfig("worker-panic"))
	if _, err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("ProcessOne() error = %v", err)
	}
	taskID := taskIDByRun(t, pool, "run-worker-panic")
	var status, lastError string
	if err := pool.QueryRow(ctx, "SELECT status, last_error FROM run_tasks WHERE task_id = $1", taskID).Scan(&status, &lastError); err != nil {
		t.Fatalf("query panic task: %v", err)
	}
	if Status(status) != StatusPending {
		t.Fatalf("status = %s, want PENDING", status)
	}
	if !strings.Contains(lastError, "handler panic: boom") {
		t.Fatalf("last_error = %q", lastError)
	}
}

func TestWorkerHeartbeatKeepsLease(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-worker-heartbeat")
	store := NewPostgres(pool)
	if _, err := store.Enqueue(ctx, NewTask("run-worker-heartbeat", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	started := make(chan struct{})
	handler := HandlerFunc(func(ctx context.Context, task Task, lease Lease) error {
		close(started)
		time.Sleep(450 * time.Millisecond)
		return nil
	})
	cfg := DefaultWorkerConfig("worker-heartbeat")
	cfg.LeaseTTL = 200 * time.Millisecond
	cfg.HeartbeatInterval = 25 * time.Millisecond
	worker := NewWorker(store, handler, cfg)

	done := make(chan error, 1)
	go func() {
		_, err := worker.ProcessOne(ctx)
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	time.Sleep(300 * time.Millisecond)
	reaped, err := store.ReapExpired(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("ReapExpired() error = %v", err)
	}
	if reaped != 0 {
		t.Fatalf("ReapExpired() = %d, want 0 (heartbeat should renew)", reaped)
	}
	if err := <-done; err != nil {
		t.Fatalf("ProcessOne() error = %v", err)
	}
	assertTaskStatus(t, pool, taskIDByRun(t, pool, "run-worker-heartbeat"), StatusDone, 1, "")
}

func TestWorkerStopGraceful(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	insertRunForTest(t, pool, "run-worker-stop")
	store := NewPostgres(pool)
	if _, err := store.Enqueue(ctx, NewTask("run-worker-stop", TaskDispatch, time.Now().UTC(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	worker := NewWorker(store, HandlerFunc(func(context.Context, Task, Lease) error {
		close(started)
		<-release
		return nil
	}), DefaultWorkerConfig("worker-stop"))
	worker.Start()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	stopped := make(chan struct{})
	go func() {
		worker.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop() returned while handler was still running")
	default:
	}

	close(release)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return")
	}
	assertTaskStatus(t, pool, taskIDByRun(t, pool, "run-worker-stop"), StatusDone, 1, "")
}

func TestPoolRunsConcurrently(t *testing.T) {
	pool := testStorePool(t)
	ctx := context.Background()
	store := NewPostgres(pool)
	const n = 8
	for i := 0; i < n; i++ {
		runID := fmt.Sprintf("run-pool-%d", i)
		insertRunForTest(t, pool, runID)
		if _, err := store.Enqueue(ctx, NewTask(runID, TaskDispatch, time.Now().UTC(), 0)); err != nil {
			t.Fatalf("Enqueue(%s) error = %v", runID, err)
		}
	}

	var handled atomic.Int32
	workerPool := NewPool(store, HandlerFunc(func(context.Context, Task, Lease) error {
		time.Sleep(10 * time.Millisecond)
		handled.Add(1)
		return nil
	}), 4, DefaultWorkerConfig("pool-test"))
	workerPool.Start()
	defer workerPool.Stop()

	deadline := time.After(3 * time.Second)
	for handled.Load() < n {
		select {
		case <-deadline:
			t.Fatalf("handled = %d, want %d", handled.Load(), n)
		case <-time.After(10 * time.Millisecond):
		}
	}

	var done int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM run_tasks WHERE status = 'DONE'").Scan(&done); err != nil {
		t.Fatalf("count done: %v", err)
	}
	if done != n {
		t.Fatalf("done = %d, want %d", done, n)
	}
}

func taskIDByRun(t *testing.T, pool *pgxpool.Pool, runID string) string {
	t.Helper()
	var taskID string
	if err := pool.QueryRow(context.Background(), "SELECT task_id FROM run_tasks WHERE run_id = $1", runID).Scan(&taskID); err != nil {
		t.Fatalf("task ID by run %s: %v", runID, err)
	}
	return taskID
}
