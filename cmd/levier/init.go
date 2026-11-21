package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type initResult struct {
	AppID       string `json:"app_id"`
	AppName     string `json:"app_name"`
	Server      string `json:"server"`
	Runtime     string `json:"runtime"`
	Credentials string `json:"credentials,omitempty"`
	Created     bool   `json:"created"`
	Reused      bool   `json:"reused"`
}

// runInit is the golden-path App bootstrap. It hides credentials paths, admin
// tokens, and MCP flags behind project defaults.
func runInit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	nameFlag := flags.String("name", "", "App name (default: current directory)")
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	runtimeName := flags.String("runtime", defaultRuntimeValue(), "default runtime name")
	mcpRefs := flags.String("mcp-ref", "", "comma-separated MCP server names")
	rotate := flags.Bool("rotate", false, "replace a lost App key")
	force := flags.Bool("force", false, "overwrite an existing credentials file")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	flagArgs, positionals := splitArgs(args, map[string]bool{
		"name": true, "server": true, "admin-token": true,
		"runtime": true, "mcp-ref": true, "timeout": true,
	})
	if ok, code := parseFlagSet(flags, flagArgs, stdout, stderr); !ok {
		return code
	}
	name := strings.TrimSpace(*nameFlag)
	if len(positionals) > 0 {
		if name == "" {
			name = strings.TrimSpace(positionals[0])
		}
		if len(positionals) > 1 {
			fmt.Fprintf(stderr, "init: unexpected argument: %s\n", positionals[1])
			return 2
		}
	}
	if name == "" {
		name = defaultAppNameValue()
	}
	if name == "" {
		workingDir, err := os.Getwd()
		if err == nil {
			name = filepath.Base(workingDir)
		}
	}
	name = safeAppName(name)
	if name == "" || name == "." {
		name = "my-app"
	}

	credentialsPath := strings.TrimSpace(os.Getenv("LEVIER_CREDENTIALS"))
	if credentialsPath == "" {
		path, err := defaultCredentialsPath(name)
		if err != nil {
			fmt.Fprintf(stderr, "resolve credentials path: %v\n", err)
			return 2
		}
		credentialsPath = path
	}
	serverValue := strings.TrimSpace(*server)
	if serverValue == "" {
		serverValue = defaultServerValue()
	}
	serverURL := resolveServer(serverValue, nil)

	if credentials, err := readCredentials(credentialsPath); err == nil {
		if credentials.AppName != "" && credentials.AppName != name {
			fmt.Fprintf(stderr, "credentials already belong to %q: %s\n", credentials.AppName, credentialsPath)
			fmt.Fprintf(stderr, "use --name %s or point LEVIER_CREDENTIALS at another file\n", credentials.AppName)
			return 2
		}
		if !*rotate {
			if _, err := verifyAppKey(context.Background(), serverURL, credentials.AppID, credentials.APIKey, *timeout); err == nil {
				return saveInitResult(stdout, stderr, initResult{
					AppID:       credentials.AppID,
					AppName:     name,
					Server:      serverURL,
					Runtime:     *runtimeName,
					Credentials: credentialsPath,
					Reused:      true,
				}, *asJSON, *runtimeName, *mcpRefs)
			}
		}
	}

	if *adminToken == "" {
		*adminToken = defaultAdminTokenValue()
	}
	if *adminToken == "" {
		fmt.Fprintln(stderr, "admin token is required to create the App: set LEVIER_ADMIN_TOKEN")
		fmt.Fprintln(stderr, "the bundled local stack uses admin-change-me; run `levier up` once or pass --admin-token")
		return 2
	}
	if _, err := os.Stat(credentialsPath); err == nil && !*force && !*rotate {
		fmt.Fprintf(stderr, "credentials already exist: %s\n", credentialsPath)
		fmt.Fprintf(stderr, "recover the existing App key with: levier init --rotate\n")
		return 2
	}
	if err := preflightCredentialsPath(credentialsPath); err != nil {
		fmt.Fprintf(stderr, "credentials path is not writable: %v\n", err)
		return 2
	}

	ctx := context.Background()
	client := newAPIClient(serverURL, *adminToken, *timeout)
	app, key, err := client.createApp(ctx, name, false, splitCSV(*mcpRefs))
	if err != nil {
		return reportAPIError(err, stderr)
	}
	created := key != ""
	if !created {
		if !*rotate {
			fmt.Fprintf(stderr, "app %q already exists (%s), but its key is not available\n", name, app.AppID)
			fmt.Fprintln(stderr, "recover it with: levier init --rotate")
			return 2
		}
		key, err = client.rotateAppKey(ctx, app.AppID)
		if err != nil {
			return reportAPIError(err, stderr)
		}
	}
	if key == "" {
		fmt.Fprintln(stderr, "server returned an empty app key")
		return 1
	}
	credentials := appCredentials{
		AppID:     app.AppID,
		AppName:   app.Name,
		APIKey:    key,
		Server:    serverURL,
		CreatedAt: app.CreatedAt,
	}
	if _, err := writeCredentials(credentialsPath, credentials, *force || *rotate); err != nil {
		fmt.Fprintf(stderr, "write credentials: %v\n", err)
		return 1
	}
	if _, err := verifyAppKey(ctx, serverURL, app.AppID, key, *timeout); err != nil {
		fmt.Fprintf(stderr, "credentials were written, but the app key could not be verified: %v\n", err)
		return 1
	}
	return saveInitResult(stdout, stderr, initResult{
		AppID:       app.AppID,
		AppName:     app.Name,
		Server:      serverURL,
		Runtime:     *runtimeName,
		Credentials: credentialsPath,
		Created:     created,
	}, *asJSON, *runtimeName, *mcpRefs)
}

