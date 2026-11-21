package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"
)

func runRuntime(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		printRuntimeUsage(stdout)
		return 0
	}
	switch args[0] {
	case "start":
		return runRuntimeStart(args[1:], stdout, stderr)
	case "init":
		return runRuntimeInit(args[1:], stdout, stderr)
	case "list":
		return runRuntimeList(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown runtime command: %s\n\n", args[0])
		printRuntimeUsage(stderr)
		return 2
	}
}

func printRuntimeUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier runtime init [--file runtime.py]")
	fmt.Fprintln(out, "  levier runtime start [--file runtime.py] [--name name]")
	fmt.Fprintln(out, "  levier runtime list [--json]")
}

func runRuntimeList(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("runtime list", flag.ContinueOnError)
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
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
		fmt.Fprintln(stdout, "no connected runtimes")
		return 0
	}
	fmt.Fprintf(stdout, "%-20s %-20s %-10s %-8s %-8s %-8s %s\n",
		"runtime", "instance", "status", "capacity", "running", "available", "heartbeat")
	for _, daemon := range list.Daemons {
		fmt.Fprintf(stdout, "%-20s %-20s %-10s %-8d %-8d %-8d %s\n",
			daemon.RuntimeName, daemon.DaemonID, daemon.Status,
			daemon.MaxConcurrentRuns, daemon.Running, daemon.Available, daemon.LastHeartbeat)
	}
	return 0
}
