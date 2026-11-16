// Middleware — internal/httpapi
//
// App registration and app-key HTTP tests.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fluffyTighnari/levier/internal/id"
	"github.com/fluffyTighnari/levier/internal/register"
	"github.com/fluffyTighnari/levier/internal/runstore"
)

type fakeApps struct {
	authErr error
	apps    map[string]runstore.App
	byName  map[string]string
	keys    map[string]string
	mcp     map[string]register.MCPServerInput
	mcpRefs map[string]int
}

func newFakeApps() *fakeApps {
	return &fakeApps{
		apps:    make(map[string]runstore.App),
		byName:  make(map[string]string),
		keys:    make(map[string]string),
		mcp:     make(map[string]register.MCPServerInput),
		mcpRefs: make(map[string]int),
	}
}

func (f *fakeApps) RegisterApp(_ context.Context, name string, multiagent bool) (runstore.App, string, bool, error) {
	if appID, ok := f.byName[name]; ok {
		return f.apps[appID], "", false, nil
	}
	app := runstore.App{
		AppID:      "app-" + id.New(),
		Name:       name,
		Multiagent: multiagent,
		Status:     "active",
	}
	key := "mw_" + id.New()
	f.apps[app.AppID] = app
	f.byName[name] = app.AppID
	f.keys[key] = app.AppID
	return app, key, true, nil
}

func (f *fakeApps) GetApp(_ context.Context, appID string) (runstore.App, error) {
	app, ok := f.apps[appID]
	if !ok {
		return runstore.App{}, runstore.ErrAppNotFound
	}
	return app, nil
}

func (f *fakeApps) AuthenticateAPIKey(_ context.Context, key string) (runstore.App, error) {
	if f.authErr != nil {
		return runstore.App{}, f.authErr
	}
	appID, ok := f.keys[key]
	if !ok {
		return runstore.App{}, runstore.ErrAppNotFound
	}
	return f.apps[appID], nil
}

func (f *fakeApps) UpdateApp(_ context.Context, appID, name string, multiagent bool) (runstore.App, error) {
	app, ok := f.apps[appID]
	if !ok {
		return runstore.App{}, runstore.ErrAppNotFound
	}
	if other, exists := f.byName[name]; exists && other != appID {
		return runstore.App{}, errors.New("name conflict")
	}
	delete(f.byName, app.Name)
	app.Name = name
	app.Multiagent = multiagent
	f.apps[appID] = app
	f.byName[name] = appID
	return app, nil
}

func (f *fakeApps) RotateAPIKey(_ context.Context, appID string) (string, error) {
	if _, ok := f.apps[appID]; !ok {
		return "", runstore.ErrAppNotFound
	}
	for key, id := range f.keys {
		if id == appID {
			delete(f.keys, key)
		}
	}
	key := "mw_" + id.New()
	f.keys[key] = appID
	return key, nil
}

func newAppTestServer(t *testing.T) (*fakeStore, *fakeQueue, *fakeApps, *httptest.Server) {
	t.Helper()
	store := newFakeStore()
	queue := &fakeQueue{}
	apps := newFakeApps()
	server := NewServer(store, queue, Config{
		BearerToken:       "legacy-token",
		AllowLegacyBearer: true,
		AdminToken:        "admin-token",
	}, apps)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)
	return store, queue, apps, httpServer
}

