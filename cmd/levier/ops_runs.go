package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

func runOpsRuns(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printOpsRunsUsage(stderr)
		return 2
	}
	switch args[0] {
	case "list":
		return runOpsRunsList(args[1:], stdout, stderr)
	case "show":
		return runOpsRunsShow(args[1:], stdout, stderr)
	case "cancel":
		return runOpsRunsCancel(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printOpsRunsUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown ops runs command: %s\n\n", args[0])
		printOpsRunsUsage(stderr)
		return 2
	}
}

func printOpsRunsUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier ops runs list [--status queued] [--app-id id] [--daemon-id id] [--session-id id] [--limit 50] [--cursor value] [--json]")
	fmt.Fprintln(out, "  levier ops runs show --run-id id [--json]")
	fmt.Fprintln(out, "  levier ops runs cancel --run-id id [--reason text] --yes [--json]")
}

func runOpsRunsList(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("ops runs list", flag.ContinueOnError)
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	status := flags.String("status", "", "comma-separated run status")
	appID := flags.String("app-id", "", "filter by app id")
	daemonID := flags.String("daemon-id", "", "filter by assigned daemon id")
	sessionID := flags.String("session-id", "", "filter by session id")
	cursor := flags.String("cursor", "", "pagination cursor")
	limit := flags.Int("limit", 50, "page size")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	ok, code := parseOpsFlagSet("ops runs list", flags, args, stdout, stderr)
	if !ok {
		return code
	}
	client, ok := opsAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	query := url.Values{}
	if value := strings.TrimSpace(*status); value != "" {
		query.Set("status", value)
	}
	if value := strings.TrimSpace(*appID); value != "" {
		query.Set("app_id", value)
	}
	if value := strings.TrimSpace(*daemonID); value != "" {
		query.Set("daemon_id", value)
	}
	if value := strings.TrimSpace(*sessionID); value != "" {
		query.Set("session_id", value)
	}
	if value := strings.TrimSpace(*cursor); value != "" {
		query.Set("cursor", value)
	}
	query.Set("limit", strconv.Itoa(*limit))

	page, err := client.listAdminRuns(context.Background(), query)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, page, stderr)
	}
	if len(page.Runs) == 0 {
		fmt.Fprintln(stdout, "no runs")
		return 0
	}
	fmt.Fprintf(stdout, "%-26s %-10s %-24s %-24s %-8s %s\n", "run_id", "status", "app_id", "daemon_id", "attempt", "updated_at")
	for _, run := range page.Runs {
		fmt.Fprintf(stdout, "%-26s %-10s %-24s %-24s %-8d %s\n",
			run.RunID, run.Status, run.AppID, run.AssignedDaemonID, run.Attempt, run.UpdatedAt)
	}
	if page.NextCursor != "" {
		fmt.Fprintf(stdout, "next_cursor: %s\n", page.NextCursor)
	}
	return 0
}

func runOpsRunsShow(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("ops runs show", flag.ContinueOnError)
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	runID := flags.String("run-id", "", "run id")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	ok, code := parseOpsFlagSet("ops runs show", flags, args, stdout, stderr)
	if !ok {
		return code
	}
	if *runID == "" {
		fmt.Fprintln(stderr, "run id is required: --run-id")
		return 2
	}
	client, ok := opsAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	run, err := client.getAdminRun(context.Background(), *runID)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, run, stderr)
	}
	fmt.Fprintf(stdout, "run_id              %s\n", run.RunID)
	fmt.Fprintf(stdout, "app_id              %s\n", run.AppID)
	fmt.Fprintf(stdout, "status              %s\n", run.Status)
	fmt.Fprintf(stdout, "runtime_name        %s\n", run.RuntimeName)
	fmt.Fprintf(stdout, "attempt             %d\n", run.Attempt)
	fmt.Fprintf(stdout, "daemon              %s\n", run.AssignedDaemonID)
	fmt.Fprintf(stdout, "session_id          %s\n", run.SessionID)
	fmt.Fprintf(stdout, "trace_id            %s\n", run.TraceID)
	fmt.Fprintf(stdout, "last_event          attempt=%d seq=%d\n", run.LastEventAttempt, run.LastEventSeq)
	fmt.Fprintf(stdout, "created_at          %s\n", run.CreatedAt)
	fmt.Fprintf(stdout, "updated_at          %s\n", run.UpdatedAt)
	if len(run.Tasks) > 0 {
		fmt.Fprintln(stdout, "tasks:")
		tasks := append([]adminTaskView(nil), run.Tasks...)
		sort.Slice(tasks, func(i, j int) bool {
			if tasks[i].UpdatedAt == tasks[j].UpdatedAt {
				return tasks[i].TaskID < tasks[j].TaskID
			}
			return tasks[i].UpdatedAt > tasks[j].UpdatedAt
		})
		for _, task := range tasks {
			fmt.Fprintf(stdout, "  %s %s %s attempt=%d/%d error=%s\n",
				task.TaskID, task.TaskType, task.Status, task.Attempt, task.MaxAttempts, task.LastError)
		}
	}
	return 0
}

func runOpsRunsCancel(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("ops runs cancel", flag.ContinueOnError)
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	runID := flags.String("run-id", "", "run id")
	reason := flags.String("reason", "operator", "cancellation reason")
	yes := flags.Bool("yes", false, "confirm cancellation")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	ok, code := parseOpsFlagSet("ops runs cancel", flags, args, stdout, stderr)
	if !ok {
		return code
	}
	if *runID == "" {
		fmt.Fprintln(stderr, "run id is required: --run-id")
		return 2
	}
	if !*yes {
		fmt.Fprintln(stderr, "refusing to cancel without --yes")
		return 5
	}
	client, ok := opsAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	cancelled, err := client.cancelAdminRun(context.Background(), *runID, strings.TrimSpace(*reason))
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, cancelled, stderr)
	}
	fmt.Fprintf(stdout, "run cancelled: %s\n", cancelled.RunID)
	fmt.Fprintf(stdout, "status: %s\n", cancelled.Status)
	return 0
}
