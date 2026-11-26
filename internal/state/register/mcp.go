// Levier — register
//
// Split from materials.go (P8R.3).
// Copyright 2026 The Levier Authors.
package register

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fluffyTighnari/levier/internal/platform/id"
)

// MCPServerInput is one registered MCP server definition.
type MCPServerInput struct {
	Name             string `json:"name"`
	Transport        string `json:"transport"`
	ConfigJSON       string `json:"config_json"`
	AllowedToolsJSON string `json:"allowed_tools_json"`
}

// MCPServerInfo is the admin-visible catalog view. It never includes config
// values, only the secret names referenced by placeholders.
type MCPServerInfo struct {
	Name         string    `json:"name"`
	Transport    string    `json:"transport"`
	AllowedTools []string  `json:"allowed_tools"`
	SecretRefs   []string  `json:"secret_refs"`
	UpdatedAt    time.Time `json:"updated_at"`
}

var (
	// ErrMCPServerNotFound reports a missing catalog entry.
	ErrMCPServerNotFound = errors.New("register: MCP server not found")
	// ErrMCPServerInUse reports an entry still referenced by an App.
	ErrMCPServerInUse = errors.New("register: MCP server is referenced by an app")
)

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

// ListMCPServers returns the admin catalog view without config values.
func (r *Registry) ListMCPServers(ctx context.Context) ([]MCPServerInfo, error) {
	if r == nil || r.pool == nil {
		return nil, fmt.Errorf("register: postgres pool is nil")
	}
	rows, err := r.pool.Query(ctx, `
		SELECT name, transport, config_json, COALESCE(allowed_tools_json, ''), updated_at
		FROM mcp_server_defs
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("register: list MCP servers: %w", err)
	}
	defer rows.Close()
	servers := make([]MCPServerInfo, 0)
	for rows.Next() {
		var info MCPServerInfo
		var configJSON, allowedJSON string
		if err := rows.Scan(&info.Name, &info.Transport, &configJSON, &allowedJSON, &info.UpdatedAt); err != nil {
			return nil, fmt.Errorf("register: scan MCP server: %w", err)
		}
		if allowedJSON != "" {
			if err := json.Unmarshal([]byte(allowedJSON), &info.AllowedTools); err != nil {
				return nil, fmt.Errorf("register: parse MCP %q allowed tools: %w", info.Name, err)
			}
		}
		info.SecretRefs = extractSecretRefs(configJSON)
		servers = append(servers, info)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("register: iterate MCP servers: %w", err)
	}
	return servers, nil
}

// DeleteMCPServer removes one catalog entry. Referenced entries are rejected
// unless force is set, which also drops the App references.
func (r *Registry) DeleteMCPServer(ctx context.Context, name string, force bool) error {
	if r == nil || r.pool == nil {
		return fmt.Errorf("register: postgres pool is nil")
	}
	if name == "" {
		return fmt.Errorf("register: MCP name is required")
	}
	var mcpID string
	if err := r.pool.QueryRow(ctx,
		"SELECT mcp_id FROM mcp_server_defs WHERE name = $1", name,
	).Scan(&mcpID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMCPServerNotFound
		}
		return fmt.Errorf("register: lookup MCP server: %w", err)
	}
	if !force {
		var refs int
		if err := r.pool.QueryRow(ctx,
			"SELECT count(*) FROM app_mcp_refs WHERE mcp_id = $1", mcpID,
		).Scan(&refs); err != nil {
			return fmt.Errorf("register: count MCP refs: %w", err)
		}
		if refs > 0 {
			return ErrMCPServerInUse
		}
	}
	if _, err := r.pool.Exec(ctx, "DELETE FROM mcp_server_defs WHERE mcp_id = $1", mcpID); err != nil {
		return fmt.Errorf("register: delete MCP server: %w", err)
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
	// Secret placeholders (${secret:name}) are resolved by mcp_runtime, not by
	// the control plane, so TaskAssign never carries plaintext credentials.
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

var secretRefPattern = regexp.MustCompile(`\$\{secret:([^}]+)\}`)

// extractSecretRefs lists the secret names referenced by a config document.
func extractSecretRefs(configJSON string) []string {
	var value any
	if err := json.Unmarshal([]byte(configJSON), &value); err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	var walk func(any)
	walk = func(node any) {
		switch typed := node.(type) {
		case string:
			for _, match := range secretRefPattern.FindAllStringSubmatch(typed, -1) {
				seen[match[1]] = struct{}{}
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		case map[string]any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	refs := make([]string, 0, len(seen))
	for name := range seen {
		refs = append(refs, name)
	}
	sort.Strings(refs)
	return refs
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
