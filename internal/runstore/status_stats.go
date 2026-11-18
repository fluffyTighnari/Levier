// Levier — aggregate run and app statistics.
//
// Copyright 2026 The Levier Authors.
package runstore

import (
	"context"
	"fmt"
)

// Stats summarizes persisted runs by status.
type Stats struct {
	Total    int64            `json:"total"`
	ByStatus map[string]int64 `json:"by_status"`
}

// Stats returns aggregate run counts.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	if s == nil || s.pool == nil {
		return Stats{}, fmt.Errorf("runstore: postgres pool is nil")
	}
	stats := Stats{ByStatus: make(map[string]int64)}
	rows, err := s.pool.Query(ctx, "SELECT status, count(*) FROM runs GROUP BY status")
	if err != nil {
		return Stats{}, fmt.Errorf("count runs by status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return Stats{}, fmt.Errorf("scan run status count: %w", err)
		}
		stats.ByStatus[status] = count
		stats.Total += count
	}
	if err := rows.Err(); err != nil {
		return Stats{}, fmt.Errorf("iterate run status counts: %w", err)
	}
	return stats, nil
}

// CountApps returns the number of registered Apps.
func (s *Store) CountApps(ctx context.Context) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, fmt.Errorf("runstore: postgres pool is nil")
	}
	var count int64
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM apps").Scan(&count); err != nil {
		return 0, fmt.Errorf("count apps: %w", err)
	}
	return count, nil
}
