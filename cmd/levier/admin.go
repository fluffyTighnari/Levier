package main

import (
	"fmt"
	"io"
)

func runAdmin(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printAdminUsage(stderr)
		return 2
	}
	switch args[0] {
	case "ops":
		return runOps(args[1:], stdout, stderr)
	case "migrate":
		return runMigrate(args[1:], stdout, stderr)
	case "maintenance":
		return runMaintenance(args[1:], stdout, stderr)
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "daemon":
		return runDaemon(args[1:], stdout, stderr)
	case "describe":
		return runDescribe(args[1:], stdout, stderr)
	case "selfcheck":
		return runSelfcheck(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printAdminUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown admin command: %s\n\n", args[0])
		printAdminUsage(stderr)
		return 2
	}
}

func printAdminUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier admin ops runs list|show|cancel")
	fmt.Fprintln(out, "  levier admin ops daemons")
	fmt.Fprintln(out, "  levier admin ops tasks")
	fmt.Fprintln(out, "  levier admin migrate status|up")
	fmt.Fprintln(out, "  levier admin maintenance status|prune")
	fmt.Fprintln(out, "  levier admin config check|print")
	fmt.Fprintln(out, "  levier admin daemon check")
	fmt.Fprintln(out, "  levier admin describe")
	fmt.Fprintln(out, "  levier admin selfcheck [--deep]")
}
