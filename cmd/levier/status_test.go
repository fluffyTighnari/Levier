package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseMetricLine(t *testing.T) {
	name, labels, value, ok := parseMetricLine(`levier_runs_total{status="completed",runtime="normal"} 2`)
	if !ok {
		t.Fatal("parseMetricLine returned !ok")
	}
	if name != "levier_runs_total" || labels["status"] != "completed" || value != 2 {
		t.Fatalf("parsed %q %v %v", name, labels, value)
	}
}

func TestParseStatusMetrics(t *testing.T) {
	report := statusReport{Runs: map[string]int{}}
	parseStatusMetrics(`
# HELP levier_daemon_capacity
levier_daemon_capacity{daemon="d1",kind="total",runtime="NORMAL"} 4
levier_daemon_capacity{daemon="d1",kind="used",runtime="NORMAL"} 1
levier_ws_connections{app="app-1"} 1
levier_runs_total{runtime="normal",status="completed"} 2
levier_runs_total{runtime="normal",status="running"} 1
`, &report)

	if report.Daemons.Count != 1 || report.Daemons.Capacity != 4 || report.Daemons.Used != 1 {
		t.Fatalf("daemons = %+v", report.Daemons)
	}
	if report.Apps.WebSockets != 1 {
		t.Fatalf("apps = %+v", report.Apps)
	}
	if report.Runs["completed"] != 2 || report.Runs["running"] != 1 {
		t.Fatalf("runs = %+v", report.Runs)
	}
}

func TestRunStatusJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/readyz":
			w.WriteHeader(http.StatusOK)
		case "/metrics":
			fmt.Fprint(w, `
levier_daemon_capacity{daemon="d1",kind="total",runtime="NORMAL"} 4
levier_daemon_capacity{daemon="d1",kind="used",runtime="NORMAL"} 0
levier_ws_connections{app="app-1"} 1
levier_runs_total{runtime="normal",status="completed"} 1
`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := runStatus([]string{"--server", server.URL, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runStatus = %d, stderr = %s", code, stderr.String())
	}
	var report statusReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode status: %v (%s)", err, stdout.String())
	}
	if report.Health != "ok" || report.Ready != "ok" || report.Metrics != "ok" {
		t.Fatalf("report = %+v", report)
	}
	if report.Daemons.Capacity != 4 || report.Apps.WebSockets != 1 || report.Runs["completed"] != 1 {
		t.Fatalf("report = %+v", report)
	}
}
