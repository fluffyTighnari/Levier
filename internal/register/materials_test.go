// Middleware — internal/register
//
// Tool/MCP material assembly tests.
//
// Copyright 2026 The Middleware Authors.
package register

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestToolShadowingACLAndAssembly(t *testing.T) {
	registry := New(testPool(t))
	ctx := context.Background()
	app, _, _, err := registry.RegisterApp(ctx, "tool-app", false)
	if err != nil {
		t.Fatalf("RegisterApp() error = %v", err)
	}
	other, _, _, err := registry.RegisterApp(ctx, "other-app", false)
	if err != nil {
		t.Fatalf("RegisterApp(other) error = %v", err)
	}

	if _, err := registry.ReplaceLocalTools(ctx, app.AppID, []ToolInput{
		{Name: "search", SchemaJSON: `{"type":"object"}`, Description: "local search"},
		{Name: "local_only", SchemaJSON: `{"type":"object"}`},
	}); err != nil {
		t.Fatalf("ReplaceLocalTools() error = %v", err)
	}
	if _, err := registry.UpsertGlobalTool(ctx, GlobalToolInput{
		Name: "search", SchemaJSON: `{"type":"object"}`, ExecutorRef: "builtin:search",
	}); err != nil {
		t.Fatalf("UpsertGlobalTool(search) error = %v", err)
	}
	if _, err := registry.UpsertGlobalTool(ctx, GlobalToolInput{
		Name: "fetch", SchemaJSON: `{"type":"object"}`, ExecutorRef: "builtin:http",
	}); err != nil {
		t.Fatalf("UpsertGlobalTool(fetch) error = %v", err)
	}
	if _, err := registry.UpsertGlobalTool(ctx, GlobalToolInput{
		Name: "private", SchemaJSON: `{"type":"object"}`, ExecutorRef: "builtin:http",
		ACL: []string{other.AppID},
	}); err != nil {
		t.Fatalf("UpsertGlobalTool(private) error = %v", err)
	}

	resolved, ok, err := registry.ResolveTool(ctx, app.AppID, "search")
	if err != nil || !ok || resolved.Scope != "local" {
		t.Fatalf("ResolveTool(search) = %+v, %v, %v; want local", resolved, ok, err)
	}
	resolved, ok, err = registry.ResolveTool(ctx, app.AppID, "fetch")
	if err != nil || !ok || resolved.Scope != "global" {
		t.Fatalf("ResolveTool(fetch) = %+v, %v, %v; want global", resolved, ok, err)
	}
	if _, ok, err := registry.ResolveTool(ctx, app.AppID, "private"); err != nil || ok {
		t.Fatalf("ResolveTool(private) = %v, %v; want not found", ok, err)
	}
	if _, ok, err := registry.ResolveTool(ctx, other.AppID, "private"); err != nil || !ok {
		t.Fatalf("ResolveTool(private other) = %v, %v; want found", ok, err)
	}

	materials, err := registry.AssembleTaskMaterials(ctx, app.AppID)
	if err != nil {
		t.Fatalf("AssembleTaskMaterials() error = %v", err)
	}
	names := make([]string, 0, len(materials.Tools))
	for _, tool := range materials.Tools {
		names = append(names, tool.Name)
	}
	if len(names) != 3 || names[0] != "local_only" || names[1] != "search" || names[2] != "fetch" {
		t.Fatalf("assembled tools = %v", names)
	}
	for _, tool := range materials.Tools {
		if tool.Name == "search" && tool.Scope != "local" {
			t.Fatalf("search was not shadowed by local tool: %+v", tool)
		}
	}
}

func TestMCPBindingAssemblyKeepsPlaceholders(t *testing.T) {
	registry := New(testPool(t))
	ctx := context.Background()
	app, _, _, err := registry.RegisterApp(ctx, "mcp-app", false)
	if err != nil {
		t.Fatalf("RegisterApp() error = %v", err)
	}
	if err := registry.UpsertMCPServer(ctx, MCPServerInput{
		Name:      "github",
		Transport: "stdio",
		ConfigJSON: `{
			"command":"mcp-github",
			"args":["--stdio"],
			"env":{"TOKEN":"${secret:github_token}"}
		}`,
		AllowedToolsJSON: `["search","get_issue"]`,
	}); err != nil {
		t.Fatalf("UpsertMCPServer() error = %v", err)
	}
	if err := registry.SetAppMCPRefs(ctx, app.AppID, []string{"github"}); err != nil {
		t.Fatalf("SetAppMCPRefs() error = %v", err)
	}

	materials, err := registry.AssembleTaskMaterials(ctx, app.AppID)
	if err != nil {
		t.Fatalf("AssembleTaskMaterials() error = %v", err)
	}
	if len(materials.MCPBindings) != 1 {
		t.Fatalf("MCP bindings = %+v", materials.MCPBindings)
	}
	binding := materials.MCPBindings[0]
	if binding.ServerName != "github" || binding.Transport != "stdio" {
		t.Fatalf("binding = %+v", binding)
	}
	if binding.Command != "mcp-github" || len(binding.Args) != 1 {
		t.Fatalf("command = %q args=%v", binding.Command, binding.Args)
	}
	if binding.Env["TOKEN"] != "${secret:github_token}" {
		t.Fatalf("control plane must not resolve MCP secrets: %+v", binding.Env)
	}
	if len(binding.AllowedTools) != 2 || binding.Namespace != "mcp__github__" {
		t.Fatalf("allowed/namespace = %+v", binding)
	}

	if err := registry.SetAppMCPRefs(ctx, app.AppID, []string{"missing"}); err == nil {
		t.Fatal("SetAppMCPRefs(missing) error = nil")
	}
}

