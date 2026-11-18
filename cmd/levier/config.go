package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/fluffyTighnari/levier/internal/config"
)

type configCheckResult struct {
	OK          bool     `json:"ok"`
	Config      string   `json:"config"`
	Version     int      `json:"version"`
	Environment string   `json:"environment"`
	Warnings    []string `json:"warnings,omitempty"`
	Error       string   `json:"error,omitempty"`
}

func runConfig(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printConfigUsage(stderr)
		return 2
	}
	switch args[0] {
	case "check":
		return runConfigCheck(args[1:], stdout, stderr)
	case "print":
		return runConfigPrint(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printConfigUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown config command: %s\n\n", args[0])
		printConfigUsage(stderr)
		return 2
	}
}

func printConfigUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier config check [--config path] [--production] [--json]")
	fmt.Fprintln(out, "  levier config print [--config path] [--json]")
}

func runConfigCheck(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config check", flag.ContinueOnError)
	path := flags.String("config", envOr("LEVIER_CONFIG", "levier.example.yaml"), "path to YAML configuration")
	production := flags.Bool("production", false, "enforce production secret requirements")
	asJSON := flags.Bool("json", false, "print JSON")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}

	result := configCheckResult{Config: *path}
	cfg, err := config.Load(*path)
	if err != nil {
		result.Error = err.Error()
	} else {
		result.Version = cfg.ConfigVersion
		result.Environment = cfg.Environment
		result.Warnings = cfg.SecretFileWarnings()
		if *production {
			if err := cfg.ValidateProduction(); err != nil {
				result.Error = err.Error()
			}
		}
	}
	result.OK = result.Error == ""

	if *asJSON {
		return encodeJSONResult(stdout, stderr, result)
	}
	fmt.Fprintf(stdout, "config:      %s\n", result.Config)
	fmt.Fprintf(stdout, "version:     %d\n", result.Version)
	fmt.Fprintf(stdout, "environment: %s\n", result.Environment)
	if result.OK {
		fmt.Fprintln(stdout, "status:      ok")
	} else {
		fmt.Fprintln(stdout, "status:      fail")
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(stdout, "warning:     %s\n", warning)
	}
	if result.Error != "" {
		fmt.Fprintf(stderr, "config error: %s\n", result.Error)
	}
	if !result.OK {
		return 1
	}
	return 0
}

func runConfigPrint(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("config print", flag.ContinueOnError)
	path := flags.String("config", envOr("LEVIER_CONFIG", "levier.example.yaml"), "path to YAML configuration")
	asJSON := flags.Bool("json", false, "print JSON")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}

	cfg, err := config.Load(*path)
	if err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return 1
	}
	view := redactedConfig(cfg)
	if *asJSON {
		return encodeJSONResult(stdout, stderr, view)
	}
	encoded, err := yaml.Marshal(view)
	if err != nil {
		fmt.Fprintf(stderr, "encode config: %v\n", err)
		return 1
	}
	_, _ = stdout.Write(encoded)
	return 0
}

func encodeJSONResult(out, stderr io.Writer, value any) int {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "encode config: %v\n", err)
		return 1
	}
	return 0
}

func redactedConfig(cfg config.Config) map[string]any {
	daemonTokens := make(map[string]string, len(cfg.GRPC.DaemonTokens))
	for id, token := range cfg.GRPC.DaemonTokens {
		if token == "" {
			daemonTokens[id] = ""
		} else {
			daemonTokens[id] = "***"
		}
	}
	return map[string]any{
		"config_version": cfg.ConfigVersion,
		"environment":    cfg.Environment,
		"http": map[string]any{
			"addr":                cfg.HTTP.Addr,
			"read_timeout":        cfg.HTTP.ReadTimeout.Std().String(),
			"write_timeout":       cfg.HTTP.WriteTimeout.Std().String(),
			"shutdown_timeout":    cfg.HTTP.ShutdownTimeout.Std().String(),
			"bearer_token":        redactSecret(cfg.HTTP.BearerToken),
			"bearer_token_file":   cfg.HTTP.BearerTokenFile,
			"admin_token":         redactSecret(cfg.HTTP.AdminToken),
			"admin_token_file":    cfg.HTTP.AdminTokenFile,
			"allow_legacy_bearer": cfg.HTTP.AllowLegacyBearer,
			"ws_token_ttl":        cfg.HTTP.WSTokenTTL.Std().String(),
		},
		"grpc": map[string]any{
			"addr":               cfg.GRPC.Addr,
			"max_recv_msg_bytes": cfg.GRPC.MaxRecvMsgBytes,
			"max_send_msg_bytes": cfg.GRPC.MaxSendMsgBytes,
			"shutdown_timeout":   cfg.GRPC.ShutdownTimeout.Std().String(),
			"daemon_tokens":      daemonTokens,
			"daemon_tokens_file": cfg.GRPC.DaemonTokensFile,
		},
		"database": map[string]any{
			"url":             redactDatabaseURL(cfg.Database.URL),
			"url_file":        cfg.Database.URLFile,
			"max_conns":       cfg.Database.MaxConns,
			"min_conns":       cfg.Database.MinConns,
			"connect_timeout": cfg.Database.ConnectTimeout.Std().String(),
			"health_timeout":  cfg.Database.HealthTimeout.Std().String(),
		},
		"workflow": map[string]any{
			"workers":        cfg.Workflow.Workers,
			"lease_ttl":      cfg.Workflow.LeaseTTL.Std().String(),
			"poll_interval":  cfg.Workflow.PollInterval.Std().String(),
			"max_attempts":   cfg.Workflow.MaxAttempts,
			"reap_interval":  cfg.Workflow.ReapInterval.Std().String(),
			"reap_limit":     cfg.Workflow.ReapLimit,
			"recovery_delay": cfg.Workflow.RecoveryDelay.Std().String(),
			"event_buffer":   cfg.Workflow.EventBuffer,
		},
		"log": map[string]any{
			"level":  cfg.Log.Level,
			"format": cfg.Log.Format,
		},
	}
}

func redactSecret(value string) string {
	if value == "" {
		return ""
	}
	return "***"
}
