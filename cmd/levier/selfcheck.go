// Levier — hop-by-hop self-check command.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/fluffyTighnari/levier/internal/selfcheck"
)

func runSelfcheck(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("selfcheck", flag.ContinueOnError)
	configPath := flags.String("config", envOr("LEVIER_CONFIG", "levier.example.yaml"), "path to YAML configuration")
	server := flags.String("server", envOr("LEVIER_SERVER", "http://127.0.0.1:8080"), "Levier HTTP address")
	grpcAddr := flags.String("grpc", envOr("LEVIER_GRPC", "127.0.0.1:9090"), "Levier gRPC address")
	adminToken := flags.String("admin-token", os.Getenv("LEVIER_ADMIN_TOKEN"), "admin token")
	credentials := flags.String("credentials", os.Getenv("LEVIER_CREDENTIALS"), "optional App credentials file")
	daemonID := flags.String("daemon-id", envOr("LEVIER_DAEMON_ID", "daemon-local-1"), "daemon id for --deep")
	daemonToken := flags.String("daemon-token", os.Getenv("LEVIER_DAEMON_TOKEN"), "daemon token for --deep")
	mcpSocket := flags.String("mcp-socket", os.Getenv("LEVIER_MCP_SOCKET"), "MCP runtime socket for --deep")
	strategy := flags.String("strategy", "echo", "daemon strategy for --deep")
	timeout := flags.Duration("timeout", 5*time.Second, "per-hop timeout")
	deep := flags.Bool("deep", false, "run active daemon protocol checks")
	asJSON := flags.Bool("json", false, "print JSON")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "selfcheck: unexpected argument: %s\n\n", flags.Arg(0))
		printSelfcheckUsage(stderr)
		return 2
	}

	report := selfcheck.Run(selfcheck.Options{
		ConfigPath:  *configPath,
		Server:      *server,
		GRPCAddr:    *grpcAddr,
		AdminToken:  *adminToken,
		Credentials: *credentials,
		DaemonID:    *daemonID,
		DaemonToken: *daemonToken,
		Strategy:    *strategy,
		MCPSocket:   *mcpSocket,
		Timeout:     *timeout,
		Deep:        *deep,
	})

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "encode selfcheck: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintf(stdout, "selfcheck: %s\n", report.Server)
		for index, hop := range report.Hops {
			fmt.Fprintf(stdout, "[%d/%d] %-22s %s", index+1, len(report.Hops), hop.Name, hop.Status)
			if hop.Message != "" {
				fmt.Fprintf(stdout, "  %s", hop.Message)
			}
			fmt.Fprintln(stdout)
			if hop.Status == selfcheck.StatusFail && hop.Fix != "" {
				fmt.Fprintf(stdout, "      fix: %s\n", hop.Fix)
			}
		}
		if report.OK {
			fmt.Fprintln(stdout, "selfcheck: PASS")
		} else {
			fmt.Fprintln(stdout, "selfcheck: FAIL")
		}
	}
	if !report.OK {
		return 1
	}
	return 0
}

func printSelfcheckUsage(out io.Writer) {
	fmt.Fprintln(out, "flags:")
	fmt.Fprintln(out, "  --config path       YAML config (default levier.example.yaml)")
	fmt.Fprintln(out, "  --server url        Levier HTTP address (default http://127.0.0.1:8080)")
	fmt.Fprintln(out, "  --grpc addr         Levier gRPC address (default 127.0.0.1:9090)")
	fmt.Fprintln(out, "  --admin-token tok   admin token for admin check")
	fmt.Fprintln(out, "  --credentials path  optional App credentials for App check")
	fmt.Fprintln(out, "  --daemon-id id      daemon id for --deep")
	fmt.Fprintln(out, "  --daemon-token tok  daemon token for --deep")
	fmt.Fprintln(out, "  --mcp-socket path   MCP runtime socket for --deep")
	fmt.Fprintln(out, "  --strategy name     daemon strategy for --deep (default echo)")
	fmt.Fprintln(out, "  --timeout duration  per-hop timeout (default 5s)")
	fmt.Fprintln(out, "  --deep              enable active daemon protocol checks")
	fmt.Fprintln(out, "  --json              print JSON")
}
