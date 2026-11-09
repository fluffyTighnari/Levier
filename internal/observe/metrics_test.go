// Middleware — internal/observe
//
// Metrics and redaction tests.
//
// Copyright 2026 The Middleware Authors.
package observe

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestRegistryRendersPrometheusText(t *testing.T) {
	registry := NewRegistry()
	registry.Inc(MetricHTTPRequestsTotal, "HTTP requests.", map[string]string{
		"method": "POST", "route": "POST /runs", "status": "202",
	})
	registry.Set(MetricWSConnections, "WebSocket connections.", map[string]string{"app": "app-1"}, 3)
	registry.Observe(MetricToolDuration, "Tool duration.", map[string]string{"tool": "search"}, 0.02)

	var buf bytes.Buffer
	if err := registry.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus() error = %v", err)
	}
	output := buf.String()
	for _, want := range []string{
		"# TYPE mw_http_requests_total counter",
		`mw_http_requests_total{method="POST",route="POST /runs",status="202"} 1`,
		"# TYPE mw_ws_connections gauge",
		`mw_ws_connections{app="app-1"} 3`,
		"# TYPE mw_tool_duration_seconds histogram",
		`mw_tool_duration_seconds_bucket{le="0.025",tool="search"} 1`,
		`mw_tool_duration_seconds_bucket{le="+Inf",tool="search"} 1`,
		`mw_tool_duration_seconds_sum{tool="search"} 0.02`,
		`mw_tool_duration_seconds_count{tool="search"} 1`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output does not contain %q:\n%s", want, output)
		}
	}
}

func TestRegistryEscapesLabelsAndSortsFamilies(t *testing.T) {
	registry := NewRegistry()
	registry.Inc("mw_b_total", "B.", map[string]string{"value": "a\"b\nc"})
	registry.Inc("mw_a_total", "A.", nil)

	var buf bytes.Buffer
	if err := registry.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus() error = %v", err)
	}
	output := buf.String()
	if strings.Index(output, "mw_a_total") > strings.Index(output, "mw_b_total") {
		t.Fatalf("families are not sorted:\n%s", output)
	}
	if !strings.Contains(output, `value="a\"b\nc"`) {
		t.Fatalf("label was not escaped:\n%s", output)
	}
}

func TestRegistryConcurrentUpdates(t *testing.T) {
	registry := NewRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				registry.Inc("mw_test_total", "Test.", map[string]string{"k": "v"})
			}
		}()
	}
	wg.Wait()

	var buf bytes.Buffer
	if err := registry.WritePrometheus(&buf); err != nil {
		t.Fatalf("WritePrometheus() error = %v", err)
	}
	if !strings.Contains(buf.String(), `mw_test_total{k="v"} 1600`) {
		t.Fatalf("counter = %s", buf.String())
	}
}

func TestRedactValue(t *testing.T) {
	tests := []struct {
		key   string
		value string
		want  string
	}{
		{"Authorization", "Bearer abc", Redacted},
		{"api_key", "mw_123", Redacted},
		{"x-api-key", "mw_123", Redacted},
		{"token", "abc", Redacted},
		{"secret_ref", "abc", Redacted},
		{"run_id", "run-1", "run-1"},
		{"", "plain", "plain"},
		{"authorization", "", ""},
	}
	for _, tt := range tests {
		if got := RedactValue(tt.key, tt.value); got != tt.want {
			t.Fatalf("RedactValue(%q, %q) = %q, want %q", tt.key, tt.value, got, tt.want)
		}
	}

	redacted := RedactMap(map[string]string{
		"Authorization": "Bearer abc",
		"run_id":        "run-1",
	})
	if redacted["Authorization"] != Redacted || redacted["run_id"] != "run-1" {
		t.Fatalf("RedactMap = %+v", redacted)
	}
}
