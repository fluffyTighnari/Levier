package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSecretFiles(t *testing.T) {
	dir := t.TempDir()
	adminPath := filepath.Join(dir, "admin-token")
	daemonPath := filepath.Join(dir, "daemon-tokens.json")
	urlPath := filepath.Join(dir, "database-url")

	writeFile(t, adminPath, "admin-token-from-file", 0o600)
	writeFile(t, daemonPath, `{"daemon-local-1":"daemon-token-from-file"}`, 0o600)
	writeFile(t, urlPath, "postgres://example-user:example-pass@localhost:5432/levier?sslmode=disable", 0o600)

	yaml := `
config_version: 1
environment: development
http:
  admin_token_file: ` + yamlPath(adminPath) + `
grpc:
  daemon_tokens_file: ` + yamlPath(daemonPath) + `
database:
  url_file: ` + yamlPath(urlPath) + `
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.HTTP.AdminToken != "admin-token-from-file" {
		t.Fatalf("AdminToken = %q", cfg.HTTP.AdminToken)
	}
	if cfg.GRPC.DaemonTokens["daemon-local-1"] != "daemon-token-from-file" {
		t.Fatalf("DaemonTokens = %+v", cfg.GRPC.DaemonTokens)
	}
	if !strings.Contains(cfg.Database.URL, "example-user") {
		t.Fatalf("Database.URL = %q", cfg.Database.URL)
	}
}

func TestParseEnvOverridesSecretFiles(t *testing.T) {
	dir := t.TempDir()
	adminPath := filepath.Join(dir, "admin-token")
	urlPath := filepath.Join(dir, "database-url")
	writeFile(t, adminPath, "file-admin-token", 0o600)
	writeFile(t, urlPath, "postgres://example-user:file-pass@localhost:5432/levier", 0o600)
	t.Setenv("LEVIER_HTTP_ADMIN_TOKEN", "env-admin-token")
	t.Setenv("LEVIER_DATABASE_URL", "postgres://example-user:env-pass@localhost:5432/levier")

	yaml := `
http:
  admin_token_file: ` + yamlPath(adminPath) + `
database:
  url_file: ` + yamlPath(urlPath) + `
`
	cfg, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.HTTP.AdminToken != "env-admin-token" {
		t.Fatalf("AdminToken = %q", cfg.HTTP.AdminToken)
	}
	if !strings.Contains(cfg.Database.URL, "env-pass") {
		t.Fatalf("Database.URL = %q", cfg.Database.URL)
	}
}

func TestApplyEnvCoversControlPlane(t *testing.T) {
	cfg := Default()
	cfg.Database.URL = "postgres://example-user:example-pass@localhost/levier"
	env := map[string]string{
		"LEVIER_ENVIRONMENT":              "production",
		"LEVIER_HTTP_READ_TIMEOUT":        "7s",
		"LEVIER_HTTP_WRITE_TIMEOUT":       "8s",
		"LEVIER_HTTP_SHUTDOWN_TIMEOUT":    "9s",
		"LEVIER_HTTP_WS_TOKEN_TTL":        "11m",
		"LEVIER_GRPC_MAX_RECV_MSG_BYTES":  "1024",
		"LEVIER_GRPC_MAX_SEND_MSG_BYTES":  "2048",
		"LEVIER_GRPC_SHUTDOWN_TIMEOUT":    "10s",
		"LEVIER_DATABASE_MAX_CONNS":       "30",
		"LEVIER_DATABASE_MIN_CONNS":       "3",
		"LEVIER_DATABASE_CONNECT_TIMEOUT": "4s",
		"LEVIER_DATABASE_HEALTH_TIMEOUT":  "5s",
		"LEVIER_HTTP_ADMIN_TOKEN_FILE":    "/run/secrets/admin",
		"LEVIER_GRPC_DAEMON_TOKENS_FILE":  "/run/secrets/daemons.json",
		"LEVIER_DATABASE_URL_FILE":        "/run/secrets/database",
		"LEVIER_HTTP_BEARER_TOKEN":        "",
		"LEVIER_HTTP_ALLOW_LEGACY_BEARER": "true",
		"LEVIER_WORKFLOW_EVENT_BUFFER":    "512",
		"LEVIER_LOG_FORMAT":               "text",
	}
	if err := ApplyEnv(&cfg, func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}); err != nil {
		t.Fatalf("ApplyEnv() error = %v", err)
	}

	if cfg.Environment != "production" {
		t.Fatalf("Environment = %q", cfg.Environment)
	}
	if cfg.HTTP.ReadTimeout.Std().String() != "7s" || cfg.HTTP.WriteTimeout.Std().String() != "8s" {
		t.Fatalf("HTTP timeouts = %v/%v", cfg.HTTP.ReadTimeout, cfg.HTTP.WriteTimeout)
	}
	if cfg.HTTP.ShutdownTimeout.Std().String() != "9s" || cfg.HTTP.WSTokenTTL.Std().String() != "11m0s" {
		t.Fatalf("HTTP timeouts = %v/%v", cfg.HTTP.ShutdownTimeout, cfg.HTTP.WSTokenTTL)
	}
	if cfg.GRPC.MaxRecvMsgBytes != 1024 || cfg.GRPC.MaxSendMsgBytes != 2048 {
		t.Fatalf("gRPC message bytes = %d/%d", cfg.GRPC.MaxRecvMsgBytes, cfg.GRPC.MaxSendMsgBytes)
	}
	if cfg.Database.MaxConns != 30 || cfg.Database.MinConns != 3 {
		t.Fatalf("database conns = %d/%d", cfg.Database.MaxConns, cfg.Database.MinConns)
	}
	if cfg.Database.ConnectTimeout.Std().String() != "4s" || cfg.Database.HealthTimeout.Std().String() != "5s" {
		t.Fatalf("database timeouts = %v/%v", cfg.Database.ConnectTimeout, cfg.Database.HealthTimeout)
	}
	if cfg.HTTP.AdminTokenFile != "/run/secrets/admin" || cfg.GRPC.DaemonTokensFile != "/run/secrets/daemons.json" {
		t.Fatalf("secret file paths = %q/%q", cfg.HTTP.AdminTokenFile, cfg.GRPC.DaemonTokensFile)
	}
	if cfg.Log.Format != "text" {
		t.Fatalf("Log.Format = %q", cfg.Log.Format)
	}
}

func TestValidateProductionRejectsPlaceholders(t *testing.T) {
	cfg := Default()
	cfg.Environment = "production"
	cfg.HTTP.AdminToken = "admin-change-me"
	cfg.GRPC.DaemonTokens = map[string]string{"daemon-local-1": "change-me"}
	cfg.Database.URL = "postgres://change-me:change-me@localhost:5432/levier"

	err := cfg.ValidateProduction()
	if err == nil {
		t.Fatal("ValidateProduction() error = nil")
	}
	for _, want := range []string{"admin_token", "daemon_tokens", "database.url"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

func TestValidateProductionAcceptsSecrets(t *testing.T) {
	cfg := Default()
	cfg.Environment = "production"
	cfg.HTTP.AdminToken = "real-admin-token"
	cfg.GRPC.DaemonTokens = map[string]string{"daemon-local-1": "real-daemon-token"}
	cfg.Database.URL = "postgres://example-user:example-pass@db:5432/levier"
	if err := cfg.ValidateProduction(); err != nil {
		t.Fatalf("ValidateProduction() error = %v", err)
	}
}

func TestValidateRejectsConfigVersionAndEnvironment(t *testing.T) {
	cfg := Default()
	cfg.Database.URL = "postgres://example-user:example-pass@localhost/levier"
	cfg.ConfigVersion = 2
	cfg.Environment = "staging"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "config_version") || !strings.Contains(err.Error(), "environment") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func yamlPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "''") + "'"
}
