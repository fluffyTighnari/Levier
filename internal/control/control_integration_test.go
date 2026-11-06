// Middleware — internal/control
//
// Control + DaemonHub + RunStore + workflow integration test.
//
// Copyright 2026 The Middleware Authors.
package control

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	"github.com/fluffyTighnari/middleware/internal/connection"
	"github.com/fluffyTighnari/middleware/internal/db"
	"github.com/fluffyTighnari/middleware/internal/id"
	"github.com/fluffyTighnari/middleware/internal/runstore"
	"github.com/fluffyTighnari/middleware/internal/testutil/fakedaemon"
	"github.com/fluffyTighnari/middleware/internal/workflow"
)

func integrationPool(t *testing.T) *pgxpool.Pool {
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

func TestDispatchEchoRunEndToEnd(t *testing.T) {
	pool := integrationPool(t)
	ctx := context.Background()
	runs := runstore.New(pool)
	app, err := runs.CreateApp(ctx, runstore.App{
		AppID:      "app-" + id.New(),
		Name:       "app-" + id.New(),
		APIKeyHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	run, err := runs.CreateRun(ctx, runstore.Run{
		RunID:    "run-" + id.New(),
		AppID:    app.AppID,
		Runtime:  "normal",
		Strategy: "echo",
		Input:    json.RawMessage(`[{"role":"user","content":"hello"}]`),
		Model:    json.RawMessage(`{"provider":"anthropic","model":"claude-test"}`),
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}

	hub := connection.New(connection.Config{
		Authenticate: func(daemonID, token string) bool {
			return daemonID == "daemon-1" && token == "token-1"
		},
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	hub.Register(server)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		hub.Close()
		_ = listener.Close()
	})

	ctrl := New(runs, hub, Config{AckTimeout: 2 * time.Second})
	hub.SetOnEnvelope(ctrl.HandleEnvelope)
	ctrl.Start(ctx)
	t.Cleanup(ctrl.Close)

	queue := workflow.NewPostgres(pool)
	workers := workflow.NewPool(queue, ctrl, 1, workflow.WorkerConfig{
		Owner:             "control-test",
		LeaseTTL:          5 * time.Second,
		PollInterval:      10 * time.Millisecond,
		HeartbeatInterval: 100 * time.Millisecond,
		ShutdownTimeout:   time.Second,
	})
	workers.Start()
	t.Cleanup(workers.Stop)

	daemonCfg := fakedaemon.DefaultConfig()
	daemonCfg.DaemonID = "daemon-1"
	daemonCfg.AuthToken = "token-1"
	daemonCfg.HeartbeatInterval = 20 * time.Millisecond
	daemonCfg.AutoEcho = true
	daemon, err := fakedaemon.Dial(ctx, listener.Addr().String(), daemonCfg)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	t.Cleanup(func() { _ = daemon.Close() })

	if _, err := queue.Enqueue(ctx, workflow.NewTask(run.RunID, workflow.TaskDispatch, time.Now(), 0)); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, err := runs.GetRun(ctx, run.RunID)
		if err == nil && current.Status == "completed" {
			if current.Attempt != 1 || current.LeaseEpoch != 1 {
				t.Fatalf("attempt/epoch = %d/%d", current.Attempt, current.LeaseEpoch)
			}
			if current.AssignedDaemonID != "daemon-1" {
				t.Fatalf("assigned daemon = %q", current.AssignedDaemonID)
			}
			var count int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM run_events WHERE run_id = $1", run.RunID).Scan(&count); err != nil {
				t.Fatalf("count events: %v", err)
			}
			if count != 3 {
				t.Fatalf("event count = %d, want 3", count)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	current, _ := runs.GetRun(ctx, run.RunID)
	t.Fatalf("run status = %q, want completed", current.Status)
}
