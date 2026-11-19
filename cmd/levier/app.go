package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

type appInitResult struct {
	AppID             string `json:"app_id"`
	AppName           string `json:"app_name"`
	Server            string `json:"server,omitempty"`
	Credentials       string `json:"credentials"`
	CredentialVersion int    `json:"credential_version,omitempty"`
	KeyFingerprint    string `json:"key_fingerprint,omitempty"`
	Created           bool   `json:"created"`
	Rotated           bool   `json:"rotated"`
	Backup            string `json:"backup,omitempty"`
}

type appShowResult struct {
	appView
	Credentials         string `json:"credentials"`
	CredentialVersion   int    `json:"credential_version"`
	Server              string `json:"server"`
	KeyFingerprint      string `json:"key_fingerprint"`
	CredentialCreatedAt string `json:"credential_created_at,omitempty"`
	CredentialUpdatedAt string `json:"credential_updated_at,omitempty"`
}

func runApp(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printAppUsage(stderr)
		return 2
	}
	switch args[0] {
	case "init":
		return runAppInit(args[1:], stdout, stderr)
	case "show":
		return runAppShow(args[1:], stdout, stderr)
	case "check":
		return runAppCheck(args[1:], stdout, stderr)
	case "list":
		return runAppList(args[1:], stdout, stderr)
	case "rotate-key":
		return runAppRotateKey(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printAppUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown app command: %s\n\n", args[0])
		printAppUsage(stderr)
		return 2
	}
}

func printAppUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier app init --name <app> [--credentials-out path]")
	fmt.Fprintln(out, "  levier app show --credentials path")
	fmt.Fprintln(out, "  levier app check --credentials path")
	fmt.Fprintln(out, "  levier app rotate-key --credentials path --yes")
	fmt.Fprintln(out, "  levier app list [--admin-token token]")
}

func runAppInit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("app init", flag.ContinueOnError)
	flags.SetOutput(stderr)
	nameFlag := flags.String("name", os.Getenv("LEVIER_APP_NAME"), "App name")
	credentialsFlag := flags.String("credentials-out", "", "credentials output path")
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	rotate := flags.Bool("rotate", false, "rotate key if the App already exists")
	yes := flags.Bool("yes", false, "confirm rotation")
	force := flags.Bool("force", false, "overwrite an existing credentials file")
	dryRun := flags.Bool("dry-run", false, "show what would happen")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}

	name := *nameFlag
	if name == "" {
		fmt.Fprintln(stderr, "app name is required: --name or LEVIER_APP_NAME")
		return 2
	}
	credentialsPath := *credentialsFlag
	if credentialsPath == "" {
		path, err := defaultCredentialsPath(name)
		if err != nil {
			fmt.Fprintf(stderr, "resolve credentials path: %v\n", err)
			return 2
		}
		credentialsPath = path
	}
	serverURL := resolveServer(*server, nil)
	if *dryRun {
		result := appInitResult{AppName: name, Server: serverURL, Credentials: credentialsPath}
		return writeAppInitResult(stdout, result, *asJSON, true)
	}
	admin := *adminToken
	if admin == "" {
		admin = os.Getenv("LEVIER_ADMIN_TOKEN")
	}
	if admin == "" {
		fmt.Fprintln(stderr, "admin token is required: --admin-token or LEVIER_ADMIN_TOKEN")
		return 2
	}

	if _, err := os.Stat(credentialsPath); err == nil && !*force {
		fmt.Fprintf(stderr, "credentials already exist: %s (use --force to overwrite)\n", credentialsPath)
		return 2
	}
	if err := preflightCredentialsPath(credentialsPath); err != nil {
		fmt.Fprintf(stderr, "credentials path is not writable: %v\n", err)
		return 2
	}

	ctx := context.Background()
	client := newAPIClient(serverURL, admin, *timeout)
	app, key, err := client.createApp(ctx, name, false)
	if err != nil {
		return reportAPIError(err, stderr)
	}

	created := key != ""
	rotated := false
	if !created {
		if !*rotate {
			result := appInitResult{AppID: app.AppID, AppName: app.Name, Server: serverURL, Credentials: credentialsPath}
			if *asJSON {
				return writeAppInitResult(stdout, result, true, false)
			}
			fmt.Fprintf(stdout, "app exists: %s\n", app.AppID)
			fmt.Fprintf(stdout, "key was not returned; rotate it with:\n")
			fmt.Fprintf(stdout, "  levier app rotate-key --credentials %s --yes\n", credentialsPath)
			return 0
		}
		if !requireConfirmation(*yes, fmt.Sprintf("Rotate key for %s? The old key stops working immediately.", app.AppID), stdout, stderr) {
			return 5
		}
		key, err = client.rotateAppKey(ctx, app.AppID)
		if err != nil {
			return reportAPIError(err, stderr)
		}
		rotated = true
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
	backup, err := writeCredentials(credentialsPath, credentials, *force)
	if err != nil {
		fmt.Fprintf(stderr, "write credentials: %v\n", err)
		return 1
	}
	if _, err := verifyAppKey(ctx, serverURL, app.AppID, key, *timeout); err != nil {
		fmt.Fprintf(stderr, "credentials were written, but the new app key could not be verified: %v\n", err)
		fmt.Fprintf(stderr, "keep %s and retry with: levier app check --credentials %s\n", credentialsPath, credentialsPath)
		return 1
	}

	result := appInitResult{
		AppID:             app.AppID,
		AppName:           app.Name,
		Server:            serverURL,
		Credentials:       credentialsPath,
		CredentialVersion: credentialVersion,
		KeyFingerprint:    credentialFingerprint(key),
		Created:           created,
		Rotated:           rotated,
		Backup:            backup,
	}
	return writeAppInitResult(stdout, result, *asJSON, false)
}

