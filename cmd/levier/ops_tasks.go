package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func runOpsTasks(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("ops tasks", flag.ContinueOnError)
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	status := flags.String("status", "DEAD", "comma-separated task status")
	taskType := flags.String("type", "", "comma-separated task type")
	runID := flags.String("run-id", "", "filter by run id")
	limit := flags.Int("limit", 100, "page size")
	offset := flags.Int("offset", 0, "page offset")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	ok, code := parseOpsFlagSet("ops tasks", flags, args, stdout, stderr)
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
	if value := strings.TrimSpace(*taskType); value != "" {
		query.Set("type", value)
	}
	if value := strings.TrimSpace(*runID); value != "" {
		query.Set("run_id", value)
	}
	query.Set("limit", strconv.Itoa(*limit))
	query.Set("offset", strconv.Itoa(*offset))

	list, err := client.listAdminTasks(context.Background(), query)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, list, stderr)
	}
	if len(list.Tasks) == 0 {
		fmt.Fprintln(stdout, "no tasks")
		return 0
	}
	fmt.Fprintf(stdout, "%-26s %-24s %-14s %-10s %-8s %-20s %s\n",
		"task_id", "run_id", "type", "status", "attempt", "updated_at", "last_error")
	for _, task := range list.Tasks {
		fmt.Fprintf(stdout, "%-26s %-24s %-14s %-10s %-8d %-20s %s\n",
			task.TaskID, task.RunID, task.TaskType, task.Status, task.Attempt,
			task.UpdatedAt, task.LastError)
	}
	return 0
}