func saveInitResult(stdout, stderr io.Writer, result initResult, asJSON bool, runtimeName, mcpRefs string) int {
	profile := loadProfile()
	profile.Server = result.Server
	profile.Credentials = result.Credentials
	profile.AppName = result.AppName
	profile.Runtime = runtimeName
	if value := strings.TrimSpace(mcpRefs); value != "" {
		profile.MCPRefs = value
	} else if value := strings.TrimSpace(os.Getenv("LEVIER_APP_MCP_REFS")); value != "" {
		profile.MCPRefs = value
	}
	if profile.MiddlewareAddress == "" {
		profile.MiddlewareAddress = middlewareAddressFor(result.Server)
	}
	if profile.RuntimeToken == "" && isLoopbackServer(result.Server) {
		profile.RuntimeToken = "change-me"
	}
	if _, err := saveProfile(profile); err != nil {
		fmt.Fprintf(stderr, "save profile: %v\n", err)
		return 1
	}
	project := loadProjectConfig()
	project.AppName = result.AppName
	project.Credentials = result.Credentials
	project.Server = result.Server
	project.Runtime = runtimeName
	if value := strings.TrimSpace(mcpRefs); value != "" {
		project.MCPRefs = value
	}
	if err := saveProjectConfig(project); err != nil {
		fmt.Fprintf(stderr, "warning: could not write .levier.json: %v\n", err)
	}
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return 1
		}
		return 0
	}
	if result.Reused {
		fmt.Fprintf(stdout, "ready: %s (app already initialized)\n", result.AppName)
	} else {
		fmt.Fprintf(stdout, "ready: %s\n", result.AppName)
	}
	fmt.Fprintf(stdout, "app:     %s\n", result.AppID)
	fmt.Fprintf(stdout, "server:  %s\n", result.Server)
	fmt.Fprintf(stdout, "runtime: %s\n", result.Runtime)
	fmt.Fprintln(stdout, "next:")
	fmt.Fprintln(stdout, "  levier dev            # serve this project's tools")
	fmt.Fprintln(stdout, `  levier run "hello"    # submit a run without flags`)
	return 0
}

func isLoopbackServer(server string) bool {
	parsed, err := url.Parse(server)
	if err != nil {
		return false
	}
	return isLoopbackHost(parsed.Hostname())
}
