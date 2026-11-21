package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runDev starts the App in the current project without asking for credential,
// server, or MCP flags.
func runDev(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("dev", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "App file (default: app.py)")
	python := flags.String("python", "", "Python executable")
	server := flags.String("server", defaultServerValue(), "Levier HTTP address")
	credentials := flags.String("credentials", defaultCredentialsValue(), "credentials file")
	mcpRefs := flags.String("mcp-ref", "", "comma-separated MCP refs")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	appFile, err := resolveProjectFile(*file, "app.py", "levier_app.py")
	if err != nil {
		fmt.Fprintln(stderr, "dev:", err)
		fmt.Fprintln(stderr, "create app.py with: from levier import App, tool")
		return 2
	}
	if strings.TrimSpace(*credentials) == "" {
		fmt.Fprintln(stderr, "no App is initialized; run: levier init")
		return 2
	}
	env := os.Environ()
	env = setEnv(env, "LEVIER_CREDENTIALS", *credentials)
	env = setEnv(env, "LEVIER_SERVER", serverBase(*server))
	refs := strings.TrimSpace(*mcpRefs)
	if refs == "" {
		refs = defaultMCPRefsValue()
	}
	if refs != "" {
		env = setEnv(env, "LEVIER_APP_MCP_REFS", refs)
	}
	fmt.Fprintf(stdout, "app:     %s\n", appFile)
	fmt.Fprintf(stdout, "server:  %s\n", serverBase(*server))
	if err := execPython(*python, appFile, nil, env); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "dev: %v\n", err)
		return 1
	}
	return 0
}

// runRuntimeStart starts a runtime file with profile-derived identity and
// connectivity. Runtime token and YAML are optional in the golden path.
func runRuntimeStart(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("runtime start", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "runtime file (default: runtime.py)")
	name := flags.String("name", "", "runtime routing name (default: current directory)")
	token := flags.String("token", "", "runtime credential (or profile / LEVIER_RUNTIME_TOKEN)")
	address := flags.String("grpc", defaultMiddlewareAddressValue(), "Levier gRPC address")
	python := flags.String("python", "", "Python executable")
	config := flags.String("config", "", "optional runtime YAML")
	logLevel := flags.String("log-level", "", "log level")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	runtimeFile, err := resolveProjectFile(*file, "runtime.py", "levier_runtime.py", "my_runtime.py")
	if err != nil {
		fmt.Fprintln(stderr, "runtime start:", err)
		fmt.Fprintln(stderr, "create runtime.py with: levier runtime init")
		return 2
	}
	if *name == "" {
		*name = strings.TrimSpace(os.Getenv("LEVIER_RUNTIME_NAME"))
	}
	if *name == "" {
		workingDir, err := os.Getwd()
		if err == nil {
			*name = safeAppName(filepath.Base(workingDir))
		}
	}
	if *name == "" || *name == "app" {
		*name = "local"
	}
	if *token == "" {
		*token = defaultRuntimeTokenValue()
	}
	if *token == "" && isLoopbackHost(hostOnly(*address)) {
		*token = "change-me"
	}
	if *token == "" {
		fmt.Fprintln(stderr, "runtime credential is required: run `levier up`, or set LEVIER_RUNTIME_TOKEN")
		return 2
	}
	env := os.Environ()
	env = setEnv(env, "LEVIER_RUNTIME_NAME", *name)
	env = setEnv(env, "LEVIER_RUNTIME_TOKEN", *token)
	env = setEnv(env, "LEVIER_MIDDLEWARE_ADDRESS", *address)
	if value := strings.TrimSpace(loadProfile().MCPSocket); value != "" {
		env = setEnv(env, "LEVIER_MCP_RUNTIME_SOCKET", value)
	}
	if *config != "" {
		env = setEnv(env, "LEVIER_RUNTIME_CONFIG", *config)
	}
	if *logLevel != "" {
		env = setEnv(env, "LEVIER_RUNTIME_LOG_LEVEL", *logLevel)
	}
	fmt.Fprintf(stdout, "runtime: %s\n", *name)
	fmt.Fprintf(stdout, "file:    %s\n", runtimeFile)
	fmt.Fprintf(stdout, "server:  %s\n", *address)
	if err := execPython(*python, runtimeFile, nil, env); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "runtime start: %v\n", err)
		return 1
	}
	return 0
}

