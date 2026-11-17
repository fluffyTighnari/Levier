// Levier — register
//
// Split from materials.go (P8R.3).
// Copyright 2026 The Levier Authors.
package register

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// TaskMaterials is the registry output consumed by Control.
type TaskMaterials struct {
	Tools       []ToolMaterial
	MCPBindings []MCPMaterial
}

// Resolve implements SecretResolver.
func (r EnvSecretResolver) Resolve(_ context.Context, ref string) (string, error) {
	prefix := r.Prefix
	if prefix == "" {
		prefix = "LEVIER_SECRET_"
	}
	suffix := strings.ToUpper(strings.ReplaceAll(ref, "-", "_"))
	key := prefix + suffix
	if value, ok := os.LookupEnv(key); ok {
		return value, nil
	}
	if r.Prefix == "" {
		legacyKey := "MIDDLEWARE_SECRET_" + suffix
		if value, ok := os.LookupEnv(legacyKey); ok {
			return value, nil
		}
	}
	return "", fmt.Errorf("secret %q is not configured (env %s)", ref, key)
}

// Resolve implements SecretResolver.
func (r FileSecretResolver) Resolve(_ context.Context, ref string) (string, error) {
	value, ok := r.Values[ref]
	if !ok {
		return "", fmt.Errorf("secret %q is not configured", ref)
	}
	return value, nil
}

const (
	// ResolutionNotFound means no visible tool matched.
	ResolutionNotFound Resolution = "not_found"
	// ResolutionFound means a visible tool matched.
	ResolutionFound Resolution = "found"
	// ResolutionDenied means a global tool exists but ACL excludes the App.
	ResolutionDenied Resolution = "denied"
)

// AssembleTaskMaterials returns local/global tools and MCP bindings for an App.
func (r *Registry) AssembleTaskMaterials(ctx context.Context, appID string) (TaskMaterials, error) {
	if r == nil || r.pool == nil {
		return TaskMaterials{}, fmt.Errorf("register: postgres pool is nil")
	}
	localRows, err := r.pool.Query(ctx, `
		SELECT tool_id, scope, app_id, name, description, schema_json,
		       executor_ref, COALESCE(config_json, ''), timeout_ms, acl_json
		FROM tool_defs
		WHERE scope = 'local' AND app_id = $1
		ORDER BY name
	`, appID)
	if err != nil {
		return TaskMaterials{}, fmt.Errorf("register: list local tools: %w", err)
	}
	local, err := scanToolMaterials(localRows)
	if err != nil {
		return TaskMaterials{}, err
	}
	globalRows, err := r.pool.Query(ctx, `
		SELECT tool_id, scope, app_id, name, description, schema_json,
		       executor_ref, COALESCE(config_json, ''), timeout_ms, acl_json
		FROM tool_defs
		WHERE scope = 'global'
		ORDER BY name
	`)
	if err != nil {
		return TaskMaterials{}, fmt.Errorf("register: list global tools: %w", err)
	}
	global, err := scanToolMaterials(globalRows)
	if err != nil {
		return TaskMaterials{}, err
	}

	seen := make(map[string]struct{}, len(local))
	materials := TaskMaterials{Tools: make([]ToolMaterial, 0, len(local)+len(global))}
	for _, tool := range local {
		if _, ok := seen[tool.Name]; ok {
			continue
		}
		seen[tool.Name] = struct{}{}
		materials.Tools = append(materials.Tools, tool)
	}
	for _, tool := range global {
		if _, ok := seen[tool.Name]; ok {
			continue
		}
		if !aclAllows(tool.ACL, appID) {
			continue
		}
		seen[tool.Name] = struct{}{}
		materials.Tools = append(materials.Tools, tool)
	}

	mcpRows, err := r.pool.Query(ctx, `
		SELECT d.name, d.transport, d.config_json, COALESCE(d.allowed_tools_json, '')
		FROM app_mcp_refs ref
		JOIN mcp_server_defs d ON d.mcp_id = ref.mcp_id
		WHERE ref.app_id = $1
		ORDER BY d.name
	`, appID)
	if err != nil {
		return TaskMaterials{}, fmt.Errorf("register: list MCP refs: %w", err)
	}
	defer mcpRows.Close()
	for mcpRows.Next() {
		var name, transport, configJSON, allowedJSON string
		if err := mcpRows.Scan(&name, &transport, &configJSON, &allowedJSON); err != nil {
			return TaskMaterials{}, fmt.Errorf("register: scan MCP ref: %w", err)
		}
		binding, err := r.buildMCPMaterial(ctx, name, transport, configJSON, allowedJSON)
		if err != nil {
			return TaskMaterials{}, err
		}
		materials.MCPBindings = append(materials.MCPBindings, binding)
	}
	if err := mcpRows.Err(); err != nil {
		return TaskMaterials{}, fmt.Errorf("register: iterate MCP refs: %w", err)
	}
	return materials, nil
}
