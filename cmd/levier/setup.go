package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type setupResult struct {
	Profile           string `json:"profile"`
	Server            string `json:"server"`
	Credentials       string `json:"credentials,omitempty"`
	Runtime           string `json:"runtime,omitempty"`
	MiddlewareAddress string `json:"middleware_address,omitempty"`
	RuntimeConfig     string `json:"runtime_config,omitempty"`
	MCPSocket         string `json:"mcp_socket,omitempty"`
	ControlConfig     string `json:"control_config,omitempty"`
	StackFile         string `json:"stack_file,omitempty"`
}

func runSetup(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("setup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	current := loadProfile()
	serverDefault := defaultServerValue()
	credentialsDefault := current.Credentials
	if value := defaultCredentialsValue(); value != "" {
		credentialsDefault = value
	}
	runtimeConfigDefault := current.RuntimeConfig
	if value := strings.TrimSpace(os.Getenv("LEVIER_RUNTIME_CONFIG")); value != "" {
		runtimeConfigDefault = value
	}
	mcpSocketDefault := current.MCPSocket
	if value := defaultMCPSocketValue(); value != "" {
		mcpSocketDefault = value
	}
	stackFileDefault := current.StackFile
	if stackFileDefault == "" {
		stackFileDefault = installedStackFileValue()
	}
	server := flags.String("server", serverDefault, "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "optional Admin token to remember in the 0600 profile")
	credentials := flags.String("credentials", credentialsDefault, "default App credentials file")
	runtimeDefault := current.Runtime
	if strings.TrimSpace(runtimeDefault) == "" {
		runtimeDefault = defaultRuntimeValue()
	}
	runtimeName := flags.String("runtime", runtimeDefault, "default runtime name")
	middlewareAddress := flags.String("grpc", current.MiddlewareAddress, "Levier gRPC address")
	runtimeToken := flags.String("runtime-token", "", "optional runtime credential to remember in the 0600 profile")
	runtimeConfig := flags.String("runtime-config", runtimeConfigDefault, "default Runtime config file")
	mcpSocket := flags.String("mcp-socket", mcpSocketDefault, "default MCP Runtime socket")
	controlConfigDefault := current.ControlConfig
	if value := defaultControlConfigValue(); value != "" {
		controlConfigDefault = value
	}
	controlConfig := flags.String("control-config", controlConfigDefault, "control-plane YAML config (server operators only)")
	stackFile := flags.String("stack-file", stackFileDefault, "default docker-compose file for levier stack")
	reset := flags.Bool("reset", false, "clear existing profile values before saving")
	show := flags.Bool("show", false, "show the current profile without writing it")
	asJSON := flags.Bool("json", false, "print JSON")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}

	profile := current
	if *reset {
		profile = cliProfile{}
	}
	profile.Server = ""
	if value := strings.TrimSpace(*server); value != "" {
		profile.Server = serverBase(value)
	}
	if value := strings.TrimSpace(*adminToken); value != "" {
		profile.AdminToken = value
	}
	profile.Credentials = cleanProfilePath(*credentials)
	if value := strings.TrimSpace(*runtimeName); value != "" {
		profile.Runtime = value
	}
	if value := strings.TrimSpace(*middlewareAddress); value != "" {
		profile.MiddlewareAddress = value
	} else if profile.Server != "" {
		if parsed, err := url.Parse(profile.Server); err == nil && parsed.Hostname() != "" {
			profile.MiddlewareAddress = net.JoinHostPort(parsed.Hostname(), "9090")
		}
	}
	if value := strings.TrimSpace(*runtimeToken); value != "" {
		profile.RuntimeToken = value
	} else if profile.RuntimeToken == "" && profile.Server != "" {
		if parsed, err := url.Parse(profile.Server); err == nil && isLoopbackHost(parsed.Hostname()) {
			profile.RuntimeToken = "change-me"
		}
	}
	profile.RuntimeConfig = cleanProfilePath(*runtimeConfig)
	profile.MCPSocket = cleanProfilePath(*mcpSocket)
	profile.ControlConfig = cleanProfilePath(*controlConfig)
	profile.StackFile = cleanProfilePath(*stackFile)
	if profile.StackFile == "" {
		profile.StackFile = installedStackFileValue()
	}

	path, err := profilePath()
	if err != nil {
		fmt.Fprintf(stderr, "resolve profile path: %v\n", err)
		return 2
	}
	result := setupResult{
		Profile:           path,
		Server:            profile.Server,
		Credentials:       profile.Credentials,
		Runtime:           profile.Runtime,
		MiddlewareAddress: profile.MiddlewareAddress,
		RuntimeConfig:     profile.RuntimeConfig,
		MCPSocket:         profile.MCPSocket,
		ControlConfig:     profile.ControlConfig,
		StackFile:         profile.StackFile,
	}
	if *show {
		return writeSetupResult(stdout, result, *asJSON, false)
	}
	if profile.Server == "" {
		fmt.Fprintln(stderr, "server address is required")
		return 2
	}

	if _, err := saveProfile(profile); err != nil {
		fmt.Fprintf(stderr, "write profile: %v\n", err)
		return 1
	}
	return writeSetupResult(stdout, result, *asJSON, true)
}

func cleanProfilePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if value == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			value = home
		}
	} else if strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			value = filepath.Join(home, value[2:])
		}
	}
	if absolute, err := filepath.Abs(value); err == nil {
		return absolute
	}
	return value
}

func writeSetupResult(stdout io.Writer, result setupResult, asJSON, saved bool) int {
	if asJSON {
		output := struct {
			setupResult
			Saved bool `json:"saved"`
		}{setupResult: result, Saved: saved}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(output); err != nil {
			return 1
		}
		return 0
	}

	fmt.Fprintf(stdout, "profile      %s\n", result.Profile)
	fmt.Fprintf(stdout, "server       %s\n", result.Server)
	if result.Runtime != "" {
		fmt.Fprintf(stdout, "runtime      %s\n", result.Runtime)
	}
	if result.MiddlewareAddress != "" {
		fmt.Fprintf(stdout, "grpc         %s\n", result.MiddlewareAddress)
	}
	if result.Credentials != "" {
		fmt.Fprintf(stdout, "credentials  %s\n", result.Credentials)
	}
	if result.RuntimeConfig != "" {
		fmt.Fprintf(stdout, "runtime      %s\n", result.RuntimeConfig)
	}
	if result.MCPSocket != "" {
		fmt.Fprintf(stdout, "mcp_socket   %s\n", result.MCPSocket)
	}
	if result.ControlConfig != "" {
		fmt.Fprintf(stdout, "control      %s\n", result.ControlConfig)
	}
	if result.StackFile != "" {
		fmt.Fprintf(stdout, "stack        %s\n", result.StackFile)
	}
	if !saved {
		return 0
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "saved. Next:")
	if result.StackFile != "" {
		fmt.Fprintln(stdout, "  levier up")
	} else {
		fmt.Fprintln(stdout, "  levier status")
	}
	fmt.Fprintln(stdout, "  levier model connect --url URL --key KEY --model MODEL")
	fmt.Fprintln(stdout, "  levier init")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "The profile is 0600. Prefer LEVIER_ADMIN_TOKEN in production instead of --admin-token.")
	return 0
}
