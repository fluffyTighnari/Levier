// Middleware — internal/register
//
// Tool and MCP catalog assembly for TaskAssign materials.
//
// Copyright 2026 The Middleware Authors.
package register

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

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
	TimeoutMS   int      `json:"timeout_ms"`
	ACL         []string `json:"acl,omitempty"`
}

// MCPServerInput is one registered MCP server definition.
type MCPServerInput struct {
	Name             string `json:"name"`
	Transport        string `json:"transport"`
	ConfigJSON       string `json:"config_json"`
	AllowedToolsJSON string `json:"allowed_tools_json"`
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
	TimeoutMS   int
	ACL         []string
}

// MCPMaterial is one resolved MCP binding for TaskAssign.
type MCPMaterial struct {
	ServerName   string
	Transport    string
	Command      string
	Args         []string
	Env          map[string]string
	URL          string
	Headers      map[string]string
	AllowedTools []string
	Namespace    string
}

// TaskMaterials is the registry output consumed by Control.
type TaskMaterials struct {
	Tools       []ToolMaterial
	MCPBindings []MCPMaterial
}

// SecretResolver resolves ${secret:name} placeholders.
type SecretResolver interface {
	Resolve(ctx context.Context, ref string) (string, error)
}

// EnvSecretResolver reads MIDDLEWARE_SECRET_<REF> environment variables.
type EnvSecretResolver struct {
	Prefix string
}

// Resolve implements SecretResolver.
func (r EnvSecretResolver) Resolve(_ context.Context, ref string) (string, error) {
	prefix := r.Prefix
	if prefix == "" {
		prefix = "MIDDLEWARE_SECRET_"
	}
	key := prefix + strings.ToUpper(strings.ReplaceAll(ref, "-", "_"))
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("secret %q is not configured (env %s)", ref, key)
	}
	return value, nil
}

// FileSecretResolver reads secrets from an in-memory JSON map.
type FileSecretResolver struct {
	Values map[string]string
}

// Resolve implements SecretResolver.
func (r FileSecretResolver) Resolve(_ context.Context, ref string) (string, error) {
	value, ok := r.Values[ref]
	if !ok {
		return "", fmt.Errorf("secret %q is not configured", ref)
	}
	return value, nil
}

// LoadFileSecretResolver reads a JSON object of secret name -> value.
func LoadFileSecretResolver(path string) (*FileSecretResolver, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read secret file: %w", err)
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("parse secret file: %w", err)
	}
	return &FileSecretResolver{Values: values}, nil
}

// NewWithSecrets returns a registry with a custom secret resolver.
func NewWithSecrets(pool *pgxpool.Pool, secrets SecretResolver) *Registry {
	if secrets == nil {
		secrets = EnvSecretResolver{}
	}
	return &Registry{pool: pool, secrets: secrets}
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
			          executor_ref, timeout_ms, acl_json
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
			executor_ref, timeout_ms, acl_json, version, created_at, updated_at
		) VALUES ($1, 'global', NULL, $2, $3, $4, $5, $6, $7, 1, now(), now())
		ON CONFLICT (name) WHERE scope = 'global'
		DO UPDATE SET
			description = EXCLUDED.description,
			schema_json = EXCLUDED.schema_json,
			executor_ref = EXCLUDED.executor_ref,
			timeout_ms = EXCLUDED.timeout_ms,
			acl_json = EXCLUDED.acl_json,
			version = tool_defs.version + 1,
			updated_at = now()
		RETURNING tool_id, scope, app_id, name, description, schema_json,
		          executor_ref, timeout_ms, acl_json
	`, "tool_"+id.New(), input.Name, input.Description, input.SchemaJSON,
		input.ExecutorRef, defaultTimeout(input.TimeoutMS), aclJSON))
	if err != nil {
		return ToolMaterial{}, fmt.Errorf("register: upsert global tool: %w", err)
	}
	return tool, nil
}

// UpsertMCPServer creates or updates one MCP server definition.
func (r *Registry) UpsertMCPServer(ctx context.Context, input MCPServerInput) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("register: postgres pool is nil")
	}
	if input.Name == "" || input.Transport == "" || input.ConfigJSON == "" {
		return fmt.Errorf("register: MCP name, transport, and config_json are required")
	}
	switch input.Transport {
	case "stdio", "sse", "streamable_http":
	default:
		return fmt.Errorf("register: invalid MCP transport %q", input.Transport)
	}
	if !json.Valid([]byte(input.ConfigJSON)) {
		return fmt.Errorf("register: MCP config_json is invalid")
	}
	if input.AllowedToolsJSON != "" && !json.Valid([]byte(input.AllowedToolsJSON)) {
		return fmt.Errorf("register: MCP allowed_tools_json is invalid")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO mcp_server_defs (mcp_id, name, transport, config_json, allowed_tools_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, now(), now())
		ON CONFLICT (name) DO UPDATE SET
			transport = EXCLUDED.transport,
			config_json = EXCLUDED.config_json,
			allowed_tools_json = EXCLUDED.allowed_tools_json,
			updated_at = now()
	`, "mcp_"+id.New(), input.Name, input.Transport, input.ConfigJSON, nullString(input.AllowedToolsJSON))
	if err != nil {
		return fmt.Errorf("register: upsert MCP server: %w", err)
	}
	return nil
}