func TestListAndDeleteMCPServers(t *testing.T) {
	registry := New(testPool(t))
	ctx := context.Background()
	app, _, _, _ := registry.RegisterApp(ctx, "mcp-catalog-app", false)
	if err := registry.UpsertMCPServer(ctx, MCPServerInput{
		Name: "github", Transport: "stdio",
		ConfigJSON:       `{"command":"mcp-github","env":{"TOKEN":"${secret:github_token}"}}`,
		AllowedToolsJSON: `["search"]`,
	}); err != nil {
		t.Fatalf("UpsertMCPServer(github) error = %v", err)
	}
	if err := registry.UpsertMCPServer(ctx, MCPServerInput{
		Name: "filesystem", Transport: "stdio",
		ConfigJSON: `{"command":"mcp-fs"}`,
	}); err != nil {
		t.Fatalf("UpsertMCPServer(filesystem) error = %v", err)
	}
	if err := registry.SetAppMCPRefs(ctx, app.AppID, []string{"github"}); err != nil {
		t.Fatalf("SetAppMCPRefs() error = %v", err)
	}

	servers, err := registry.ListMCPServers(ctx)
	if err != nil {
		t.Fatalf("ListMCPServers() error = %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("servers = %+v", servers)
	}
	var github MCPServerInfo
	for _, server := range servers {
		if server.Name == "github" {
			github = server
		}
	}
	if len(github.SecretRefs) != 1 || github.SecretRefs[0] != "github_token" {
		t.Fatalf("secret refs = %v", github.SecretRefs)
	}
	if len(github.AllowedTools) != 1 || github.AllowedTools[0] != "search" {
		t.Fatalf("allowed tools = %v", github.AllowedTools)
	}

	if err := registry.DeleteMCPServer(ctx, "github", false); !errors.Is(err, ErrMCPServerInUse) {
		t.Fatalf("DeleteMCPServer(in use) error = %v", err)
	}
	if err := registry.DeleteMCPServer(ctx, "filesystem", false); err != nil {
		t.Fatalf("DeleteMCPServer(filesystem) error = %v", err)
	}
	if err := registry.DeleteMCPServer(ctx, "github", true); err != nil {
		t.Fatalf("DeleteMCPServer(force) error = %v", err)
	}
	if err := registry.DeleteMCPServer(ctx, "github", false); !errors.Is(err, ErrMCPServerNotFound) {
		t.Fatalf("DeleteMCPServer(missing) error = %v", err)
	}
	materials, err := registry.AssembleTaskMaterials(ctx, app.AppID)
	if err != nil {
		t.Fatalf("AssembleTaskMaterials() error = %v", err)
	}
	if len(materials.MCPBindings) != 0 {
		t.Fatalf("forced delete left bindings: %+v", materials.MCPBindings)
	}
}

func TestReplaceLocalToolsRemovesStaleTools(t *testing.T) {
	registry := New(testPool(t))
	ctx := context.Background()
	app, _, _, _ := registry.RegisterApp(ctx, "replace-app", false)
	if _, err := registry.ReplaceLocalTools(ctx, app.AppID, []ToolInput{
		{Name: "old", SchemaJSON: `{"type":"object"}`},
	}); err != nil {
		t.Fatalf("first ReplaceLocalTools() error = %v", err)
	}
	if _, err := registry.ReplaceLocalTools(ctx, app.AppID, []ToolInput{
		{Name: "new", SchemaJSON: `{"type":"object"}`},
	}); err != nil {
		t.Fatalf("second ReplaceLocalTools() error = %v", err)
	}
	materials, err := registry.AssembleTaskMaterials(ctx, app.AppID)
	if err != nil {
		t.Fatalf("AssembleTaskMaterials() error = %v", err)
	}
	if len(materials.Tools) != 1 || materials.Tools[0].Name != "new" {
		t.Fatalf("tools = %+v", materials.Tools)
	}
}

func TestFileSecretResolver(t *testing.T) {
	path := t.TempDir() + "/secrets.json"
	if err := os.WriteFile(path, []byte(`{"token":"file-secret"}`), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}
	resolver, err := LoadFileSecretResolver(path)
	if err != nil {
		t.Fatalf("LoadFileSecretResolver() error = %v", err)
	}
	value, err := resolver.Resolve(context.Background(), "token")
	if err != nil || value != "file-secret" {
		t.Fatalf("Resolve() = %q, %v", value, err)
	}
}

func TestGlobalToolConfigPersistsAndResolvesSecrets(t *testing.T) {
	pool := testPool(t)
	registry := NewWithSecrets(pool, FileSecretResolver{Values: map[string]string{"token": "abc123"}})
	ctx := context.Background()

	config := `{"url":"https://example.com","headers":{"Authorization":"Bearer ${secret:token}"}}`
	if _, err := registry.UpsertGlobalTool(ctx, GlobalToolInput{
		Name:        "fetch",
		SchemaJSON:  `{"type":"object"}`,
		ExecutorRef: "builtin:http",
		ConfigJSON:  config,
	}); err != nil {
		t.Fatalf("UpsertGlobalTool() error = %v", err)
	}

	tool, resolution, err := registry.ResolveToolDecision(ctx, "app-x", "fetch")
	if err != nil || resolution != ResolutionFound {
		t.Fatalf("ResolveToolDecision() = %+v, %q, %v", tool, resolution, err)
	}
	if !strings.Contains(tool.ConfigJSON, "Bearer abc123") {
		t.Fatalf("config was not resolved: %q", tool.ConfigJSON)
	}

	var stored string
	if err := pool.QueryRow(ctx, "SELECT config_json FROM tool_defs WHERE name = 'fetch'").Scan(&stored); err != nil {
		t.Fatalf("read stored config: %v", err)
	}
	if stored != config {
		t.Fatalf("stored config = %q, want placeholder form", stored)
	}
}
