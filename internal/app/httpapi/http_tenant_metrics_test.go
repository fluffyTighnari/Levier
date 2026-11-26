// Levier - tenant-aware HTTP metric tests.
// Copyright 2026 The Levier Authors.

package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fluffyTighnari/levier/internal/platform/observe"
)

func TestHTTPMetricsIncludeTenant(t *testing.T) {
	ctx := context.Background()
	apps := newFakeApps()
	app, key, _, err := apps.RegisterApp(ctx, "tenant-app", false)
	if err != nil {
		t.Fatalf("RegisterApp() error = %v", err)
	}
	app.TenantID = "tenant-a"
	apps.apps[app.AppID] = app

	registry := observe.NewRegistry()
	server := NewServer(newFakeStore(), &fakeQueue{}, Config{
		Metrics: registry,
	}, apps)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	body := []byte(fmt.Sprintf(
		`{"app_id":%q,"runtime_name":"echo1","input":[{"role":"user","content":"hello"}]}`,
		app.AppID,
	))
	resp := request(t, http.MethodPost, httpServer.URL+"/api/v1/runs", key, body)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("POST status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
	resp.Body.Close()

	var output bytes.Buffer
	if err := registry.WritePrometheus(&output); err != nil {
		t.Fatalf("WritePrometheus() error = %v", err)
	}
	want := `levier_http_requests_total{method="POST",route="POST /api/v1/runs",status="202",tenant="tenant-a"} 1`
	if !strings.Contains(output.String(), want) {
		t.Fatalf("metrics do not contain %q:\n%s", want, output.String())
	}
}