// SetAppMCPRefs replaces an App's MCP server references.
func (r *Registry) SetAppMCPRefs(ctx context.Context, appID string, names []string) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("register: postgres pool is nil")
	}
	if appID == "" {
		return fmt.Errorf("register: app_id is required")
	}
	ids := make([]string, 0, len(names))
	for _, name := range names {
		var mcpID string
		if err := r.pool.QueryRow(ctx,
			"SELECT mcp_id FROM mcp_server_defs WHERE name = $1", name,
		).Scan(&mcpID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("register: MCP server %q is not registered", name)
			}
			return fmt.Errorf("register: lookup MCP server: %w", err)
		}
		ids = append(ids, mcpID)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("register: begin MCP refs: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, "DELETE FROM app_mcp_refs WHERE app_id = $1", appID); err != nil {
		return fmt.Errorf("register: clear MCP refs: %w", err)
	}
	for _, mcpID := range ids {
		if _, err := tx.Exec(ctx,
			"INSERT INTO app_mcp_refs (app_id, mcp_id) VALUES ($1, $2)",
			appID, mcpID,
		); err != nil {
			return fmt.Errorf("register: insert MCP ref: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("register: commit MCP refs: %w", err)
	}
	return nil
}

// Resolution describes the result of tool resolution.
type Resolution string

const (
	// ResolutionNotFound means no visible tool matched.
	ResolutionNotFound Resolution = "not_found"
	// ResolutionFound means a visible tool matched.
	ResolutionFound Resolution = "found"
	// ResolutionDenied means a global tool exists but ACL excludes the App.
	ResolutionDenied Resolution = "denied"
)

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
		       executor_ref, timeout_ms, acl_json
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
		       executor_ref, timeout_ms, acl_json
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
	return global, ResolutionFound, nil
}

// AssembleTaskMaterials returns local/global tools and MCP bindings for an App.
func (r *Registry) AssembleTaskMaterials(ctx context.Context, appID string) (TaskMaterials, error) {
	if r == nil || r.pool == nil {
		return TaskMaterials{}, fmt.Errorf("register: postgres pool is nil")
	}
	localRows, err := r.pool.Query(ctx, `
		SELECT tool_id, scope, app_id, name, description, schema_json,
		       executor_ref, timeout_ms, acl_json
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
		       executor_ref, timeout_ms, acl_json
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

func (r *Registry) buildMCPMaterial(ctx context.Context, name, transport, configJSON, allowedJSON string) (MCPMaterial, error) {
	var config struct {
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return MCPMaterial{}, fmt.Errorf("register: parse MCP %q config: %w", name, err)
	}
	var err error
	config.Command, err = r.resolveString(ctx, config.Command)
	if err != nil {
		return MCPMaterial{}, err
	}
	config.URL, err = r.resolveString(ctx, config.URL)
	if err != nil {
		return MCPMaterial{}, err
	}
	config.Env, err = r.resolveMap(ctx, config.Env)
	if err != nil {
		return MCPMaterial{}, err
	}
	config.Headers, err = r.resolveMap(ctx, config.Headers)
	if err != nil {
		return MCPMaterial{}, err
	}
	allowed := []string{}
	if allowedJSON != "" {
		if err := json.Unmarshal([]byte(allowedJSON), &allowed); err != nil {
			return MCPMaterial{}, fmt.Errorf("register: parse MCP %q allowed tools: %w", name, err)
		}
	}
	namespace := "mcp__" + sanitizeNamespace(name) + "__"
	return MCPMaterial{
		ServerName:   name,
		Transport:    transport,
		Command:      config.Command,
		Args:         config.Args,
		Env:          config.Env,
		URL:          config.URL,
		Headers:      config.Headers,
		AllowedTools: allowed,
		Namespace:    namespace,
	}, nil
}

func (r *Registry) resolveString(ctx context.Context, value string) (string, error) {
	for {
		start := strings.Index(value, "${secret:")
		if start < 0 {
			return value, nil
		}
		end := strings.Index(value[start:], "}")
		if end < 0 {
			return "", fmt.Errorf("register: malformed secret placeholder")
		}
		ref := value[start+len("${secret:") : start+end]
		secret, err := r.secretResolver().Resolve(ctx, ref)
		if err != nil {
			return "", err
		}
		value = value[:start] + secret + value[start+end+1:]
	}
}

func (r *Registry) resolveMap(ctx context.Context, values map[string]string) (map[string]string, error) {
	if values == nil {
		return map[string]string{}, nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		resolved, err := r.resolveString(ctx, value)
		if err != nil {
			return nil, err
		}
		result[key] = resolved
	}
	return result, nil
}

func (r *Registry) secretResolver() SecretResolver {
	if r.secrets == nil {
		return EnvSecretResolver{}
	}
	return r.secrets
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
		&tool.SchemaJSON, &executorRef, &tool.TimeoutMS, &acl,
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

var namespaceCleaner = regexp.MustCompile(`[^A-Za-z0-9_]`)

func sanitizeNamespace(value string) string {
	clean := namespaceCleaner.ReplaceAllString(value, "_")
	clean = strings.Trim(clean, "_")
	if clean == "" {
		return "server"
	}
	return clean
}
