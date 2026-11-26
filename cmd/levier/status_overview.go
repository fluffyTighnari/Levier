// Levier — human and JSON status overview output.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"

	statuspkg "github.com/fluffyTighnari/levier/internal/ops/status"
)

func printStatusOverview(server string, snapshot statuspkg.Snapshot, asJSON bool, stdout, stderr io.Writer) int {
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(snapshot); err != nil {
			fmt.Fprintf(stderr, "encode status: %v\n", err)
			return 1
		}
		if snapshot.Health.Readiness != "ok" {
			return 1
		}
		return 0
	}

	readiness := snapshot.Health.Readiness
	if readiness == "" {
		readiness = "unknown"
	}
	fmt.Fprintf(stdout, "server      %s\n", server)
	fmt.Fprintf(stdout, "status      %s\n", readiness)
	fmt.Fprintf(stdout, "version     %s (%s)\n", snapshot.Build.Version, snapshot.Build.Commit)
	fmt.Fprintf(stdout, "uptime      %s\n", formatUptime(snapshot.UptimeSeconds))
	fmt.Fprintf(stdout, "health      %s\n", snapshot.Health.Liveness)
	fmt.Fprintf(stdout, "ready       %s\n", readiness)
	fmt.Fprintf(stdout, "database    %s\n", snapshot.Health.Database)
	fmt.Fprintf(stdout, "daemons     connected=%d healthy=%d capacity=%d used=%d available=%d\n",
		snapshot.Daemons.Connected,
		snapshot.Daemons.Healthy,
		snapshot.Daemons.Capacity,
		snapshot.Daemons.Used,
		snapshot.Daemons.Available,
	)
	fmt.Fprintf(stdout, "apps        registered=%d websocket=%d\n",
		snapshot.Apps.Registered,
		snapshot.Apps.WebSocketConnections,
	)
	fmt.Fprintf(stdout, "runs        total=%d active=%d %s\n",
		snapshot.Runs.Total,
		snapshot.Runs.Active,
		formatInt64Map(snapshot.Runs.ByStatus),
	)
	fmt.Fprintf(stdout, "workflow    total=%d pending=%d leased=%d dead=%d oldest_pending=%ds\n",
		snapshot.Workflow.Total,
		snapshot.Workflow.Pending,
		snapshot.Workflow.Leased,
		snapshot.Workflow.Dead,
		snapshot.Workflow.OldestPendingSeconds,
	)
	if readiness != "ok" {
		return 1
	}
	return 0
}

func formatUptime(seconds int64) string {
	if seconds < 60 {
		return strconv.FormatInt(seconds, 10) + "s"
	}
	minutes := seconds / 60
	if minutes < 60 {
		return strconv.FormatInt(minutes, 10) + "m" + strconv.FormatInt(seconds%60, 10) + "s"
	}
	hours := minutes / 60
	return fmt.Sprintf("%dh%dm", hours, minutes%60)
}

func formatInt64Map(values map[string]int64) string {
	if len(values) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, values[key]))
	}
	return joinStrings(parts, " ")
}

func joinStrings(parts []string, separator string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, part := range parts[1:] {
		result += separator + part
	}
	return result
}
