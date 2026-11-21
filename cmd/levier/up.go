package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runUp(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("up", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "docker-compose file")
	server := flags.String("server", "http://127.0.0.1:8080", "Levier HTTP address")
	profiles := flags.String("profile", "", "comma-separated compose profiles")
	build := flags.Bool("build", true, "build images before starting")
	all := flags.Bool("all", false, "also start the example App container")
	wait := flags.Bool("wait", true, "wait until the control plane is ready")
	dryRun := flags.Bool("dry-run", false, "print the docker compose command")
	waitTimeout := flags.Duration("timeout", 3*time.Minute, "readiness timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	target, err := resolveStackTarget(*file, "", "")
	if err != nil {
		fmt.Fprintln(stderr, "up:", err)
		return 2
	}
	if err := os.MkdirAll(filepath.Join(target.projectDir, "data", "control"), 0o700); err != nil {
		fmt.Fprintf(stderr, "up: create data directory: %v\n", err)
		return 1
	}
	target.profiles = *profiles
	compose, err := composeCommand(target)
	if err != nil {
		fmt.Fprintln(stderr, "up:", err)
		return 2
	}
	compose = append(compose, "up")
	if *build {
		compose = append(compose, "--build")
	}
	compose = append(compose, "--detach")
	if *all {
		compose = append(compose, "postgres", "levier", "mcp-runtime", "react-runtime", "echo-runtime", "agent")
	} else {
		compose = append(compose, "postgres", "levier", "mcp-runtime", "react-runtime", "echo-runtime")
	}
	if *dryRun {
		fmt.Fprintln(stdout, formatCommand(compose))
		return 0
	}
	if code := runCompose(compose, false, stdout, stderr); code != 0 {
		return code
	}

	serverURL := serverBase(*server)
	if *wait {
		fmt.Fprintf(stdout, "waiting for %s ...\n", serverURL)
		if err := waitForReady(serverURL, *waitTimeout); err != nil {
			fmt.Fprintf(stderr, "up: %v\n", err)
			fmt.Fprintln(stderr, "logs: levier logs")
			return 1
		}
	}
	profile := loadProfile()
	profile.Server = serverURL
	profile.StackFile = target.file
	if strings.TrimSpace(profile.Runtime) == "" {
		profile.Runtime = "react1"
	}
	profile.MiddlewareAddress = middlewareAddressFor(serverURL)
	if strings.TrimSpace(profile.MCPSocket) == "" && isLoopbackServer(serverURL) {
		profile.MCPSocket = "tcp://127.0.0.1:9091"
	}
	if strings.TrimSpace(profile.RuntimeToken) == "" && isLoopbackServer(serverURL) {
		profile.RuntimeToken = "change-me"
	}
	if _, err := saveProfile(profile); err != nil {
		fmt.Fprintf(stderr, "up: save profile: %v\n", err)
		return 1
	}
	project := loadProjectConfig()
	if project.Server != "" || project.AppName != "" {
		project.Server = serverURL
		if err := saveProjectConfig(project); err != nil {
			fmt.Fprintf(stderr, "warning: could not update .levier.json: %v\n", err)
		}
	}
	fmt.Fprintln(stdout, "levier is ready")
	fmt.Fprintln(stdout, "next:")
	fmt.Fprintf(stdout, "  levier model connect --url https://api.deepseek.com --key sk-... --model deepseek-chat\n")
	fmt.Fprintln(stdout, "  levier init")
	return 0
}

func runDown(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("down", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "docker-compose file")
	volumes := flags.Bool("volumes", false, "remove named volumes")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	target, err := resolveStackTarget(*file, "", "")
	if err != nil {
		fmt.Fprintln(stderr, "down:", err)
		return 2
	}
	compose, err := composeCommand(target)
	if err != nil {
		fmt.Fprintln(stderr, "down:", err)
		return 2
	}
	compose = append(compose, "down", "--remove-orphans")
	if *volumes {
		compose = append(compose, "--volumes")
	}
	return runCompose(compose, false, stdout, stderr)
}

func runLogs(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("logs", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "docker-compose file")
	follow := flags.Bool("follow", false, "follow log output")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	target, err := resolveStackTarget(*file, "", "")
	if err != nil {
		fmt.Fprintln(stderr, "logs:", err)
		return 2
	}
	compose, err := composeCommand(target)
	if err != nil {
		fmt.Fprintln(stderr, "logs:", err)
		return 2
	}
	compose = append(compose, "logs")
	if *follow {
		compose = append(compose, "--follow")
	}
	return runCompose(compose, false, stdout, stderr)
}

func runPS(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("ps", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "docker-compose file")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	target, err := resolveStackTarget(*file, "", "")
	if err != nil {
		fmt.Fprintln(stderr, "ps:", err)
		return 2
	}
	compose, err := composeCommand(target)
	if err != nil {
		fmt.Fprintln(stderr, "ps:", err)
		return 2
	}
	compose = append(compose, "ps")
	return runCompose(compose, false, stdout, stderr)
}

func waitForReady(serverURL string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		if code, _, err := httpGet(ctx, client, serverURL+"/readyz", ""); err == nil && code == http.StatusOK {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("control plane did not become ready within %s", timeout)
		case <-time.After(time.Second):
		}
	}
}

func middlewareAddressFor(server string) string {
	parsed, err := url.Parse(server)
	if err != nil || parsed.Hostname() == "" {
		return "127.0.0.1:9090"
	}
	return net.JoinHostPort(parsed.Hostname(), "9090")
}