func verifyAppKey(ctx context.Context, server, appID, key string, timeout time.Duration) (appView, error) {
	client := newAPIClient(server, key, timeout)
	app, err := client.getApp(ctx, appID)
	if err != nil {
		return appView{}, err
	}
	if app.Status != "active" {
		return appView{}, fmt.Errorf("app status is %q, want active", app.Status)
	}
	return app, nil
}

func writeAppInitResult(stdout io.Writer, result appInitResult, asJSON, dryRun bool) int {
	if asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return 1
		}
		return 0
	}
	if dryRun {
		fmt.Fprintf(stdout, "would create app %q\n", result.AppName)
		fmt.Fprintf(stdout, "server: %s\n", result.Server)
		fmt.Fprintf(stdout, "credentials: %s\n", result.Credentials)
		return 0
	}
	switch {
	case result.Rotated:
		fmt.Fprintf(stdout, "app key rotated: %s\n", result.AppID)
	case result.Created:
		fmt.Fprintf(stdout, "app created: %s\n", result.AppID)
	default:
		fmt.Fprintf(stdout, "app exists: %s\n", result.AppID)
	}
	if result.AppName != "" {
		fmt.Fprintf(stdout, "name: %s\n", result.AppName)
	}
	if result.Server != "" {
		fmt.Fprintf(stdout, "server: %s\n", result.Server)
	}
	if result.Credentials != "" {
		fmt.Fprintf(stdout, "credentials: %s (0600)\n", result.Credentials)
	}
	if result.KeyFingerprint != "" {
		fmt.Fprintf(stdout, "key fingerprint: %s\n", result.KeyFingerprint)
	}
	if result.Backup != "" {
		fmt.Fprintf(stdout, "backup: %s\n", result.Backup)
	}
	return 0
}

func runAppShow(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("app show", flag.ContinueOnError)
	flags.SetOutput(stderr)
	credentialsPath := flags.String("credentials", os.Getenv("LEVIER_CREDENTIALS"), "credentials file")
	server := flags.String("server", "", "Levier HTTP address")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if *credentialsPath == "" {
		fmt.Fprintln(stderr, "credentials path is required: --credentials or LEVIER_CREDENTIALS")
		return 2
	}
	credentials, err := readCredentials(*credentialsPath)
	if err != nil {
		fmt.Fprintf(stderr, "read credentials: %v\n", err)
		return 2
	}
	serverURL := resolveServer(*server, &credentials)

	client := newAPIClient(serverURL, credentials.APIKey, *timeout)
	app, err := client.getApp(context.Background(), credentials.AppID)
	if err != nil {
		return reportAPIError(err, stderr)
	}

	result := appShowResult{
		appView:             app,
		Credentials:         *credentialsPath,
		CredentialVersion:   credentials.CredentialVersion,
		Server:              serverURL,
		KeyFingerprint:      credentialFingerprint(credentials.APIKey),
		CredentialCreatedAt: credentials.CreatedAt,
		CredentialUpdatedAt: credentials.UpdatedAt,
	}
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			return 1
		}
		return 0
	}
	fmt.Fprintf(stdout, "app_id              %s\n", app.AppID)
	fmt.Fprintf(stdout, "name                %s\n", app.Name)
	fmt.Fprintf(stdout, "status              %s\n", app.Status)
	fmt.Fprintf(stdout, "multiagent          %t\n", app.Multiagent)
	if app.CreatedAt != "" {
		fmt.Fprintf(stdout, "created             %s\n", app.CreatedAt)
	}
	if app.UpdatedAt != "" {
		fmt.Fprintf(stdout, "updated             %s\n", app.UpdatedAt)
	}
	fmt.Fprintf(stdout, "credentials         %s\n", *credentialsPath)
	fmt.Fprintf(stdout, "credential_version  %d\n", credentials.CredentialVersion)
	fmt.Fprintf(stdout, "server              %s\n", serverURL)
	fmt.Fprintf(stdout, "key_fingerprint     %s\n", result.KeyFingerprint)
	if credentials.UpdatedAt != "" {
		fmt.Fprintf(stdout, "credential_updated  %s\n", credentials.UpdatedAt)
	}
	return 0
}

