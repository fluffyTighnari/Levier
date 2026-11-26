package maintenance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/levier/internal/platform/db"
	"github.com/fluffyTighnari/levier/internal/platform/id"
	"github.com/fluffyTighnari/levier/internal/testing/testutil"
)

func TestPolicyNormalize(t *testing.T) {
	policy, err := Policy{RunDays: 30, SessionDays: 7}.Normalize()
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if policy.BatchSize != 1000 {
		t.Fatalf("BatchSize = %d, want 1000", policy.BatchSize)
	}
	if _, err := (Policy{RunDays: 0, SessionDays: 7}).Normalize(); err == nil {
		t.Fatal("Normalize() accepted zero run days")
	}
	if _, err := (Policy{RunDays: 30, SessionDays: 7, BatchSize: 10001}).Normalize(); err == nil {
		t.Fatal("Normalize() accepted oversized batch")
	}
}

func TestStatusAndPrune(t *testing.T) {
	pool := maintenanceTestPool(t)
	ctx := context.Background()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	now := time.Now().UTC()
	old := now.Add(-40 * 24 * time.Hour)
	recent := now.Add(-1 * 24 * time.Hour)
	oldExpiry := now.Add(-10 * 24 * time.Hour)

	if _, err := pool.Exec(ctx, `
		INSERT INTO apps (app_id, name, api_key_hash) VALUES ($1, $2, $3)
	`, "app-maintenance", "maintenance-test", "hash"); err != nil {
		t.Fatalf("insert app: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO runs (run_id, app_id, status, updated_at) VALUES
			('old-completed', 'app-maintenance', 'completed', $1),
			('old-running', 'app-maintenance', 'running', $1),
			('recent-completed', 'app-maintenance', 'completed', $2)
	`, old, recent); err != nil {
		t.Fatalf("insert runs: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO run_events (run_id, attempt, seq, source, type, ts_ms)
		VALUES ('old-completed', 1, 1, 'daemon', 'run.started', $1)
	`, now.UnixMilli()); err != nil {
		t.Fatalf("insert event: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO run_tasks (task_id, run_id, task_type, status, available_at, created_at, updated_at)
		VALUES ('old-task', 'old-completed', 'RUN_DISPATCH', 'DONE', $1, $1, $1)
	`, old); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tool_invocations (invocation_id, run_id, tool_name, status, created_at)
		VALUES ('old-invocation', 'old-completed', 'echo', 'completed', $1)
	`, old); err != nil {
		t.Fatalf("insert invocation: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (session_id, app_id, status, current_run_id, expires_at, created_at, updated_at)
		VALUES
			('expired-session', 'app-maintenance', 'expired', '', $1, $1, $1),
			('idle-active-session', 'app-maintenance', 'active', '', $1, $1, $1),
			('paused-session', 'app-maintenance', 'paused', '', $1, $1, $1),
			('busy-session', 'app-maintenance', 'active', 'old-running', $1, $1, $1)
	`, oldExpiry); err != nil {
		t.Fatalf("insert sessions: %v", err)
	}
	policy := Policy{Enabled: true, RunDays: 30, SessionDays: 7, BatchSize: 2}
	status, err := GetStatus(ctx, pool, policy)
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if status.RunsTotal != 3 || status.RunsActive != 1 || status.RunsTerminal != 2 || status.RunsEligible != 1 {
		t.Fatalf("run status = %+v", status)
	}
	if status.EventsTotal != 1 || status.TasksByStatus["DONE"] != 1 ||
		status.InvocationsByStatus["completed"] != 1 {
		t.Fatalf("child status = %+v", status)
	}
	if status.SessionsTotal != 4 || status.SessionsActive != 2 || status.SessionsPaused != 1 ||
		status.SessionsExpired != 1 || status.SessionsEligible != 3 {
		t.Fatalf("session status = %+v", status)
	}
	if status.OldestTerminalAt == nil {
		t.Fatal("OldestTerminalAt = nil")
	}

	dryRun, err := Prune(ctx, pool, policy, false)
	if err != nil {
		t.Fatalf("dry-run Prune() error = %v", err)
	}
	if !dryRun.DryRun || dryRun.RunsEligible != 1 || dryRun.RunsDeleted != 0 ||
		dryRun.SessionsEligible != 3 || dryRun.SessionsDeleted != 0 {
		t.Fatalf("dry-run result = %+v", dryRun)
	}
	var dryRunRuns int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM runs WHERE run_id = 'old-completed'`).Scan(&dryRunRuns); err != nil {
		t.Fatalf("count dry-run run: %v", err)
	}
	if dryRunRuns != 1 {
		t.Fatal("dry-run deleted a run")
	}

	result, err := Prune(ctx, pool, policy, true)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if result.DryRun || result.RunsDeleted != 1 || result.SessionsExpired != 2 || result.SessionsDeleted != 3 {
		t.Fatalf("prune result = %+v", result)
	}

	// The running run and its busy session must survive.
	var activeRuns, busySessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM runs WHERE run_id = 'old-running'`).Scan(&activeRuns); err != nil {
		t.Fatalf("count active run: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE session_id = 'busy-session'`).Scan(&busySessions); err != nil {
		t.Fatalf("count busy session: %v", err)
	}
	if activeRuns != 1 || busySessions != 1 {
		t.Fatalf("active data removed: runs=%d sessions=%d", activeRuns, busySessions)
	}

	var oldRuns, oldEvents, oldTasks, oldInvocations, oldSessions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM runs WHERE run_id = 'old-completed'`).Scan(&oldRuns); err != nil {
		t.Fatalf("count old run: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM run_events WHERE run_id = 'old-completed'`).Scan(&oldEvents); err != nil {
		t.Fatalf("count old events: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM run_tasks WHERE run_id = 'old-completed'`).Scan(&oldTasks); err != nil {
		t.Fatalf("count old tasks: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tool_invocations WHERE run_id = 'old-completed'`).Scan(&oldInvocations); err != nil {
		t.Fatalf("count old invocations: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE session_id IN ('expired-session', 'idle-active-session', 'paused-session')`).Scan(&oldSessions); err != nil {
		t.Fatalf("count old sessions: %v", err)
	}
	if oldRuns != 0 || oldEvents != 0 || oldTasks != 0 || oldInvocations != 0 || oldSessions != 0 {
		t.Fatalf("eligible data remained: run=%d events=%d tasks=%d invocations=%d sessions=%d",
			oldRuns, oldEvents, oldTasks, oldInvocations, oldSessions)
	}
}

func TestPruneRejectsDisabledRetention(t *testing.T) {
	pool := maintenanceTestPool(t)
	ctx := context.Background()
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	_, err := Prune(ctx, pool, Policy{
		Enabled: false, RunDays: 30, SessionDays: 7, BatchSize: 10,
	}, true)
	if !errors.Is(err, ErrRetentionDisabled) {
		t.Fatalf("Prune() error = %v, want ErrRetentionDisabled", err)
	}
}

func maintenanceTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testutil.EnvOrLegacy("LEVIER_TEST_POSTGRES", "MIDDLEWARE_TEST_POSTGRES") != "1" {
		t.Skip("LEVIER_TEST_POSTGRES is not set to 1")
	}
	dsn := testutil.EnvOrLegacy("LEVIER_TEST_DATABASE_URL", "MIDDLEWARE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEVIER_TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	schema := strings.ToLower("mw_maintenance_" + id.New())
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
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
	return pool
}
