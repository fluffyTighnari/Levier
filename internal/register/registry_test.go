// Middleware — internal/register
//
// App registration and API-key tests.
//
// Copyright 2026 The Middleware Authors.
package register

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/middleware/internal/db"
	"github.com/fluffyTighnari/middleware/internal/id"
	"github.com/fluffyTighnari/middleware/internal/runstore"
)

func testPool(t *testing.T) *pgxpool.Pool {
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
	return pool
}

func TestRegisterAppAndAuthenticate(t *testing.T) {
	registry := New(testPool(t))
	ctx := context.Background()

	app, key, created, err := registry.RegisterApp(ctx, "my-app", true)
	if err != nil {
		t.Fatalf("RegisterApp() error = %v", err)
	}
	if !created || key == "" || !strings.HasPrefix(key, "mw_") {
		t.Fatalf("created=%v key=%q", created, key)
	}
	if app.AppID == "" || app.Name != "my-app" || !app.Multiagent || app.Status != "active" {
		t.Fatalf("app = %+v", app)
	}
	if app.APIKeyHash == key || app.APIKeyHash == "" {
		t.Fatal("API key was stored in plaintext or not hashed")
	}

	authenticated, err := registry.AuthenticateAPIKey(ctx, key)
	if err != nil {
		t.Fatalf("AuthenticateAPIKey() error = %v", err)
	}
	if authenticated.AppID != app.AppID {
		t.Fatalf("authenticated app = %q, want %q", authenticated.AppID, app.AppID)
	}
	if _, err := registry.AuthenticateAPIKey(ctx, "mw_wrong"); !errors.Is(err, runstore.ErrAppNotFound) {
		t.Fatalf("wrong key error = %v", err)
	}

	duplicate, duplicateKey, duplicateCreated, err := registry.RegisterApp(ctx, "my-app", false)
	if err != nil {
		t.Fatalf("duplicate RegisterApp() error = %v", err)
	}
	if duplicateCreated || duplicateKey != "" || duplicate.AppID != app.AppID {
		t.Fatalf("duplicate = %+v key=%q created=%v", duplicate, duplicateKey, duplicateCreated)
	}
}

func TestUpdateAndRotateApp(t *testing.T) {
	registry := New(testPool(t))
	ctx := context.Background()
	app, key, _, err := registry.RegisterApp(ctx, "before", false)
	if err != nil {
		t.Fatalf("RegisterApp() error = %v", err)
	}
	if _, err := registry.UpdateApp(ctx, app.AppID, "after", true); err != nil {
		t.Fatalf("UpdateApp() error = %v", err)
	}
	updated, err := registry.GetApp(ctx, app.AppID)
	if err != nil || updated.Name != "after" || !updated.Multiagent {
		t.Fatalf("updated = %+v, %v", updated, err)
	}
	byName, err := registry.GetAppByName(ctx, "after")
	if err != nil || byName.AppID != app.AppID {
		t.Fatalf("GetAppByName() = %+v, %v", byName, err)
	}

	newKey, err := registry.RotateAPIKey(ctx, app.AppID)
	if err != nil {
		t.Fatalf("RotateAPIKey() error = %v", err)
	}
	if newKey == "" || newKey == key {
		t.Fatalf("new key = %q", newKey)
	}
	if _, err := registry.AuthenticateAPIKey(ctx, key); !errors.Is(err, runstore.ErrAppNotFound) {
		t.Fatalf("old key error = %v", err)
	}
	if _, err := registry.AuthenticateAPIKey(ctx, newKey); err != nil {
		t.Fatalf("new key error = %v", err)
	}
}