func TestAdminCreateAppIdempotent(t *testing.T) {
	_, _, _, server := newAppTestServer(t)

	resp := request(t, http.MethodPost, server.URL+"/admin/apps", "admin-token", []byte(`{"name":"app-a","multiagent":true}`))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d", resp.StatusCode)
	}
	var created CreateAppResponse
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	resp.Body.Close()
	if created.App.Name != "app-a" || created.APIKey == "" {
		t.Fatalf("created = %+v", created)
	}

	resp = request(t, http.MethodPost, server.URL+"/admin/apps", "admin-token", []byte(`{"name":"app-a"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("duplicate status = %d", resp.StatusCode)
	}
	var duplicate CreateAppResponse
	if err := json.NewDecoder(resp.Body).Decode(&duplicate); err != nil {
		t.Fatalf("decode duplicate: %v", err)
	}
	resp.Body.Close()
	if duplicate.App.AppID != created.App.AppID || duplicate.APIKey != "" {
		t.Fatalf("duplicate = %+v", duplicate)
	}

	resp = request(t, http.MethodPost, server.URL+"/admin/apps", "wrong", []byte(`{"name":"app-b"}`))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong admin status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRunUsesAuthenticatedApp(t *testing.T) {
	store, queue, apps, server := newAppTestServer(t)
	app, key, _, _ := apps.RegisterApp(context.Background(), "app-run", false)

	body := []byte(`{
		"runtime":"normal",
		"strategy":{"name":"echo"},
		"input":[{"role":"user","content":"hello"}],
		"model":{"provider":"anthropic","model":"claude-test"}
	}`)
	resp := request(t, http.MethodPost, server.URL+"/api/v1/runs", key, body)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("create run status = %d", resp.StatusCode)
	}
	var created CreateRunResponse
	_ = json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if store.runs[created.RunID].AppID != app.AppID {
		t.Fatalf("run app = %q, want %q", store.runs[created.RunID].AppID, app.AppID)
	}
	if len(queue.tasks) != 2 {
		t.Fatalf("tasks = %d, want 2", len(queue.tasks))
	}

	resp = request(t, http.MethodGet, server.URL+"/api/v1/runs/"+created.RunID, key, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("own run status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	other, otherKey, _, _ := apps.RegisterApp(context.Background(), "app-other", false)
	_ = other
	resp = request(t, http.MethodGet, server.URL+"/api/v1/runs/"+created.RunID, otherKey, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("other run status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodPost, server.URL+"/api/v1/runs", key, []byte(`{
		"app_id":"`+other.AppID+`",
		"runtime":"normal",
		"strategy":{"name":"echo"},
		"input":[{"role":"user","content":"hello"}],
		"model":{"provider":"anthropic","model":"claude-test"}
	}`))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("mismatched app status = %d, want 403", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestUpdateAndRotateAppHTTP(t *testing.T) {
	_, _, apps, server := newAppTestServer(t)
	app, key, _, _ := apps.RegisterApp(context.Background(), "before", false)

	resp := request(t, http.MethodPost, server.URL+"/api/v1/apps", key, []byte(`{"name":"after","multiagent":true}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodGet, server.URL+"/api/v1/apps/"+app.AppID, key, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get app status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodPost, server.URL+"/admin/apps/"+app.AppID+"/rotate-key", "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotate status = %d", resp.StatusCode)
	}
	var rotated map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&rotated)
	resp.Body.Close()
	if rotated["api_key"] == "" || rotated["api_key"] == key {
		t.Fatalf("rotated key = %q", rotated["api_key"])
	}

	resp = request(t, http.MethodGet, server.URL+"/api/v1/apps/"+app.AppID, key, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old key status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
	resp = request(t, http.MethodGet, server.URL+"/api/v1/apps/"+app.AppID, rotated["api_key"], nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new key status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestLegacyBearerDisabledByDefault(t *testing.T) {
	store := newFakeStore()
	apps := newFakeApps()
	app, key, _, _ := apps.RegisterApp(context.Background(), "app-secure", false)
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "legacy-token",
		AdminToken:        "admin-token",
		AllowLegacyBearer: false,
	}, apps)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	body := []byte(`{
		"app_id":"` + app.AppID + `",
		"runtime":"normal",
		"strategy":{"name":"echo"},
		"input":[{"role":"user","content":"hello"}],
		"model":{"provider":"anthropic","model":"claude-test"}
	}`)
	resp := request(t, http.MethodPost, httpServer.URL+"/api/v1/runs", "legacy-token", body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("legacy bearer status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	body = []byte(`{
		"runtime":"normal",
		"strategy":{"name":"echo"},
		"input":[{"role":"user","content":"hello"}],
		"model":{"provider":"anthropic","model":"claude-test"}
	}`)
	resp = request(t, http.MethodPost, httpServer.URL+"/api/v1/runs", key, body)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("app key status = %d, want 202", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestAdminTokenHasNoBearerFallback(t *testing.T) {
	store := newFakeStore()
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken: "legacy-token",
		AdminToken:  "",
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	resp := request(t, http.MethodPost, httpServer.URL+"/admin/apps", "legacy-token", []byte(`{"name":"app-x"}`))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("legacy admin status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestCrossAppRunEventAndCancelDenied(t *testing.T) {
	store, _, apps, server := newAppTestServer(t)
	owner, ownerKey, _, _ := apps.RegisterApp(context.Background(), "app-owner", false)
	_, otherKey, _, _ := apps.RegisterApp(context.Background(), "app-other", false)
	store.runs["run-1"] = runstore.Run{RunID: "run-1", AppID: owner.AppID, Status: "running"}

	resp := request(t, http.MethodGet, server.URL+"/api/v1/runs/run-1/events", otherKey, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign events status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodPost, server.URL+"/api/v1/runs/run-1/cancel", otherKey, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign cancel status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodGet, server.URL+"/api/v1/runs/run-1/events", ownerKey, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner events status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodGet, server.URL+"/api/v1/runs/run-1", "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin run status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestRunAuthBackendUnavailable(t *testing.T) {
	_, _, apps, server := newAppTestServer(t)
	apps.authErr = errors.New("connection refused")

	resp := request(t, http.MethodPost, server.URL+"/api/v1/runs", "any-key", []byte(validBody))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	var body ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "DEPENDENCY_UNAVAILABLE" {
		t.Fatalf("code = %q", body.Error.Code)
	}
}

func (f *fakeApps) UpsertMCPServer(_ context.Context, input register.MCPServerInput) error {
	f.mcp[input.Name] = input
	return nil
}

func (f *fakeApps) ListMCPServers(_ context.Context) ([]register.MCPServerInfo, error) {
	servers := make([]register.MCPServerInfo, 0, len(f.mcp))
	for name, input := range f.mcp {
		info := register.MCPServerInfo{Name: name, Transport: input.Transport}
		if input.AllowedToolsJSON != "" {
			_ = json.Unmarshal([]byte(input.AllowedToolsJSON), &info.AllowedTools)
		}
		servers = append(servers, info)
	}
	return servers, nil
}

func (f *fakeApps) DeleteMCPServer(_ context.Context, name string, force bool) error {
	if _, ok := f.mcp[name]; !ok {
		return register.ErrMCPServerNotFound
	}
	if f.mcpRefs[name] > 0 && !force {
		return register.ErrMCPServerInUse
	}
	delete(f.mcp, name)
	return nil
}

func TestAdminMCPCatalogLifecycle(t *testing.T) {
	_, _, apps, server := newAppTestServer(t)

	resp := request(t, http.MethodPost, server.URL+"/admin/mcp", "admin-token", []byte(`{
		"name":"fake","transport":"stdio",
		"config":{"command":"python","env":{"TOKEN":"${secret:fake_token}"}},
		"allowed_tools":["add"]
	}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upsert status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodGet, server.URL+"/admin/mcp", "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d", resp.StatusCode)
	}
	var listed MCPListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	resp.Body.Close()
	if len(listed.Servers) != 1 || listed.Servers[0].Name != "fake" {
		t.Fatalf("servers = %+v", listed.Servers)
	}
	if len(listed.Servers[0].AllowedTools) != 1 || listed.Servers[0].AllowedTools[0] != "add" {
		t.Fatalf("allowed = %+v", listed.Servers[0].AllowedTools)
	}
	for _, server := range listed.Servers {
		if server.Name == "fake" && len(server.SecretRefs) != 0 {
			t.Fatalf("fake registry should not report refs, got %v", server.SecretRefs)
		}
	}

	apps.mcpRefs["fake"] = 1
	resp = request(t, http.MethodDelete, server.URL+"/admin/mcp/fake", "admin-token", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("delete in-use status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = request(t, http.MethodDelete, server.URL+"/admin/mcp/fake?force=true", "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("force delete status = %d", resp.StatusCode)
	}
	resp.Body.Close()
	if len(apps.mcp) != 0 {
		t.Fatalf("catalog not empty: %+v", apps.mcp)
	}
}

func TestAdminMCPValidation(t *testing.T) {
	_, _, _, server := newAppTestServer(t)
	cases := []struct {
		name string
		body string
	}{
		{"missing name", `{"transport":"stdio","config":{"command":"x"}}`},
		{"bad transport", `{"name":"x","transport":"tcp","config":{"command":"x"}}`},
		{"stdio without command", `{"name":"x","transport":"stdio","config":{}}`},
		{"http without url", `{"name":"x","transport":"sse","config":{}}`},
		{"array config", `{"name":"x","transport":"stdio","config":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := request(t, http.MethodPost, server.URL+"/admin/mcp", "admin-token", []byte(tc.body))
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d", resp.StatusCode)
			}
		})
	}

	resp := request(t, http.MethodPost, server.URL+"/admin/mcp", "wrong", []byte(`{}`))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("non-admin status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}
