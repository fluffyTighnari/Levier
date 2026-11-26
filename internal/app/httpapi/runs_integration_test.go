// Levier — HTTP API + PostgreSQL transaction tests.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/levier/internal/platform/db"
	"github.com/fluffyTighnari/levier/internal/platform/id"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
	"github.com/fluffyTighnari/levier/internal/state/workflow"
)

func httpIntegrationPool(t *testing.T) *pgxpool.Pool {
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

type failingTxQueue struct {
	*workflow.PostgresStore
}

func (f failingTxQueue) EnqueueTx(context.Context, pgx.Tx, workflow.Task) (workflow.Task, error) {
	return workflow.Task{}, errors.New("queue down")
}

func runBody(t *testing.T, appID, key string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"app_id":       appID,
		"runtime":      "normal",
		"runtime_name": "echo1",
		"params":       map[string]any{},
		"input":        []map[string]any{{"role": "user", "content": "hello"}},
		"model":        map[string]any{"provider": "anthropic", "model": "claude-test"},
		"timeouts":     map[string]any{"run_ms": 1000, "tool_ms": 500},
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	if key == "" {
		return body
	}
	return body
}

func postRun(t *testing.T, url, key string, body []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url+"/api/v1/runs", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	return response
}

func TestCreateRunAndTasksAreTransactional(t *testing.T) {
	pool := httpIntegrationPool(t)
	ctx := context.Background()
	runs := runstore.New(pool)
	queue := workflow.NewPostgres(pool)
	app, err := runs.CreateApp(ctx, runstore.App{
		AppID: "app-" + id.New(), Name: "app-" + id.New(), APIKeyHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	server := NewServer(runs, queue, Config{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	key := "tx-" + id.New()
	body := runBody(t, app.AppID, key)
	for attempt := 0; attempt < 2; attempt++ {
		response := postRun(t, httpServer.URL, key, body)
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("attempt %d status = %d", attempt, response.StatusCode)
		}
		response.Body.Close()
	}

	var runCount, taskCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM runs WHERE app_id = $1", app.AppID).Scan(&runCount); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM run_tasks WHERE run_id IN (SELECT run_id FROM runs WHERE app_id = $1)", app.AppID).Scan(&taskCount); err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if runCount != 1 || taskCount != 2 {
		t.Fatalf("runs/tasks = %d/%d, want 1/2", runCount, taskCount)
	}
}

func TestCreateRunRollsBackWhenEnqueueFails(t *testing.T) {
	pool := httpIntegrationPool(t)
	ctx := context.Background()
	runs := runstore.New(pool)
	queue := workflow.NewPostgres(pool)
	app, err := runs.CreateApp(ctx, runstore.App{
		AppID: "app-" + id.New(), Name: "app-" + id.New(), APIKeyHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	server := NewServer(runs, failingTxQueue{PostgresStore: queue}, Config{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := postRun(t, httpServer.URL, "tx-fail-"+id.New(), runBody(t, app.AppID, ""))
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.StatusCode)
	}
	var body ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	response.Body.Close()
	if body.Error.Code != "QUEUE_UNAVAILABLE" {
		t.Fatalf("error code = %q", body.Error.Code)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM runs WHERE app_id = $1", app.AppID).Scan(&count); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if count != 0 {
		t.Fatalf("orphan runs = %d, want 0", count)
	}
}

func TestCreateRunTransactionTimeoutTask(t *testing.T) {
	pool := httpIntegrationPool(t)
	ctx := context.Background()
	runs := runstore.New(pool)
	queue := workflow.NewPostgres(pool)
	app, err := runs.CreateApp(ctx, runstore.App{
		AppID: "app-" + id.New(), Name: "app-" + id.New(), APIKeyHash: "hash",
	})
	if err != nil {
		t.Fatalf("CreateApp() error = %v", err)
	}
	server := NewServer(runs, queue, Config{})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response := postRun(t, httpServer.URL, "", runBody(t, app.AppID, ""))
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", response.StatusCode)
	}
	response.Body.Close()

	var availableAt time.Time
	if err := pool.QueryRow(ctx, `
		SELECT available_at FROM run_tasks
		WHERE run_id IN (SELECT run_id FROM runs WHERE app_id = $1)
		  AND task_type = 'RUN_TIMEOUT'
	`, app.AppID).Scan(&availableAt); err != nil {
		t.Fatalf("query timeout task: %v", err)
	}
	if time.Until(availableAt) > 2*time.Second {
		t.Fatalf("timeout task is too far in the future: %v", availableAt)
	}
}
