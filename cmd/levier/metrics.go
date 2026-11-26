// Levier - control-plane lease metrics collection.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/levier/internal/platform/observe"
)

// collectControlPlaneMetrics reads lease snapshots from PostgreSQL at scrape
// time. These gauges are derived state; the outbox and lease tables remain the
// source of truth.
func collectControlPlaneMetrics(ctx context.Context, pool *pgxpool.Pool, metrics *observe.Registry) {
	if pool == nil || metrics == nil {
		return
	}
	rows, err := pool.Query(ctx, `
		SELECT role, status, count(*)
		FROM (
			SELECT unnest(roles) AS role, status
			FROM gateway_instances
		) grouped
		GROUP BY role, status
	`)
	if err == nil {
		for rows.Next() {
			var role, status string
			var count int64
			if rows.Scan(&role, &status, &count) == nil {
				metrics.Set(observe.MetricGatewayInstances,
					"Current gateway instances by role and status.",
					map[string]string{"role": role, "status": status}, float64(count))
			}
		}
		rows.Close()
	}

	collectLeaseCounts(ctx, pool, metrics, "runtime_connections", observe.MetricRuntimeConnectionLease,
		"Current Runtime connection leases by status.")
	collectLeaseCounts(ctx, pool, metrics, "app_connections", observe.MetricAppConnectionLease,
		"Current App connection leases by status.")

	var lag float64
	if err := pool.QueryRow(ctx, `
		SELECT COALESCE(EXTRACT(EPOCH FROM (now() - min(created_at))), 0)
		FROM gateway_notifications
		WHERE published_at IS NULL
	`).Scan(&lag); err == nil {
		metrics.Set(observe.MetricNotificationLagSeconds,
			"Age in seconds of the oldest unpublished gateway notification.", nil, lag)
	}
}

func collectLeaseCounts(
	ctx context.Context,
	pool *pgxpool.Pool,
	metrics *observe.Registry,
	table, name, help string,
) {
	rows, err := pool.Query(ctx, "SELECT status, count(*) FROM "+table+" GROUP BY status")
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if rows.Scan(&status, &count) == nil {
			metrics.Set(name, help, map[string]string{"status": status}, float64(count))
		}
	}
}
