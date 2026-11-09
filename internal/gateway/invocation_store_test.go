// Middleware — internal/gateway
//
// PostgreSQL invocation store integration tests.
//
// Copyright 2026 The Middleware Authors.
package gateway

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/db"
	"github.com/fluffyTighnari/middleware/internal/id"
)

func testGatewayPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("MIDDLEWARE_TEST_POSTGRES") != "1" {
		t.Skip("MIDDLEWARE_TEST_POSTGRES is not set to 1")
	}
	dsn := os.Getenv("MIDDLEWARE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("MIDDLEWARE_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := strings.ToLower("mw_test_" + id.New())
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("NewWithConfig() error = %v", err)
	}
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		pool.Close()
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE")
		pool.Close()
	})
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO apps (app_id, name, api_key_hash) VALUES ('app-1', 'app-1', 'hash')
	`); err != nil {
		t.Fatalf("insert app: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO runs (run_id, app_id, status) VALUES ('run-1', 'app-1', 'queued')
	`); err != nil {
		t.Fatalf("insert run: %v", err)
	}
	return pool
}

func TestPostgresInvocationStoreLifecycle(t *testing.T) {
	store := NewPostgresInvocationStore(testGatewayPool(t))
	ctx := context.Background()

	owner, stored, err := store.Begin(ctx, "inv-1", "run-1", "search")
	if err != nil || !owner || stored != nil {
		t.Fatalf("Begin() = %v, %+v, %v", owner, stored, err)
	}
	owner, stored, err = store.Begin(ctx, "inv-1", "run-1", "search")
	if err != nil || owner || stored != nil {
		t.Fatalf("second Begin() = %v, %+v, %v", owner, stored, err)
	}
	if err := store.Complete(ctx, "inv-1", &runtimev1.ToolCallResult{
		InvocationId: "inv-1", Ok: true, ResultJson: `"done"`,
	}); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	owner, stored, err = store.Begin(ctx, "inv-1", "run-1", "search")
	if err != nil || owner || stored == nil {
		t.Fatalf("terminal Begin() = %v, %+v, %v", owner, stored, err)
	}
	if !stored.Result.GetOk() || stored.Result.GetResultJson() != `"done"` {
		t.Fatalf("stored result = %+v", stored.Result)
	}

	if _, _, err := store.Begin(ctx, "inv-2", "run-1", "search"); err != nil {
		t.Fatalf("Begin(inv-2) error = %v", err)
	}
	if err := store.Fail(ctx, "inv-2", toolError("inv-2", "TOOL_DENIED", "no", false)); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	stored, ok, err := store.Get(ctx, "inv-2")
	if err != nil || !ok {
		t.Fatalf("Get(inv-2) = %+v, %v, %v", stored, ok, err)
	}
	if stored.Result.GetOk() || stored.Result.GetError().GetCode() != "TOOL_DENIED" {
		t.Fatalf("failed result = %+v", stored.Result)
	}
}

func TestPostgresInvocationStoreReclaimsStaleClaim(t *testing.T) {
	pool := testGatewayPool(t)
	ctx := context.Background()
	store := NewPostgresInvocationStore(pool)
	if owner, _, err := store.Begin(ctx, "inv-1", "run-1", "search"); err != nil || !owner {
		t.Fatalf("Begin() = %v, %v", owner, err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE tool_invocations SET created_at = now() - interval '10 minutes'
		WHERE invocation_id = 'inv-1'
	`); err != nil {
		t.Fatalf("age invocation: %v", err)
	}

	fresh := NewPostgresInvocationStore(pool, WithStaleAfter(time.Minute))
	owner, stored, err := fresh.Begin(ctx, "inv-1", "run-1", "search")
	if err != nil || !owner || stored != nil {
		t.Fatalf("reclaim Begin() = %v, %+v, %v", owner, stored, err)
	}

	stale := NewPostgresInvocationStore(pool, WithStaleAfter(time.Hour))
	owner, stored, err = stale.Begin(ctx, "inv-1", "run-1", "search")
	if err != nil || owner || stored != nil {
		t.Fatalf("non-stale Begin() = %v, %+v, %v", owner, stored, err)
	}
}
