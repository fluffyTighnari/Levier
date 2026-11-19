package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"
)

func runOpsDaemons(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("ops daemons", flag.ContinueOnError)
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	ok, code := parseOpsFlagSet("ops daemons", flags, args, stdout, stderr)
	if !ok {
		return code
	}
	client, ok := opsAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	list, err := client.listAdminDaemons(context.Background())
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, list, stderr)
	}
	if len(list.Daemons) == 0 {
		fmt.Fprintln(stdout, "no connected daemons")
		return 0
	}
	fmt.Fprintf(stdout, "%-20s %-20s %-10s %-8s %-8s %-8s %-8s %-8s %s\n",
		"daemon_id", "runtime_name", "status", "healthy", "capacity", "running", "reserved", "available", "heartbeat")
	for _, daemon := range list.Daemons {
		fmt.Fprintf(stdout, "%-20s %-20s %-10s %-8t %-8d %-8d %-8d %-8d %s\n",
			daemon.DaemonID, daemon.RuntimeName, daemon.Status, daemon.Healthy,
			daemon.MaxConcurrentRuns, daemon.Running, daemon.Reserved, daemon.Available,
			daemon.LastHeartbeat)
	}
	return 0
}
