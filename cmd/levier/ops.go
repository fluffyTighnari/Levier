package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func runOps(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printOpsUsage(stderr)
		return 2
	}
	switch args[0] {
	case "runs":
		return runOpsRuns(args[1:], stdout, stderr)
	case "daemons":
		return runOpsDaemons(args[1:], stdout, stderr)
	case "tasks":
		return runOpsTasks(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printOpsUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown ops command: %s\n\n", args[0])
		printOpsUsage(stderr)
		return 2
	}
}

func printOpsUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier ops runs list [--status status] [--app-id id] [--daemon-id id]")
	fmt.Fprintln(out, "  levier ops runs show --run-id id")
	fmt.Fprintln(out, "  levier ops runs cancel --run-id id --reason text --yes")
	fmt.Fprintln(out, "  levier ops daemons")
	fmt.Fprintln(out, "  levier ops tasks [--status DEAD] [--type RUN_DISPATCH] [--run-id id]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "All ops commands require an Admin token.")
}

func opsAdminClient(server, token string, timeout time.Duration, stderr io.Writer) (*apiClient, bool) {
	if token == "" {
		token = os.Getenv("LEVIER_ADMIN_TOKEN")
	}
	if token == "" {
		fmt.Fprintln(stderr, "admin token is required: --admin-token or LEVIER_ADMIN_TOKEN")
		return nil, false
	}
	return newAPIClient(resolveServer(server, nil), token, timeout), true
}

func parseOpsFlagSet(name string, flags *flag.FlagSet, args []string, stdout, stderr io.Writer) (bool, int) {
	ok, code := parseFlagSet(flags, args, stdout, stderr)
	if !ok {
		return false, code
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "%s: unexpected argument: %s\n\n", name, flags.Arg(0))
		printOpsUsage(stderr)
		return false, 2
	}
	return true, 0
}
