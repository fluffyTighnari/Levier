package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/fluffyTighnari/levier/internal/version"
)

type versionInfo struct {
	Version  string `json:"version"`
	Commit   string `json:"commit"`
	Built    string `json:"built"`
	Go       string `json:"go"`
	Platform string `json:"platform"`
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		runServer(args)
		return 0
	}

	switch args[0] {
	case "version":
		return printVersion(args[1:], stdout, stderr)
	case "describe":
		return runDescribe(args[1:], stdout, stderr)
	case "selfcheck":
		return runSelfcheck(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "app":
		return runApp(args[1:], stdout, stderr)
	case "run":
		return runRun(args[1:], stdout, stderr)
	case "migrate":
		return runMigrate(args[1:], stdout, stderr)
	case "maintenance":
		return runMaintenance(args[1:], stdout, stderr)
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "daemon":
		return runDaemon(args[1:], stdout, stderr)
	case "serve":
		runServer(args[1:])
		return 0
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	}

	// Backward compatibility: `levier -config path` starts the server.
	if strings.HasPrefix(args[0], "-") {
		runServer(args)
		return 0
	}

	fmt.Fprintf(stderr, "unknown command: %s\n\n", args[0])
	printUsage(stderr)
	return 2
}

func printVersion(args []string, stdout, stderr io.Writer) int {
	info := versionInfo{
		Version:  version.Version,
		Commit:   version.Commit,
		Built:    version.Built,
		Go:       runtime.Version(),
		Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}

	asJSON := false
	for _, arg := range args {
		switch arg {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintln(stdout, "usage: levier version [--json]")
			return 0
		default:
			fmt.Fprintf(stderr, "unknown version flag: %s\n", arg)
			return 2
		}
	}

	if asJSON {
		encoder := json.NewEncoder(stdout)
		if err := encoder.Encode(info); err != nil {
			fmt.Fprintf(stderr, "encode version: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(stdout, "levier %s\n", info.Version)
	fmt.Fprintf(stdout, "commit: %s\n", info.Commit)
	fmt.Fprintf(stdout, "built: %s\n", info.Built)
	fmt.Fprintf(stdout, "go: %s\n", info.Go)
	fmt.Fprintf(stdout, "platform: %s\n", info.Platform)
	return 0
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "levier — Agent Run control plane")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  levier serve [-config path]")
	fmt.Fprintln(out, "  levier version [--json]")
	fmt.Fprintln(out, "  levier describe [--server url] [--json]")
	fmt.Fprintln(out, "  levier selfcheck [options]")
	fmt.Fprintln(out, "  levier doctor [--config path] [--server url] [--credentials path] [--json]")
	fmt.Fprintln(out, "  levier status [--server url] [--json]")
	fmt.Fprintln(out, "  levier app init|show|rotate-key")
	fmt.Fprintln(out, "  levier run submit|watch|cancel")
	fmt.Fprintln(out, "  levier migrate status|up")
	fmt.Fprintln(out, "  levier maintenance status|prune")
	fmt.Fprintln(out, "  levier config check|print")
	fmt.Fprintln(out, "  levier daemon check")
	fmt.Fprintln(out, "  levier help")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Compatibility:")
	fmt.Fprintln(out, "  levier [-config path]   same as levier serve")
}
