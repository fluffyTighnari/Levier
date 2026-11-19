package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

type appListResult struct {
	Apps []appView `json:"apps"`
}

func runAppList(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("app list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "app list: unexpected argument: %s\n\n", flags.Arg(0))
		printAppUsage(stderr)
		return 2
	}
	admin := *adminToken
	if admin == "" {
		admin = os.Getenv("LEVIER_ADMIN_TOKEN")
	}
	if admin == "" {
		fmt.Fprintln(stderr, "admin token is required: --admin-token or LEVIER_ADMIN_TOKEN")
		return 2
	}

	client := newAPIClient(resolveServer(*server, nil), admin, *timeout)
	apps, err := client.listApps(context.Background())
	if err != nil {
		return reportAPIError(err, stderr)
	}
	result := appListResult{Apps: apps}
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintf(stderr, "encode app list: %v\n", err)
			return 1
		}
		return 0
	}
	if len(apps) == 0 {
		fmt.Fprintln(stdout, "no apps")
		return 0
	}
	fmt.Fprintf(stdout, "%-24s %-24s %-10s %s\n", "app_id", "name", "status", "created_at")
	for _, app := range apps {
		fmt.Fprintf(stdout, "%-24s %-24s %-10s %s\n", app.AppID, app.Name, app.Status, app.CreatedAt)
	}
	return 0
}
