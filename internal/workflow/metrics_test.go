// Middleware — internal/workflow
//
// In-process metrics tests.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"testing"
	"time"
)

func TestMetricsSnapshot(t *testing.T) {
	metrics := NewMetrics()
	metrics.recordClaim(10 * time.Millisecond)
	metrics.recordHandler(20 * time.Millisecond)
	metrics.Acks.Add(1)
	metrics.Nacks.Add(2)
	metrics.Fails.Add(3)
	metrics.Dead.Add(4)
	metrics.LeaseLost.Add(5)
	metrics.Panics.Add(6)

	snapshot := metrics.Snapshot()
	if snapshot.Claims != 1 || snapshot.Handled != 1 {
		t.Fatalf("counters = %+v", snapshot)
	}
	if snapshot.Acks != 1 || snapshot.Nacks != 2 || snapshot.Fails != 3 {
		t.Fatalf("outcome counters = %+v", snapshot)
	}
	if snapshot.Dead != 4 || snapshot.LeaseLost != 5 || snapshot.Panics != 6 {
		t.Fatalf("fault counters = %+v", snapshot)
	}
	if snapshot.ClaimAverage != 10*time.Millisecond {
		t.Fatalf("ClaimAverage = %v", snapshot.ClaimAverage)
	}
	if snapshot.HandlerAverage != 20*time.Millisecond {
		t.Fatalf("HandlerAverage = %v", snapshot.HandlerAverage)
	}
}

func TestNilMetricsSnapshot(t *testing.T) {
	var metrics *Metrics
	if snapshot := metrics.Snapshot(); snapshot != (MetricsSnapshot{}) {
		t.Fatalf("nil snapshot = %+v", snapshot)
	}
	metrics.recordClaim(time.Second)
	metrics.recordHandler(time.Second)
}
