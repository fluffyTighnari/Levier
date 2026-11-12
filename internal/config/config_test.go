// Middleware — internal/config
//
// Configuration loading and validation tests.
//
// Copyright 2026 The Middleware Authors.
package config

import (
	"strings"
	"testing"
	"time"
)

const validYAML = `
http:
  addr: ":18080"
  read_timeout: "5s"
grpc:
  addr: ":19090"
database:
  url: "postgres://change-me:change-me@localhost:5432/middleware?sslmode=disable"
log:
  level: "debug"
  format: "text"
`

func TestParseDefaultsAndOverrides(t *testing.T) {
	cfg, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if cfg.HTTP.Addr != ":18080" {
		t.Fatalf("HTTP.Addr = %q", cfg.HTTP.Addr)
	}
	if cfg.HTTP.ReadTimeout.Std() != 5*time.Second {
		t.Fatalf("HTTP.ReadTimeout = %v", cfg.HTTP.ReadTimeout.Std())
	}
	if cfg.HTTP.WriteTimeout.Std() != 30*time.Second {
		t.Fatalf("HTTP.WriteTimeout default = %v", cfg.HTTP.WriteTimeout.Std())
	}
	if cfg.HTTP.AllowLegacyBearer {
		t.Fatal("AllowLegacyBearer default = true, want false")
	}
	if cfg.HTTP.WSTokenTTL.Std() != 10*time.Minute {
		t.Fatalf("WSTokenTTL default = %v", cfg.HTTP.WSTokenTTL.Std())
	}
	if cfg.Workflow.Workers != 4 {
		t.Fatalf("Workflow.Workers default = %d", cfg.Workflow.Workers)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Format != "text" {
		t.Fatalf("Log = %+v", cfg.Log)
	}
}

func TestParseDaemonTokens(t *testing.T) {
	cfg, err := Parse([]byte(strings.Replace(validYAML,
		"grpc:\n  addr: \":19090\"",
		"grpc:\n  addr: \":19090\"\n  daemon_tokens:\n    daemon-1: \"token-1\"",
		1,
	)))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.GRPC.DaemonTokens["daemon-1"] != "token-1" {
		t.Fatalf("DaemonTokens = %+v", cfg.GRPC.DaemonTokens)
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	_, err := Parse([]byte(validYAML + "\nunknown: true\n"))
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestApplyEnv(t *testing.T) {
	cfg := Default()
	cfg.Database.URL = "postgres://localhost/middleware"
	env := map[string]string{
		"MIDDLEWARE_HTTP_ADDR":                ":28080",
		"MIDDLEWARE_WORKFLOW_WORKERS":         "9",
		"MIDDLEWARE_WORKFLOW_LEASE_TTL":       "45s",
		"MIDDLEWARE_WORKFLOW_MAX_ATTEMPTS":    "7",
		"MIDDLEWARE_WORKFLOW_REAP_INTERVAL":   "15s",
		"MIDDLEWARE_WORKFLOW_REAP_LIMIT":      "25",
		"MIDDLEWARE_WORKFLOW_RECOVERY_DELAY":  "12s",
		"MIDDLEWARE_WORKFLOW_EVENT_BUFFER":    "128",
		"MIDDLEWARE_LOG_LEVEL":                "warn",
		"MIDDLEWARE_HTTP_ALLOW_LEGACY_BEARER": "true",
	}

	if err := ApplyEnv(&cfg, func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}); err != nil {
		t.Fatalf("ApplyEnv() error = %v", err)
	}

	if cfg.HTTP.Addr != ":28080" {
		t.Fatalf("HTTP.Addr = %q", cfg.HTTP.Addr)
	}
	if cfg.Workflow.Workers != 9 || cfg.Workflow.MaxAttempts != 7 {
		t.Fatalf("Workflow = %+v", cfg.Workflow)
	}
	if cfg.Workflow.LeaseTTL.Std() != 45*time.Second {
		t.Fatalf("LeaseTTL = %v", cfg.Workflow.LeaseTTL.Std())
	}
	if cfg.Workflow.ReapInterval.Std() != 15*time.Second || cfg.Workflow.ReapLimit != 25 {
		t.Fatalf("reaper = interval %v limit %d", cfg.Workflow.ReapInterval.Std(), cfg.Workflow.ReapLimit)
	}
	if cfg.Workflow.RecoveryDelay.Std() != 12*time.Second {
		t.Fatalf("recovery delay = %v", cfg.Workflow.RecoveryDelay.Std())
	}
	if cfg.Workflow.EventBuffer != 128 {
		t.Fatalf("event buffer = %d", cfg.Workflow.EventBuffer)
	}
	if cfg.Log.Level != "warn" {
		t.Fatalf("Log.Level = %q", cfg.Log.Level)
	}
	if !cfg.HTTP.AllowLegacyBearer {
		t.Fatal("AllowLegacyBearer was not applied")
	}
}

func TestApplyEnvRejectsBadValues(t *testing.T) {
	cfg := Default()
	cfg.Database.URL = "postgres://localhost/middleware"
	err := ApplyEnv(&cfg, func(key string) (string, bool) {
		if key == "MIDDLEWARE_WORKFLOW_WORKERS" {
			return "many", true
		}
		return "", false
	})
	if err == nil || !strings.Contains(err.Error(), "invalid integer") {
		t.Fatalf("ApplyEnv() error = %v", err)
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"missing database", func(c *Config) { c.Database.URL = "" }, "database.url is required"},
		{"bad http addr", func(c *Config) { c.HTTP.Addr = "localhost" }, "http.addr"},
		{"bad grpc port", func(c *Config) { c.GRPC.Addr = ":70000" }, "grpc.addr"},
		{"bad log level", func(c *Config) { c.Log.Level = "verbose" }, "log.level"},
		{"bad min conns", func(c *Config) { c.Database.MinConns = 100 }, "database.min_conns"},
		{"zero workers", func(c *Config) { c.Workflow.Workers = 0 }, "workflow.workers"},
		{"zero reap interval", func(c *Config) { c.Workflow.ReapInterval = 0 }, "workflow.reap_interval"},
		{"zero reap limit", func(c *Config) { c.Workflow.ReapLimit = 0 }, "workflow.reap_limit"},
		{"zero recovery delay", func(c *Config) { c.Workflow.RecoveryDelay = 0 }, "workflow.recovery_delay"},
		{"zero event buffer", func(c *Config) { c.Workflow.EventBuffer = 0 }, "workflow.event_buffer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			cfg.Database.URL = "postgres://localhost/middleware"
			tt.mutate(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestDurationUnmarshal(t *testing.T) {
	cfg, err := Parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.Workflow.PollInterval.Std() != 200*time.Millisecond {
		t.Fatalf("PollInterval = %v", cfg.Workflow.PollInterval.Std())
	}
}
