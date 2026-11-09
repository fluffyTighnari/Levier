// Middleware — internal/register
//
// Tool/MCP material assembly tests.
//
// Copyright 2026 The Middleware Authors.
package register

import (
	"context"
	"os"
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

func TestMCPBindingAssemblyAndSecrets(t *testing.T) {
	t.Setenv("TEST_SECRET_GITHUB_TOKEN", "secret-token")
	registry := NewWithSecrets(testPool(t), EnvSecretResolver{Prefix: "TEST_SECRET_"})
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
	if binding.Env["TOKEN"] != "secret-token" {
		t.Fatalf("env = %+v", binding.Env)
	}
	if len(binding.AllowedTools) != 2 || binding.Namespace != "mcp__github__" {
		t.Fatalf("allowed/namespace = %+v", binding)
	}
	if binding.Env["TOKEN"] == "${secret:github_token}" {
		t.Fatal("secret placeholder was not resolved")
	}

	if err := registry.SetAppMCPRefs(ctx, app.AppID, []string{"missing"}); err == nil {
		t.Fatal("SetAppMCPRefs(missing) error = nil")
	}
}

func TestMCPMissingSecretFailsAssembly(t *testing.T) {
	registry := NewWithSecrets(testPool(t), FileSecretResolver{Values: map[string]string{}})
	ctx := context.Background()
	app, _, _, _ := registry.RegisterApp(ctx, "mcp-secret-app", false)
	if err := registry.UpsertMCPServer(ctx, MCPServerInput{
		Name: "github", Transport: "sse",
		ConfigJSON: `{"url":"https://example.test/sse","headers":{"Authorization":"Bearer ${secret:missing}"}}`,
	}); err != nil {
		t.Fatalf("UpsertMCPServer() error = %v", err)
	}
	if err := registry.SetAppMCPRefs(ctx, app.AppID, []string{"github"}); err != nil {
		t.Fatalf("SetAppMCPRefs() error = %v", err)
	}
	if _, err := registry.AssembleTaskMaterials(ctx, app.AppID); err == nil {
		t.Fatal("AssembleTaskMaterials() error = nil, want missing secret")
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
