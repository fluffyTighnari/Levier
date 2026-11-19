package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/fluffyTighnari/levier/internal/daemoncheck"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func runDaemon(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printDaemonUsage(stderr)
		return 2
	}
	switch args[0] {
	case "check":
		return runDaemonCheck(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printDaemonUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown daemon command: %s\n\n", args[0])
		printDaemonUsage(stderr)
		return 2
	}
}

func printDaemonUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier daemon check [flags]")
	fmt.Fprintln(out)
	printDaemonCheckUsage(out)
}

func printDaemonCheckUsage(out io.Writer) {
	fmt.Fprintln(out, "flags:")
	fmt.Fprintln(out, "  --grpc addr        Levier gRPC address (default 127.0.0.1:9090)")
	fmt.Fprintln(out, "  --server url       Levier HTTP address (default http://127.0.0.1:8080)")
	fmt.Fprintln(out, "  --daemon-id id     pre-provisioned daemon id (default daemon-local-1)")
	fmt.Fprintln(out, "  --token token      daemon auth token")
	fmt.Fprintln(out, "  --admin-token tok  admin token for app bootstrap")
	fmt.Fprintln(out, "  --runtime name     runtime name to advertise (default echo1)")
	fmt.Fprintln(out, "  --timeout duration per-step timeout (default 30s)")
	fmt.Fprintln(out, "  --json             print JSON")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Protocol conformance check: creates a temporary App and Run.")
	fmt.Fprintln(out, "Use a pre-provisioned daemon id and token, not a live harness id.")
}

func runDaemonCheck(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("daemon check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}
	cfg := daemoncheck.Config{}
	flags.StringVar(&cfg.GRPCAddr, "grpc", envOr("LEVIER_GRPC", "127.0.0.1:9090"), "Levier gRPC address")
	flags.StringVar(&cfg.HTTPURL, "server", envOr("LEVIER_SERVER", "http://127.0.0.1:8080"), "Levier HTTP address")
	flags.StringVar(&cfg.DaemonID, "daemon-id", envOr("LEVIER_DAEMON_ID", "daemon-local-1"), "pre-provisioned daemon id")
	flags.StringVar(&cfg.Token, "token", envOr("LEVIER_DAEMON_TOKEN", "change-me"), "daemon auth token")
	flags.StringVar(&cfg.AdminToken, "admin-token", envOr("LEVIER_ADMIN_TOKEN", "admin-change-me"), "admin token for app bootstrap")
	flags.StringVar(&cfg.RuntimeName, "runtime", "echo1", "runtime name to advertise")
	flags.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "per-step timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printDaemonCheckUsage(stdout)
			return 0
		}
		fmt.Fprintf(stderr, "daemon check: %v\n\n", err)
		printDaemonCheckUsage(stderr)
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "daemon check: unexpected argument: %s\n\n", flags.Arg(0))
		printDaemonCheckUsage(stderr)
		return 2
	}

	checks, err := daemoncheck.Run(cfg)
	result := struct {
		OK     bool              `json:"ok"`
		Checks map[string]string `json:"checks"`
		Error  string            `json:"error,omitempty"`
	}{OK: err == nil, Checks: checks}
	if err != nil {
		result.Error = err.Error()
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if encodeErr := encoder.Encode(result); encodeErr != nil {
			fmt.Fprintf(stderr, "encode daemon check: %v\n", encodeErr)
			return 1
		}
	} else {
		keys := make([]string, 0, len(checks))
		for key := range checks {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for index, key := range keys {
			fmt.Fprintf(stdout, "[%d/%d] %-16s %s\n", index+1, len(keys), key, checks[key])
		}
		if err != nil {
			fmt.Fprintf(stderr, "daemon check: FAIL: %v\n", err)
		} else {
			fmt.Fprintln(stdout, "daemon check: PASS")
		}
	}
	if err != nil {
		return daemonCheckExitCode(err)
	}
	return 0
}

func daemonCheckExitCode(err error) int {
	var httpErr *daemoncheck.HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.Status == 0 {
			return 4
		}
		return exitCodeForStatus(httpErr.Status)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return 4
	}
	switch grpcstatus.Code(err) {
	case codes.Unauthenticated, codes.PermissionDenied:
		return 3
	case codes.Unavailable, codes.DeadlineExceeded:
		return 4
	case codes.AlreadyExists, codes.FailedPrecondition:
		return 5
	default:
		return 1
	}
}
