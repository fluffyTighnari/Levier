package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func runMCP(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printMCPUsage(stderr)
		return 2
	}
	switch args[0] {
	case "list":
		return runMCPList(args[1:], stdout, stderr)
	case "add", "update":
		return runMCPUpsert(args[1:], stdout, stderr)
	case "remove", "delete":
		return runMCPRemove(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printMCPUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown mcp command: %s\n\n", args[0])
		printMCPUsage(stderr)
		return 2
	}
}

func printMCPUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier mcp list [--json]")
	fmt.Fprintln(out, "  levier mcp add --name <name> --transport stdio|sse|streamable_http --config-file <json> [--allowed-tools a,b]")
	fmt.Fprintln(out, "  levier mcp remove --name <name> --yes [--force]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "MCP mutations require an Admin token.")
}

func mcpFlagSet(name string) (*flag.FlagSet, *string, *string, *time.Duration, *bool) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	return flags, server, adminToken, timeout, asJSON
}

func runMCPList(args []string, stdout, stderr io.Writer) int {
	flags, server, adminToken, timeout, asJSON := mcpFlagSet("mcp list")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	servers, err := client.listMCPServers(context.Background())
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, map[string]any{"servers": servers}, stderr)
	}
	if len(servers) == 0 {
		fmt.Fprintln(stdout, "no MCP servers")
		return 0
	}
	fmt.Fprintf(stdout, "%-20s %-18s %s\n", "name", "transport", "secret_refs")
	for _, server := range servers {
		fmt.Fprintf(stdout, "%-20s %-18s %s\n", server.Name, server.Transport, strings.Join(server.SecretRefs, ", "))
	}
	return 0
}

func runMCPUpsert(args []string, stdout, stderr io.Writer) int {
	flags, server, adminToken, timeout, asJSON := mcpFlagSet("mcp add")
	name := flags.String("name", "", "MCP server name")
	transport := flags.String("transport", "stdio", "stdio, sse, or streamable_http")
	configFile := flags.String("config-file", "", "JSON config file")
	allowed := flags.String("allowed-tools", "", "comma-separated allowed tools")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if *name == "" || *configFile == "" {
		fmt.Fprintln(stderr, "mcp name and --config-file are required")
		return 2
	}
	data, err := os.ReadFile(*configFile)
	if err != nil {
		fmt.Fprintf(stderr, "read MCP config: %v\n", err)
		return 2
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Fprintf(stderr, "MCP config must be a JSON object: %v\n", err)
		return 2
	}
	payload := map[string]any{
		"name":          *name,
		"transport":     *transport,
		"config":        config,
		"allowed_tools": splitCSV(*allowed),
	}
	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	view, err := client.upsertMCPServer(context.Background(), payload)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, view, stderr)
	}
	fmt.Fprintf(stdout, "MCP server saved: %s (%s)\n", view.Name, view.Transport)
	return 0
}

func runMCPRemove(args []string, stdout, stderr io.Writer) int {
	flags, server, adminToken, timeout, asJSON := mcpFlagSet("mcp remove")
	name := flags.String("name", "", "MCP server name")
	yes := flags.Bool("yes", false, "confirm removal")
	force := flags.Bool("force", false, "also remove App references")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if *name == "" {
		fmt.Fprintln(stderr, "mcp name is required: --name")
		return 2
	}
	if !*yes {
		fmt.Fprintln(stderr, "refusing to remove without --yes")
		return 5
	}
	client, ok := providerAdminClient(*server, *adminToken, *timeout, stderr)
	if !ok {
		return 2
	}
	if err := client.deleteMCPServer(context.Background(), *name, *force); err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, map[string]any{"status": "deleted", "name": *name}, stderr)
	}
	fmt.Fprintf(stdout, "MCP server removed: %s\n", *name)
	return 0
}