func runRuntimeInit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("runtime init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "runtime.py", "runtime file to create")
	force := flags.Bool("force", false, "overwrite an existing file")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if _, err := os.Stat(*file); err == nil && !*force {
		fmt.Fprintf(stderr, "runtime file already exists: %s\n", *file)
		return 2
	}
	if err := os.WriteFile(*file, []byte(runtimeTemplate), 0o644); err != nil {
		fmt.Fprintf(stderr, "write %s: %v\n", *file, err)
		return 1
	}
	fmt.Fprintf(stdout, "created %s\n", *file)
	fmt.Fprintln(stdout, "start it with: levier runtime start")
	return 0
}

func resolveProjectFile(explicit string, candidates ...string) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		if _, err := os.Stat(value); err != nil {
			return "", fmt.Errorf("file not found: %s", value)
		}
		return value, nil
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			absolute, err := filepath.Abs(candidate)
			if err == nil {
				return absolute, nil
			}
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no %s found in %s", strings.Join(candidates, " / "), filepath.Dir(candidates[0]))
}

func execPython(explicit, file string, args []string, env []string) error {
	command, err := launchPython(explicit, file, args, env)
	if err != nil {
		return err
	}
	return command.Wait()
}

func launchPython(explicit, file string, args []string, env []string) (*exec.Cmd, error) {
	python := strings.TrimSpace(explicit)
	if python == "" {
		python = strings.TrimSpace(os.Getenv("LEVIER_PYTHON"))
	}
	if python == "" {
		for _, candidate := range []string{"python", "python3"} {
			if path, err := exec.LookPath(candidate); err == nil {
				python = path
				break
			}
		}
	}
	if python == "" {
		return nil, fmt.Errorf("Python was not found; install Python or pass --python")
	}
	commandArgs := append([]string{file}, args...)
	command := exec.Command(python, commandArgs...)
	command.Env = env
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Stdin = os.Stdin
	if err := command.Start(); err != nil {
		return nil, err
	}
	return command, nil
}

type appProcess struct {
	command *exec.Cmd
	done    chan error
}

func startAppProcess(file, python, server, credentials, refs string) (*appProcess, error) {
	env := os.Environ()
	env = setEnv(env, "LEVIER_CREDENTIALS", credentials)
	env = setEnv(env, "LEVIER_SERVER", serverBase(server))
	if strings.TrimSpace(refs) != "" {
		env = setEnv(env, "LEVIER_APP_MCP_REFS", refs)
	}
	command, err := launchPython(python, file, nil, env)
	if err != nil {
		return nil, err
	}
	process := &appProcess{command: command, done: make(chan error, 1)}
	go func() { process.done <- command.Wait() }()
	return process, nil
}

func (p *appProcess) stop() {
	if p == nil || p.command == nil || p.command.Process == nil {
		return
	}
	_ = p.command.Process.Kill()
	select {
	case <-p.done:
	case <-time.After(3 * time.Second):
	}
}

func appConnections(server string) (int, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	code, body, err := httpGet(context.Background(), client, serverBase(server)+"/status", "")
	if err != nil {
		return 0, err
	}
	if code != http.StatusOK {
		return 0, fmt.Errorf("status returned %d", code)
	}
	var snapshot struct {
		Apps struct {
			WebSocketConnections int `json:"websocket_connections"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return 0, err
	}
	return snapshot.Apps.WebSocketConnections, nil
}

func waitForAppConnection(server string, baseline int, process *appProcess, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-process.done:
			process.done <- err // keep the result for stop()
			if err == nil {
				return fmt.Errorf("App process exited before connecting")
			}
			return fmt.Errorf("App process exited before connecting: %w", err)
		default:
		}
		count, err := appConnections(server)
		if err == nil && count > baseline {
			return nil
		}
		if err != nil && time.Until(deadline) < timeout-2*time.Second {
			// Older control planes may not expose /status; give the SDK a
			// short grace period rather than blocking the run forever.
			time.Sleep(time.Second)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("App did not connect within %s", timeout)
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for index, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[index] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func hostOnly(address string) string {
	if strings.HasPrefix(address, "[") {
		if end := strings.Index(address, "]"); end > 0 {
			return address[1:end]
		}
	}
	if index := strings.LastIndex(address, ":"); index > 0 {
		return address[:index]
	}
	return address
}

const runtimeTemplate = `"""A Levier runtime: write the logic, not the plumbing."""

from levier import Context, Runtime


async def run(ctx: Context) -> str:
    # Everything the Run needs is already on ctx.
    # tools = await ctx.toolbox()
    # reply = await ctx.chat(ctx.input, tools=tools)
    # results = await ctx.invoke_all(reply, toolbox=tools)
    text = ctx.input[-1].content if ctx.input else "hello"
    await ctx.reply(f"you said: {text}")
    return "completed"


if __name__ == "__main__":
    Runtime.start(run)
`
