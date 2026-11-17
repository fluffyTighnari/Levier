// Levier — internal/db
//
// P5-37 schema and index tests.
//
// Copyright 2026 The Levier Authors.
package db

import (
	"context"
	"testing"
)

func TestSchemaTablesIndexesAndConstraints(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	for _, index := range []string{
		"uq_tool_defs_local",
		"uq_tool_defs_global",
		"idx_tool_defs_scope",
		"idx_sessions_app_status",
		"idx_sessions_expires",
		"idx_tool_invocations_run",
		"idx_tool_invocations_status",
		"idx_runs_app_status",
		"idx_run_events_type",
		"uq_apps_api_key_hash",
		"idx_apps_status",
		"idx_app_mcp_refs_mcp",
	} {
		var exists bool
		if err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1)",
			index,
		).Scan(&exists); err != nil {
			t.Fatalf("check index %s: %v", index, err)
		}
		if !exists {
			t.Fatalf("index %q does not exist", index)
		}
	}

	var hasEventAttempt bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'sessions'
			  AND column_name = 'event_attempt'
		)`).Scan(&hasEventAttempt); err != nil {
		t.Fatalf("check sessions.event_attempt: %v", err)
	}
	if !hasEventAttempt {
		t.Fatal("sessions.event_attempt is missing")
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO tool_defs (tool_id, scope, app_id, name, schema_json)
		VALUES ('bad-local', 'local', NULL, 'bad', '{}')
	`); err == nil {
		t.Fatal("local tool without app_id was accepted")
	}

	appID := "app-schema-" + t.Name()
	if _, err := pool.Exec(ctx, `
		INSERT INTO apps (app_id, name, api_key_hash) VALUES ($1, $1, 'hash')
	`, appID); err != nil {
		t.Fatalf("insert app: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tool_defs (tool_id, scope, app_id, name, schema_json)
		VALUES ('local-1', 'local', $1, 'search', '{}')
	`, appID); err != nil {
		t.Fatalf("insert local tool: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tool_defs (tool_id, scope, app_id, name, schema_json)
		VALUES ('local-2', 'local', $1, 'search', '{}')
	`, appID); err == nil {
		t.Fatal("duplicate local tool name was accepted")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tool_defs (tool_id, scope, name, schema_json)
		VALUES ('global-1', 'global', 'fetch', '{}')
	`); err == nil {
		t.Fatal("global tool without executor_ref was accepted")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO tool_defs (tool_id, scope, name, schema_json, executor_ref)
		VALUES ('global-2', 'global', 'fetch', '{}', 'builtin:http')
	`); err != nil {
		t.Fatalf("insert global tool: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (session_id, app_id, status, event_attempt, event_watermark)
		VALUES ('session-1', $1, 'bogus', 1, 1)
	`, appID); err == nil {
		t.Fatal("invalid session status was accepted")
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (session_id, app_id, status, event_attempt, event_watermark)
		VALUES ('session-1', $1, 'active', 1, 1)
	`, appID); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	if _, err := pool.Exec(ctx,
		"INSERT INTO runs (run_id, app_id, status, attempt) VALUES ('run-bad', $1, 'bogus', 0)",
		appID,
	); err == nil {
		t.Fatal("invalid run status was accepted")
	}
}