func runAppRotateKey(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("app rotate-key", flag.ContinueOnError)
	flags.SetOutput(stderr)
	credentialsPath := flags.String("credentials", os.Getenv("LEVIER_CREDENTIALS"), "credentials file")
	server := flags.String("server", "", "Levier HTTP address")
	adminToken := flags.String("admin-token", "", "Admin token (or LEVIER_ADMIN_TOKEN)")
	yes := flags.Bool("yes", false, "confirm rotation")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if *credentialsPath == "" {
		fmt.Fprintln(stderr, "credentials path is required: --credentials or LEVIER_CREDENTIALS")
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
	credentials, err := readCredentials(*credentialsPath)
	if err != nil {
		fmt.Fprintf(stderr, "read credentials: %v\n", err)
		return 2
	}
	serverURL := resolveServer(*server, &credentials)
	if err := preflightCredentialsPath(*credentialsPath); err != nil {
		fmt.Fprintf(stderr, "credentials path is not writable: %v\n", err)
		return 2
	}
	if !requireConfirmation(*yes, fmt.Sprintf("Rotate key for %s? The old key stops working immediately.", credentials.AppID), stdout, stderr) {
		return 5
	}

	ctx := context.Background()
	client := newAPIClient(serverURL, admin, *timeout)
	key, err := client.rotateAppKey(ctx, credentials.AppID)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if key == "" {
		fmt.Fprintln(stderr, "server returned an empty app key")
		return 1
	}
	credentials.APIKey = key
	credentials.Server = serverURL
	backup, err := writeCredentials(*credentialsPath, credentials, true)
	if err != nil {
		fmt.Fprintf(stderr, "write credentials: %v\n", err)
		return 1
	}
	if _, err := verifyAppKey(ctx, serverURL, credentials.AppID, key, *timeout); err != nil {
		fmt.Fprintf(stderr, "credentials were written, but the new key could not be verified: %v\n", err)
		fmt.Fprintf(stderr, "keep %s and retry with: levier app check --credentials %s\n", *credentialsPath, *credentialsPath)
		return 1
	}
	result := appInitResult{
		AppID:             credentials.AppID,
		AppName:           credentials.AppName,
		Server:            serverURL,
		Credentials:       *credentialsPath,
		CredentialVersion: credentialVersion,
		KeyFingerprint:    credentialFingerprint(key),
		Rotated:           true,
		Backup:            backup,
	}
	return writeAppInitResult(stdout, result, *asJSON, false)
}

func requireConfirmation(yes bool, prompt string, stdout, stderr io.Writer) bool {
	if yes {
		return true
	}
	if !isTerminal(os.Stdin) {
		fmt.Fprintln(stderr, "refusing without --yes in a non-interactive session")
		return false
	}
	return confirm(os.Stdin, stdout, prompt)
}

func reportAPIError(err error, stderr io.Writer) int {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		fmt.Fprintf(stderr, "error: %s\n", apiErr.Error())
		return exitCodeForStatus(apiErr.Status)
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	return 4
}

func exitCodeForStatus(status int) int {
	switch {
	case status == 401 || status == 403:
		return 3
	case status == 400 || status == 404 || status == 409 || status == 422:
		return 2
	case status == 429 || status >= 500:
		return 4
	default:
		return 1
	}
}
