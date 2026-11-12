// Middleware — internal/workflow
//
// Periodic expired-lease reaper loop.
//
// Copyright 2026 The Middleware Authors.
package workflow

import (
	"context"
	"time"
)

// ReapStore is the subset needed by the reaper loop.
type ReapStore interface {
	ReapExpired(ctx context.Context, now time.Time, limit int) (int, error)
}

// ReapResult reports one reap pass.
type ReapResult struct {
	Count int
	Err   error
}

// RunReaper reaps expired leases immediately and then every interval until ctx
// is done. It is synchronous within one loop, so reaps never overlap.
func RunReaper(
	ctx context.Context,
	store ReapStore,
	interval time.Duration,
	limit int,
	onResult func(ReapResult),
) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if limit <= 0 {
		limit = 100
	}

	reap := func() {
		if store == nil || ctx.Err() != nil {
			return
		}
		count, err := store.ReapExpired(ctx, time.Now().UTC(), limit)
		if onResult != nil {
			onResult(ReapResult{Count: count, Err: err})
		}
	}

	reap()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reap()
		}
	}
}
