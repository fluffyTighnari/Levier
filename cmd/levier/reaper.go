// Levier - expired control-plane connection reaper.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/fluffyTighnari/levier/internal/platform/observe"
	"github.com/fluffyTighnari/levier/internal/state/cluster"
)

func startConnectionReaper(
	ctx context.Context,
	store *cluster.Store,
	interval time.Duration,
	limit int,
	logger *slog.Logger,
	metrics ...*observe.Registry,
) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			reapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			result, err := store.ReapExpired(reapCtx, time.Now().UTC())
			cancel()
			if err != nil {
				if ctx.Err() == nil {
					logger.Warn("connection reaper failed", "err", err)
				}
				continue
			}
			total := result.Gateways + result.Runtimes + result.Apps + result.Exec + result.Capacity
			if total == 0 {
				continue
			}
			if len(metrics) > 0 && metrics[0] != nil {
				metrics[0].Add(observe.MetricConnectionReapedTotal,
					"Cumulative expired control-plane leases reclaimed.", nil, float64(total))
			}
			logger.Info("connection reaper reclaimed expired leases",
				"gateways", result.Gateways,
				"runtimes", result.Runtimes,
				"apps", result.Apps,
				"execution_leases", result.Exec,
				"capacity_leases", result.Capacity,
				"limit", limit,
			)
		}
	}()
}
