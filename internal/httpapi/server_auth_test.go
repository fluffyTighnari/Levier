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

	"github.com/fluffyTighnari/middleware/internal/id"
	"github.com/fluffyTighnari/middleware/internal/runstore"
)

type fakeApps struct {
	apps   map[string]runstore.App
	byName map[string]string
	keys   map[string]string
}

func newFakeApps() *fakeApps {
	return &fakeApps{
		apps:   make(map[string]runstore.App),
		byName: make(map[string]string),
		keys:   make(map[string]string),
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
		BearerToken: "legacy-token",
		AdminToken:  "admin-token",
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
	if len(queue.tasks) != 1 {
		t.Fatalf("tasks = %d", len(queue.tasks))
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
