// Middleware — internal/runstore
//
// PostgreSQL RunStore integration tests.
//
// Copyright 2026 The Middleware Authors.
package runstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/middleware/internal/db"
	"github.com/fluffyTighnari/middleware/internal/id"
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

func createApp(t *testing.T, store *Store) App {
	t.Helper()
	app, err := store.CreateApp(context.Background(), App{
		AppID:      "app-" + id.New(),
		Name:       "app-" + id.New(),
		APIKeyHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	return app
}

func TestCreateRunAndGetRun(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)

	params := json.RawMessage(`{"max_steps":8}`)
	run, err := store.CreateRun(ctx, Run{
		RunID:          "run-" + id.New(),
		AppID:          app.AppID,
		IdempotencyKey: "idem-1",
		RequestHash:    "hash-1",
		Runtime:        "normal",
		Strategy:       "echo",
		Params:         params,
		Model:          json.RawMessage(`{"provider":"anthropic","model":"claude-test"}`),
		TraceID:        "trace-1",
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if run.Status != "queued" {
		t.Fatalf("status = %q, want queued", run.Status)
	}
	if run.RunTimeoutMS != 1_800_000 {
		t.Fatalf("timeout = %d", run.RunTimeoutMS)
	}

	got, err := store.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.AppID != app.AppID || got.Strategy != "echo" || got.TraceID != "trace-1" {
		t.Fatalf("run = %+v", got)
	}
	var gotParams, wantParams any
	if err := json.Unmarshal(got.Params, &gotParams); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if err := json.Unmarshal(params, &wantParams); err != nil {
		t.Fatalf("decode want params: %v", err)
	}
	if !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("params = %s, want %s", got.Params, params)
	}

	if _, err := store.GetRun(ctx, "missing"); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("GetRun(missing) error = %v, want ErrRunNotFound", err)
	}
}

func TestCreateRunIdempotency(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)

	first, err := store.CreateRun(ctx, Run{
		RunID:          "run-" + id.New(),
		AppID:          app.AppID,
		IdempotencyKey: "same-key",
		RequestHash:    "hash-a",
	})
	if err != nil {
		t.Fatalf("first CreateRun() error = %v", err)
	}

	second, err := store.CreateRun(ctx, Run{
		RunID:          "run-" + id.New(),
		AppID:          app.AppID,
		IdempotencyKey: "same-key",
		RequestHash:    "hash-a",
	})
	if err != nil {
		t.Fatalf("second CreateRun() error = %v", err)
	}
	if second.RunID != first.RunID {
		t.Fatalf("second run ID = %q, want %q", second.RunID, first.RunID)
	}

	_, err = store.CreateRun(ctx, Run{
		RunID:          "run-" + id.New(),
		AppID:          app.AppID,
		IdempotencyKey: "same-key",
		RequestHash:    "hash-b",
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting CreateRun() error = %v, want ErrIdempotencyConflict", err)
	}
}

func TestAppendEventIsIdempotentAndAdvancesProjection(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, err := store.CreateRun(ctx, Run{
		RunID: "run-" + id.New(),
		AppID: app.AppID,
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	event := Event{
		RunID:   run.RunID,
		Attempt: 1,
		Seq:     1,
		Source:  "daemon",
		Type:    "RUN_STATUS",
		Payload: json.RawMessage(`{"status":"running"}`),
	}
	inserted, err := store.AppendEvent(ctx, event)
	if err != nil || !inserted {
		t.Fatalf("first AppendEvent() inserted=%v error=%v", inserted, err)
	}
	inserted, err = store.AppendEvent(ctx, event)
	if err != nil {
		t.Fatalf("duplicate AppendEvent() error = %v", err)
	}
	if inserted {
		t.Fatal("duplicate AppendEvent() inserted = true, want false")
	}

	var count int
	if err := store.pool.QueryRow(ctx,
		"SELECT count(*) FROM run_events WHERE run_id = $1", run.RunID,
	).Scan(&count); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 1 {
		t.Fatalf("event count = %d, want 1", count)
	}

	got, err := store.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.LastEventAttempt != 1 || got.LastEventSeq != 1 {
		t.Fatalf("projection = (%d,%d), want (1,1)", got.LastEventAttempt, got.LastEventSeq)
	}

	// A newer attempt advances the projection.
	if _, err := store.AppendEvent(ctx, Event{
		RunID: run.RunID, Attempt: 2, Seq: 1, Source: "daemon", Type: "RUN_STATUS",
	}); err != nil {
		t.Fatalf("append attempt 2: %v", err)
	}
	got, _ = store.GetRun(ctx, run.RunID)
	if got.LastEventAttempt != 2 || got.LastEventSeq != 1 {
		t.Fatalf("projection = (%d,%d), want (2,1)", got.LastEventAttempt, got.LastEventSeq)
	}

	// An older attempt must not move the watermark backwards.
	if _, err := store.AppendEvent(ctx, Event{
		RunID: run.RunID, Attempt: 1, Seq: 9, Source: "daemon", Type: "RUN_STATUS",
	}); err != nil {
		t.Fatalf("append old attempt: %v", err)
	}
	got, _ = store.GetRun(ctx, run.RunID)
	if got.LastEventAttempt != 2 || got.LastEventSeq != 1 {
		t.Fatalf("projection moved backwards: (%d,%d)", got.LastEventAttempt, got.LastEventSeq)
	}
}

func TestRunPersistsAcrossStoreInstances(t *testing.T) {
	pool := testPool(t)
	store := New(pool)
	ctx := context.Background()
	app := createApp(t, store)
	run, err := store.CreateRun(ctx, Run{
		RunID: "run-" + id.New(),
		AppID: app.AppID,
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	other := New(pool)
	got, err := other.GetRun(ctx, run.RunID)
	if err != nil {
		t.Fatalf("second Store GetRun() error = %v", err)
	}
	if got.RunID != run.RunID {
		t.Fatalf("run ID = %q, want %q", got.RunID, run.RunID)
	}
}

func TestInvalidEventRejected(t *testing.T) {
	store := New(testPool(t))
	ctx := context.Background()
	app := createApp(t, store)
	run, _ := store.CreateRun(ctx, Run{RunID: "run-" + id.New(), AppID: app.AppID})

	for _, event := range []Event{
		{RunID: run.RunID, Attempt: 0, Seq: 1, Source: "daemon", Type: "RUN_STATUS"},
		{RunID: run.RunID, Attempt: 1, Seq: 0, Source: "daemon", Type: "RUN_STATUS"},
		{RunID: run.RunID, Attempt: 1, Seq: 1, Source: "bad", Type: "RUN_STATUS"},
		{RunID: run.RunID, Attempt: 1, Seq: 1, Source: "daemon", Type: ""},
		{RunID: run.RunID, Attempt: 1, Seq: 1, Source: "daemon", Type: "RUN_STATUS", Payload: json.RawMessage(`{bad`)},
	} {
		if _, err := store.AppendEvent(ctx, event); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("AppendEvent(%+v) error = %v, want ErrInvalidEvent", event, err)
		}
	}
}
