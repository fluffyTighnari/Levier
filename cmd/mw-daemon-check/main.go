// Levier — daemon protocol conformance check.
//
// Connects as a RuntimeChannel daemon, accepts one TaskAssign, emits events,
// then verifies cancel handling against a real Levier instance.
//
// Usage:
//
//	go run ./cmd/mw-daemon-check
//	go run ./cmd/mw-daemon-check --grpc 127.0.0.1:9090 --http http://127.0.0.1:8080
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/fluffyTighnari/levier/internal/daemoncheck"
)

func main() {
	cfg := daemoncheck.Config{}
	jsonOut := false
	flag.StringVar(&cfg.GRPCAddr, "grpc", "127.0.0.1:9090", "Levier gRPC address")
	flag.StringVar(&cfg.HTTPURL, "http", "http://127.0.0.1:8080", "Levier HTTP address")
	flag.StringVar(&cfg.DaemonID, "daemon-id", "daemon-local-1", "pre-provisioned daemon id")
	flag.StringVar(&cfg.Token, "token", "change-me", "daemon auth token")
	flag.StringVar(&cfg.AdminToken, "admin-token", "admin-change-me", "admin token for app bootstrap")
	flag.StringVar(&cfg.RuntimeName, "runtime", "echo1", "runtime name to advertise")
	flag.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "per-step timeout")
	flag.BoolVar(&jsonOut, "json", false, "print JSON result")
	flag.Parse()

	checks, err := daemoncheck.Run(cfg)
	result := map[string]any{"ok": err == nil, "checks": checks}
	if err != nil {
		result["error"] = err.Error()
	}
	if jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(result)
	} else {
		encoded, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(encoded))
	}
	if err != nil {
		os.Exit(1)
	}
}
