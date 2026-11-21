package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"
)

type appCheckItem struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type appCheckResult struct {
	OK             bool           `json:"ok"`
	Credentials    string         `json:"credentials"`
	Server         string         `json:"server"`
	AppID          string         `json:"app_id,omitempty"`
	AppName        string         `json:"app_name,omitempty"`
	KeyFingerprint string         `json:"key_fingerprint,omitempty"`
	Checks         []appCheckItem `json:"checks"`
}

func runAppCheck(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("app check", flag.ContinueOnError)
	flags.SetOutput(stderr)
	credentialsPath := flags.String("credentials", defaultCredentialsValue(), "credentials file")
	server := flags.String("server", "", "Levier HTTP address")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "app check: unexpected argument: %s\n\n", flags.Arg(0))
		printAppUsage(stderr)
		return 2
	}
	if *credentialsPath == "" {
		fmt.Fprintln(stderr, "credentials path is required: pass --credentials, set LEVIER_CREDENTIALS, or run levier setup --credentials <path>")
		return 2
	}

	credentials, err := readCredentials(*credentialsPath)
	if err != nil {
		fmt.Fprintf(stderr, "read credentials: %v\n", err)
		return 2
	}
	serverURL := resolveServer(*server, &credentials)
	result := appCheckResult{
		Credentials:    *credentialsPath,
		Server:         serverURL,
		AppID:          credentials.AppID,
		AppName:        credentials.AppName,
		KeyFingerprint: credentialFingerprint(credentials.APIKey),
	}
	result.add("credentials", "ok", fmt.Sprintf("%s (version %d)", *credentialsPath, credentials.CredentialVersion))

	ctx := context.Background()
	client := newAPIClient(serverURL, credentials.APIKey, *timeout)
	appOK := false
	app, err := client.getApp(ctx, credentials.AppID)
	if err != nil {
		result.add("app", "fail", err.Error())
	} else {
		result.AppName = app.Name
		if app.Status != "active" {
			result.add("app", "fail", fmt.Sprintf("status is %s, want active", app.Status))
		} else {
			appOK = true
			result.add("app", "ok", fmt.Sprintf("%s %s", app.AppID, app.Name))
		}
	}

	if appOK {
		ws, err := client.issueWSToken(ctx)
		if err != nil {
			result.add("websocket", "fail", err.Error())
		} else if ws.WSToken == "" || ws.WSURL == "" {
			result.add("websocket", "fail", "server returned an incomplete WebSocket token")
		} else {
			result.add("websocket", "ok", fmt.Sprintf("%s expires=%ds", ws.WSURL, ws.ExpiresIn))
		}
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintf(stderr, "encode app check: %v\n", err)
			return 1
		}
	} else {
		fmt.Fprintf(stdout, "app check: %s\n", serverURL)
		for _, check := range result.Checks {
			fmt.Fprintf(stdout, "%-12s %-4s %s\n", check.Name, check.Status, check.Message)
		}
		if result.OK {
			fmt.Fprintln(stdout, "app check: PASS")
		} else {
			fmt.Fprintln(stdout, "app check: FAIL")
		}
	}
	if !result.OK {
		return 1
	}
	return 0
}

func (r *appCheckResult) add(name, status, message string) {
	r.Checks = append(r.Checks, appCheckItem{Name: name, Status: status, Message: message})
	if status == "fail" {
		r.OK = false
	}
	if len(r.Checks) == 1 && name == "credentials" && status == "ok" {
		r.OK = true
	}
}
