// Levier — internal/state/workflow
//
// In-process RunTaskQueue metrics.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"sync/atomic"
	"time"
)

// Metrics records worker and queue activity without an external dependency.
type Metrics struct {
	Claims    atomic.Int64
	Acks      atomic.Int64
	Nacks     atomic.Int64
	Fails     atomic.Int64
	Dead      atomic.Int64
	LeaseLost atomic.Int64
	Panics    atomic.Int64

	claimNanos   atomic.Int64
	handlerNanos atomic.Int64
	handled      atomic.Int64
}

// MetricsSnapshot is a point-in-time metrics view.
type MetricsSnapshot struct {
	Claims         int64
	Acks           int64
	Nacks          int64
	Fails          int64
	Dead           int64
	LeaseLost      int64
	Panics         int64
	Handled        int64
	ClaimAverage   time.Duration
	HandlerAverage time.Duration
}

// NewMetrics returns an empty metrics registry.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// Snapshot returns the current counters and average durations.
func (m *Metrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	snapshot := MetricsSnapshot{
		Claims:    m.Claims.Load(),
		Acks:      m.Acks.Load(),
		Nacks:     m.Nacks.Load(),
		Fails:     m.Fails.Load(),
		Dead:      m.Dead.Load(),
		LeaseLost: m.LeaseLost.Load(),
		Panics:    m.Panics.Load(),
		Handled:   m.handled.Load(),
	}
	if claims := m.Claims.Load(); claims > 0 {
		snapshot.ClaimAverage = time.Duration(m.claimNanos.Load() / claims)
	}
	if handled := m.handled.Load(); handled > 0 {
		snapshot.HandlerAverage = time.Duration(m.handlerNanos.Load() / handled)
	}
	return snapshot
}

func (m *Metrics) recordClaim(duration time.Duration) {
	if m == nil {
		return
	}
	m.Claims.Add(1)
	m.claimNanos.Add(int64(duration))
}

func (m *Metrics) recordHandler(duration time.Duration) {
	if m == nil {
		return
	}
	m.handled.Add(1)
	m.handlerNanos.Add(int64(duration))
}
