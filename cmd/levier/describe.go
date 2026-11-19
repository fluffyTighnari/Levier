// Levier — describe command.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/fluffyTighnari/levier/internal/selfdesc"
)

func runDescribe(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("describe", flag.ContinueOnError)
	server := flags.String("server", envOr("LEVIER_SERVER", "http://127.0.0.1:8080"), "Levier HTTP address")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 5*time.Second, "request timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}

	ctx := context.Background()
	client := &http.Client{Timeout: *timeout}
	code, body, err := httpGet(ctx, client, serverBase(*server)+"/meta", "")
	if err != nil {
		fmt.Fprintf(stderr, "describe failed: %v\n", err)
		return 1
	}
	if code != http.StatusOK {
		fmt.Fprintf(stderr, "describe failed: GET /meta returned HTTP %d\n", code)
		return 1
	}

	var doc selfdesc.Document
	if err := json.Unmarshal(body, &doc); err != nil {
		fmt.Fprintf(stderr, "describe failed: invalid /meta JSON: %v\n", err)
		return 1
	}
	if doc.Service != selfdesc.ServiceName {
		fmt.Fprintf(stderr, "describe failed: unexpected service %q\n", doc.Service)
		return 1
	}
	if doc.MetaVersion != selfdesc.MetaVersion {
		fmt.Fprintf(stderr, "describe failed: unsupported meta_version %d (client supports %d)\n",
			doc.MetaVersion, selfdesc.MetaVersion)
		return 1
	}

	if *asJSON {
		fmt.Fprintln(stdout, strings.TrimSpace(string(body)))
		return 0
	}

	fmt.Fprintf(stdout, "service      %s\n", doc.Service)
	fmt.Fprintf(stdout, "version      %s\n", doc.Build.Version)
	fmt.Fprintf(stdout, "commit       %s\n", doc.Build.Commit)
	fmt.Fprintf(stdout, "api          %s\n", doc.API.HTTPVersion)
	fmt.Fprintf(stdout, "ws protocol  %d\n", doc.API.WSProtocol)
	fmt.Fprintf(stdout, "runtime      %s\n", strings.Join(doc.Capabilities.RuntimeTypes, ", "))

	fmt.Fprintf(stdout, "limits       body=%d run=%dms tool=%dms ws_token=%ds\n",
		doc.Limits.MaxBodyBytes,
		doc.Limits.DefaultRunTimeoutMS,
		doc.Limits.DefaultToolTimeoutMS,
		doc.Limits.WSTokenTTLSeconds,
	)
	fmt.Fprintf(stdout, "endpoints    health, ready, metrics, runs, apps, websocket\n")
	return 0
}
