// Middleware — internal/db
//
// PostgreSQL integration tests. Set MIDDLEWARE_TEST_DATABASE_URL to run.
//
// Copyright 2026 The Middleware Authors.
package db

import (
	"context"
	"testing"
)

func TestOpenRequiresURL(t *testing.T) {
	if _, err := Open(context.Background(), Config{}); err == nil {
		t.Fatal("Open() error = nil, want error")
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&applied); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if applied != 4 {
		t.Fatalf("applied migrations = %d, want 4", applied)
	}

	for _, table := range []string{"apps", "daemons", "runs", "run_events", "run_tasks"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %q was not created", table)
		}
	}
}
