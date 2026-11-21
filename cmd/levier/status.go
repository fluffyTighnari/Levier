package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	statuspkg "github.com/fluffyTighnari/levier/internal/status"
)

type statusReport struct {
	Server           string         `json:"server"`
	Health           string         `json:"health"`
	Ready            string         `json:"ready"`
	Metrics          string         `json:"metrics"`
	Daemons          daemonSummary  `json:"daemons"`
	Apps             appSummary     `json:"apps"`
	Runs             map[string]int `json:"runs"`
	MetricsAvailable bool           `json:"metrics_available"`
}

type daemonSummary struct {
	Count    int `json:"count"`
	Capacity int `json:"capacity"`
	Used     int `json:"used"`
}

type appSummary struct {
	WebSockets int `json:"websockets"`
}

var metricLabelPattern = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)="([^"]*)"`)

func runStatus(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	server := flags.String("server", defaultServerValue(), "Levier HTTP address")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 5*time.Second, "per-request timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}

	ctx := context.Background()
	client := &http.Client{Timeout: *timeout}
	base := serverBase(*server)

	if code, body, err := httpGet(ctx, client, base+"/status", ""); err == nil && code == http.StatusOK {
		var snapshot statuspkg.Snapshot
		if err := json.Unmarshal(body, &snapshot); err != nil {
			fmt.Fprintf(stderr, "status failed: invalid /status JSON: %v\n", err)
			return 1
		}
		if snapshot.StatusVersion != statuspkg.Version {
			fmt.Fprintf(stderr, "status failed: unsupported status_version %d (client supports %d)\n",
				snapshot.StatusVersion, statuspkg.Version)
			return 1
		}
		return printStatusOverview(base, snapshot, *asJSON, stdout, stderr)
	}

	report := statusReport{
		Server:  base,
		Health:  "fail",
		Ready:   "fail",
		Metrics: "fail",
		Runs:    map[string]int{},
	}

	if code, _, err := httpGet(ctx, client, base+"/healthz", ""); err == nil && code == http.StatusOK {
		report.Health = "ok"
	}
	if code, _, err := httpGet(ctx, client, base+"/readyz", ""); err == nil && code == http.StatusOK {
		report.Ready = "ok"
	}
	if code, body, err := httpGet(ctx, client, base+"/metrics", ""); err == nil && code == http.StatusOK {
		report.Metrics = "ok"
		report.MetricsAvailable = true
		parseStatusMetrics(string(body), &report)
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "encode status: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintf(stdout, "server      %s\n", report.Server)
		fmt.Fprintf(stdout, "health      %s\n", report.Health)
		fmt.Fprintf(stdout, "ready       %s\n", report.Ready)
		fmt.Fprintf(stdout, "metrics     %s\n", report.Metrics)
		if report.MetricsAvailable {
			fmt.Fprintf(stdout, "daemons     ok %d (capacity %d, used %d)\n", report.Daemons.Count, report.Daemons.Capacity, report.Daemons.Used)
			fmt.Fprintf(stdout, "apps        ok %d websocket\n", report.Apps.WebSockets)
			fmt.Fprintf(stdout, "runs        %s\n", formatRuns(report.Runs))
		}
	}

	if report.Health != "ok" || report.Ready != "ok" || report.Metrics != "ok" {
		return 1
	}
	return 0
}

func parseStatusMetrics(text string, report *statusReport) {
	for _, line := range strings.Split(text, "\n") {
		name, labels, value, ok := parseMetricLine(line)
		if !ok {
			continue
		}
		switch name {
		case "levier_daemon_capacity":
			switch labels["kind"] {
			case "total":
				report.Daemons.Count++
				report.Daemons.Capacity += int(value)
			case "used":
				report.Daemons.Used += int(value)
			}
		case "levier_ws_connections":
			if value > 0 {
				report.Apps.WebSockets++
			}
		case "levier_runs_total":
			status := labels["status"]
			if status == "" {
				status = "unknown"
			}
			report.Runs[status] += int(value)
		}
	}
}

func parseMetricLine(line string) (string, map[string]string, float64, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil, 0, false
	}
	separator := strings.LastIndexByte(line, ' ')
	if separator < 0 {
		return "", nil, 0, false
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(line[separator+1:]), 64)
	if err != nil {
		return "", nil, 0, false
	}

	namePart := strings.TrimSpace(line[:separator])
	name := namePart
	labels := map[string]string{}
	if open := strings.IndexByte(namePart, '{'); open >= 0 && strings.HasSuffix(namePart, "}") {
		name = strings.TrimSpace(namePart[:open])
		for _, match := range metricLabelPattern.FindAllStringSubmatch(namePart[open+1:len(namePart)-1], -1) {
			labels[match[1]] = match[2]
		}
	}
	return name, labels, value, name != ""
}

func formatRuns(runs map[string]int) string {
	if len(runs) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(runs))
	for key := range runs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, runs[key]))
	}
	return strings.Join(parts, " ")
}
