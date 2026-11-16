// Middleware — internal/db
//
// Shared PostgreSQL integration test helper.
//
// Copyright 2026 The Middleware Authors.
package db

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/levier/internal/id"
	"github.com/fluffyTighnari/levier/internal/testutil"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testutil.EnvOrLegacy("LEVIER_TEST_POSTGRES", "MIDDLEWARE_TEST_POSTGRES") != "1" {
		t.Skip("LEVIER_TEST_POSTGRES is not set to 1")
	}
	dsn := testutil.EnvOrLegacy("LEVIER_TEST_DATABASE_URL", "MIDDLEWARE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEVIER_TEST_DATABASE_URL is not set")
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

	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		pool.Close()
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
		pool.Close()
	})
	return pool
}
