package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/fluffyTighnari/levier/internal/platform/version"
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
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "version":
		return printVersion(args[1:], stdout, stderr)
	case "up":
		return runUp(args[1:], stdout, stderr)
	case "down":
		return runDown(args[1:], stdout, stderr)
	case "logs":
		return runLogs(args[1:], stdout, stderr)
	case "ps":
		return runPS(args[1:], stdout, stderr)
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "dev":
		return runDev(args[1:], stdout, stderr)
	case "model":
		return runModel(args[1:], stdout, stderr)
	case "setup":
		return runSetup(args[1:], stdout, stderr)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "app":
		return runApp(args[1:], stdout, stderr)
	case "mcp":
		return runMCP(args[1:], stdout, stderr)
	case "run":
		return runRun(args[1:], stdout, stderr)
	case "runtime":
		return runRuntime(args[1:], stdout, stderr)
	case "stack":
		// Hidden compatibility; the public lifecycle is up/down/logs/ps.
		return runStack(args[1:], stdout, stderr)
	case "admin":
		return runAdmin(args[1:], stdout, stderr)
	// Compatibility aliases for deployment scripts and existing operators.
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
	fmt.Fprintln(out, "levier — run your agent, not your infrastructure")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Start:")
	fmt.Fprintln(out, "  levier up                     start the local Levier stack")
	fmt.Fprintln(out, "  levier init [name]            connect this project (defaults: current directory)")
	fmt.Fprintln(out, "  levier model connect --url URL --key KEY --model MODEL")
	fmt.Fprintln(out, "  levier dev                    serve tools from ./app.py")
	fmt.Fprintln(out, "  levier run \"prompt\"           submit a Run and watch it")
	fmt.Fprintln(out, "  levier runtime init           create ./runtime.py")
	fmt.Fprintln(out, "  levier runtime start          serve ./runtime.py")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Observe:")
	fmt.Fprintln(out, "  levier status                 health, runtimes, apps")
	fmt.Fprintln(out, "  levier run show RUN_ID        inspect one Run")
	fmt.Fprintln(out, "  levier logs [--follow]        stack logs")
	fmt.Fprintln(out, "  levier down                   stop the local stack")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Advanced commands remain available: app (internal), mcp, admin.")
	fmt.Fprintln(out, "Compatibility: levier serve / levier [-config path] starts the server.")
}
