// Middleware — internal/db
//
// PostgreSQL integration tests. Set MIDDLEWARE_TEST_DATABASE_URL to run.
//
// Copyright 2026 The Middleware Authors.
package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/middleware/internal/id"
)

func TestOpenRequiresURL(t *testing.T) {
	if _, err := Open(context.Background(), Config{}); err == nil {
		t.Fatal("Open() error = nil, want error")
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	if os.Getenv("MIDDLEWARE_TEST_POSTGRES") != "1" {
		t.Skip("MIDDLEWARE_TEST_POSTGRES is not set to 1")
	}
	dsn := os.Getenv("MIDDLEWARE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIDDLEWARE_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	schema := strings.ToLower("mw_test_" + id.New())

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("NewWithConfig() error = %v", err)
	}
	defer pool.Close()

	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
	}()

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
	if applied != 1 {
		t.Fatalf("applied migrations = %d, want 1", applied)
	}

	for _, table := range []string{"runs", "run_events", "run_tasks"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", schema+"."+table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("table %q was not created", table)
		}
	}
}
