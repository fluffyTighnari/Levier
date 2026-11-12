// Middleware — register
//
// Split from materials.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package register

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fluffyTighnari/middleware/internal/id"
)

// ToolInput is one local tool declared by an App.
type ToolInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	SchemaJSON  string `json:"schema_json"`
	TimeoutMS   int    `json:"timeout_ms"`
}

// GlobalToolInput is one admin-managed global tool.
type GlobalToolInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	SchemaJSON  string   `json:"schema_json"`
	ExecutorRef string   `json:"executor_ref"`
	ConfigJSON  string   `json:"config_json"`
	TimeoutMS   int      `json:"timeout_ms"`
	ACL         []string `json:"acl,omitempty"`
}

// ToolMaterial is one resolved tool for TaskAssign.
type ToolMaterial struct {
	ToolID      string
	Scope       string
	AppID       string
	Name        string
	Description string
	SchemaJSON  string
	ExecutorRef string
	ConfigJSON  string
	TimeoutMS   int
	ACL         []string
}

// ReplaceLocalTools replaces all local tools for an App.
func (r *Registry) ReplaceLocalTools(ctx context.Context, appID string, tools []ToolInput) ([]ToolMaterial, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("register: postgres pool is nil")
	}
	if appID == "" {
		return nil, fmt.Errorf("register: app_id is required")
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if err := validateToolInput(tool.Name, tool.SchemaJSON, tool.TimeoutMS); err != nil {
			return nil, err
		}
		names = append(names, tool.Name)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("register: begin local tools: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, `
		DELETE FROM tool_defs
		WHERE scope = 'local' AND app_id = $1 AND NOT (name = ANY($2))
	`, appID, names); err != nil {
		return nil, fmt.Errorf("register: prune local tools: %w", err)
	}
	result := make([]ToolMaterial, 0, len(tools))
	for _, input := range tools {
		tool, err := scanToolMaterial(tx.QueryRow(ctx, `
			INSERT INTO tool_defs (
				tool_id, scope, app_id, name, description, schema_json,
				timeout_ms, version, created_at, updated_at
			) VALUES ($1, 'local', $2, $3, $4, $5, $6, 1, now(), now())
			ON CONFLICT (app_id, name) WHERE scope = 'local'
			DO UPDATE SET
				description = EXCLUDED.description,
				schema_json = EXCLUDED.schema_json,
				timeout_ms = EXCLUDED.timeout_ms,
				version = tool_defs.version + 1,
				updated_at = now()
			RETURNING tool_id, scope, app_id, name, description, schema_json,
			          executor_ref, COALESCE(config_json, ''), timeout_ms, acl_json
		`, "tool_"+id.New(), appID, input.Name, input.Description,
			input.SchemaJSON, defaultTimeout(input.TimeoutMS)))
		if err != nil {
			return nil, fmt.Errorf("register: upsert local tool: %w", err)
		}
		result = append(result, tool)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("register: commit local tools: %w", err)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// UpsertGlobalTool creates or updates one global tool.
func (r *Registry) UpsertGlobalTool(ctx context.Context, input GlobalToolInput) (ToolMaterial, error) {
	if r == nil || r.pool == nil {
		return ToolMaterial{}, fmt.Errorf("register: postgres pool is nil")
	}
	if err := validateToolInput(input.Name, input.SchemaJSON, input.TimeoutMS); err != nil {
		return ToolMaterial{}, err
	}
	if input.ExecutorRef == "" {
		return ToolMaterial{}, fmt.Errorf("register: executor_ref is required for global tools")
	}
	if input.ConfigJSON != "" && !json.Valid([]byte(input.ConfigJSON)) {
		return ToolMaterial{}, fmt.Errorf("register: tool %q config_json is invalid", input.Name)
	}
	var aclJSON any
	if input.ACL != nil {
		encoded, err := json.Marshal(input.ACL)
		if err != nil {
			return ToolMaterial{}, fmt.Errorf("register: encode acl: %w", err)
		}
		aclJSON = encoded
	}
	tool, err := scanToolMaterial(r.pool.QueryRow(ctx, `
		INSERT INTO tool_defs (
			tool_id, scope, app_id, name, description, schema_json,
			executor_ref, config_json, timeout_ms, acl_json, version, created_at, updated_at
		) VALUES ($1, 'global', NULL, $2, $3, $4, $5, $6, $7, $8, 1, now(), now())
		ON CONFLICT (name) WHERE scope = 'global'
		DO UPDATE SET
			description = EXCLUDED.description,
			schema_json = EXCLUDED.schema_json,
			executor_ref = EXCLUDED.executor_ref,
			config_json = EXCLUDED.config_json,
			timeout_ms = EXCLUDED.timeout_ms,
			acl_json = EXCLUDED.acl_json,
			version = tool_defs.version + 1,
			updated_at = now()
		RETURNING tool_id, scope, app_id, name, description, schema_json,
		          executor_ref, COALESCE(config_json, ''), timeout_ms, acl_json
	`, "tool_"+id.New(), input.Name, input.Description, input.SchemaJSON,
		input.ExecutorRef, nullString(input.ConfigJSON), defaultTimeout(input.TimeoutMS), aclJSON))
	if err != nil {
		return ToolMaterial{}, fmt.Errorf("register: upsert global tool: %w", err)
	}
	return tool, nil
}

// Resolution describes the result of tool resolution.
type Resolution string

// ResolveTool resolves a local tool first, then a global tool with ACL.
func (r *Registry) ResolveTool(ctx context.Context, appID, name string) (ToolMaterial, bool, error) {
	tool, resolution, err := r.ResolveToolDecision(ctx, appID, name)
	return tool, resolution == ResolutionFound, err
}

// ResolveToolDecision resolves a tool and distinguishes missing from denied.
func (r *Registry) ResolveToolDecision(ctx context.Context, appID, name string) (ToolMaterial, Resolution, error) {
	if r == nil || r.pool == nil {
		return ToolMaterial{}, ResolutionNotFound, fmt.Errorf("register: postgres pool is nil")
	}
	local, err := scanToolMaterial(r.pool.QueryRow(ctx, `
		SELECT tool_id, scope, app_id, name, description, schema_json,
		       executor_ref, COALESCE(config_json, ''), timeout_ms, acl_json
		FROM tool_defs
		WHERE scope = 'local' AND app_id = $1 AND name = $2
	`, appID, name))
	if err == nil {
		return local, ResolutionFound, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ToolMaterial{}, ResolutionNotFound, fmt.Errorf("register: resolve local tool: %w", err)
	}
	global, err := scanToolMaterial(r.pool.QueryRow(ctx, `
		SELECT tool_id, scope, app_id, name, description, schema_json,
		       executor_ref, COALESCE(config_json, ''), timeout_ms, acl_json
		FROM tool_defs
		WHERE scope = 'global' AND name = $1
	`, name))
	if errors.Is(err, pgx.ErrNoRows) {
		return ToolMaterial{}, ResolutionNotFound, nil
	}
	if err != nil {
		return ToolMaterial{}, ResolutionNotFound, fmt.Errorf("register: resolve global tool: %w", err)
	}
	if !aclAllows(global.ACL, appID) {
		return ToolMaterial{}, ResolutionDenied, nil
	}
	if global.ConfigJSON != "" {
		resolved, err := r.resolveString(ctx, global.ConfigJSON)
		if err != nil {
			return ToolMaterial{}, ResolutionNotFound, fmt.Errorf("register: resolve tool config: %w", err)
		}
		global.ConfigJSON = resolved
	}
	return global, ResolutionFound, nil
}

func scanToolMaterials(rows pgx.Rows) ([]ToolMaterial, error) {
	defer rows.Close()
	var result []ToolMaterial
	for rows.Next() {
		tool, err := scanToolMaterial(rows)
		if err != nil {
			return nil, fmt.Errorf("register: scan tool: %w", err)
		}
		result = append(result, tool)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("register: iterate tools: %w", err)
	}
	return result, nil
}

func scanToolMaterial(row pgx.Row) (ToolMaterial, error) {
	var tool ToolMaterial
	var acl []byte
	var appID, executorRef pgtype.Text
	if err := row.Scan(
		&tool.ToolID, &tool.Scope, &appID, &tool.Name, &tool.Description,
		&tool.SchemaJSON, &executorRef, &tool.ConfigJSON, &tool.TimeoutMS, &acl,
	); err != nil {
		return ToolMaterial{}, err
	}
	if appID.Valid {
		tool.AppID = appID.String
	}
	if executorRef.Valid {
		tool.ExecutorRef = executorRef.String
	}
	if len(acl) > 0 {
		if err := json.Unmarshal(acl, &tool.ACL); err != nil {
			return ToolMaterial{}, fmt.Errorf("decode tool acl: %w", err)
		}
	}
	return tool, nil
}

func validateToolInput(name, schemaJSON string, timeoutMS int) error {
	if name == "" {
		return fmt.Errorf("register: tool name is required")
	}
	if schemaJSON == "" || !json.Valid([]byte(schemaJSON)) {
		return fmt.Errorf("register: tool %q schema_json is invalid", name)
	}
	if timeoutMS < 0 {
		return fmt.Errorf("register: tool %q timeout_ms must not be negative", name)
	}
	return nil
}

func defaultTimeout(value int) int {
	if value <= 0 {
		return 60_000
	}
	return value
}

func aclAllows(acl []string, appID string) bool {
	if acl == nil {
		return true
	}
	for _, value := range acl {
		if value == appID || value == "*" {
			return true
		}
	}
	return false
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
