// Middleware — internal/workflow
//
// PostgreSQL queue throughput benchmarks.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func BenchmarkEnqueue(b *testing.B) {
	pool := testStorePool(b)
	ctx := context.Background()
	store := NewPostgres(pool)

	for i := 0; i < b.N; i++ {
		insertRunForTest(b, pool, fmt.Sprintf("bench-enqueue-%d", i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := store.Enqueue(ctx, NewTask(
			fmt.Sprintf("bench-enqueue-%d", i),
			TaskDispatch,
			time.Now().UTC(),
			0,
		))
		if err != nil {
			b.Fatalf("Enqueue() error = %v", err)
		}
	}
}

func BenchmarkClaimAck(b *testing.B) {
	pool := testStorePool(b)
	ctx := context.Background()
	store := NewPostgres(pool)

	for i := 0; i < b.N; i++ {
		runID := fmt.Sprintf("bench-claim-%d", i)
		insertRunForTest(b, pool, runID)
		if _, err := store.Enqueue(ctx, NewTask(runID, TaskDispatch, time.Now().UTC(), 0)); err != nil {
			b.Fatalf("Enqueue() error = %v", err)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task, lease, err := store.Claim(ctx, "bench-worker", 30*time.Second)
		if err != nil {
			b.Fatalf("Claim() error = %v", err)
		}
		if err := store.Ack(ctx, task, lease); err != nil {
			b.Fatalf("Ack() error = %v", err)
		}
	}
}
