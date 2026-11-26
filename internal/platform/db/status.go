// Levier — internal/platform/db
//
// Migration status without applying changes.
//
// Copyright 2026 The Levier Authors.
package db

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PendingMigration describes an embedded migration that has not been applied.
type PendingMigration struct {
	Version  int64  `json:"version"`
	Name     string `json:"name"`
	Filename string `json:"filename"`
}

// MigrationState summarizes the applied and pending migrations.
type MigrationState struct {
	Current int64              `json:"current"`
	Applied []int64            `json:"applied"`
	Pending []PendingMigration `json:"pending"`
}

// Status reads schema_migrations and compares it with the embedded migrations.
func Status(ctx context.Context, pool *pgxpool.Pool) (MigrationState, error) {
	if pool == nil {
		return MigrationState{}, fmt.Errorf("database pool is nil")
	}

	var exists bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&exists); err != nil {
		return MigrationState{}, fmt.Errorf("check schema_migrations: %w", err)
	}

	state := MigrationState{Applied: []int64{}, Pending: []PendingMigration{}}
	applied := map[int64]bool{}
	if exists {
		rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
		if err != nil {
			return MigrationState{}, fmt.Errorf("list applied migrations: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var version int64
			if err := rows.Scan(&version); err != nil {
				return MigrationState{}, fmt.Errorf("scan applied migration: %w", err)
			}
			applied[version] = true
			state.Applied = append(state.Applied, version)
		}
		if err := rows.Err(); err != nil {
			return MigrationState{}, fmt.Errorf("iterate applied migrations: %w", err)
		}
	}
	sort.Slice(state.Applied, func(i, j int) bool { return state.Applied[i] < state.Applied[j] })
	if len(state.Applied) > 0 {
		state.Current = state.Applied[len(state.Applied)-1]
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return MigrationState{}, fmt.Errorf("read migrations: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, name, err := parseMigrationName(entry.Name())
		if err != nil {
			return MigrationState{}, err
		}
		if applied[version] {
			continue
		}
		state.Pending = append(state.Pending, PendingMigration{
			Version:  version,
			Name:     name,
			Filename: entry.Name(),
		})
	}
	sort.Slice(state.Pending, func(i, j int) bool {
		return state.Pending[i].Version < state.Pending[j].Version
	})
	return state, nil
}
