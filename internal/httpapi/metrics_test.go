// Middleware — internal/httpapi
//
// HTTP metrics and tracing tests.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fluffyTighnari/levier/internal/observe"
)

func TestMetricsEndpointExposesHTTPMetrics(t *testing.T) {
	store := newFakeStore()
	registry := observe.NewRegistry()
	collected := false
	server := NewServer(store, &fakeQueue{}, Config{
		BearerToken:       "token-1",
		AllowLegacyBearer: true,
		Metrics:           registry,
		MetricsCollector: func(context.Context) {
			collected = true
			registry.Set(observe.MetricWorkflowTasks, "Tasks.", map[string]string{
				"type": "RUN_DISPATCH", "status": "PENDING",
			}, 2)
		},
	})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	resp := request(t, http.MethodGet, httpServer.URL+"/healthz", "", nil)
	resp.Body.Close()

	resp, err := http.Get(httpServer.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	output := string(payload)
	for _, want := range []string{
		`levier_http_requests_total{method="GET",route="GET /healthz",status="200"} 1`,
		`levier_workflow_tasks{status="PENDING",type="RUN_DISPATCH"} 2`,
		"# TYPE levier_http_request_duration_seconds histogram",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("metrics do not contain %q:\n%s", want, output)
		}
	}
	if !collected {
		t.Fatal("metrics collector was not called")
	}
	if strings.Contains(output, `route="GET /metrics"`) {
		t.Fatal("/metrics scraped itself")
	}
}

func TestTraceIDUsesRequestID(t *testing.T) {
	store := newFakeStore()
	server := NewServer(store, &fakeQueue{}, Config{BearerToken: "token-1", AllowLegacyBearer: true})
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	req, _ := http.NewRequest(http.MethodPost, httpServer.URL+"/api/v1/runs", bytes.NewReader([]byte(validBody)))
	req.Header.Set("Authorization", "Bearer token-1")
	req.Header.Set(RequestIDHeader, "trace-from-header")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get(RequestIDHeader); got != "trace-from-header" {
		t.Fatalf("request id = %q", got)
	}
	for _, run := range store.runs {
		if run.TraceID != "trace-from-header" {
			t.Fatalf("run trace id = %q", run.TraceID)
		}
	}
}
