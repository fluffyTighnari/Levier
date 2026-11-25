// Package config loads and validates middleware runtime configuration.
//
// Levier — internal/config
// Copyright 2026 The Levier Authors.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fluffyTighnari/levier/internal/roles"
)

// Config is the complete Levier runtime configuration.
type Config struct {
	ConfigVersion int             `yaml:"config_version"`
	Environment   string          `yaml:"environment"`
	HTTP          HTTPConfig      `yaml:"http"`
	GRPC          GRPCConfig      `yaml:"grpc"`
	Gateway       GatewayConfig   `yaml:"gateway"`
	Database      DatabaseConfig  `yaml:"database"`
	Workflow      WorkflowConfig  `yaml:"workflow"`
	Retention     RetentionConfig `yaml:"retention"`
	Log           LogConfig       `yaml:"log"`
}

// HTTPConfig configures the App-facing HTTP server.
type HTTPConfig struct {
	Addr              string   `yaml:"addr"`
	ReadTimeout       Duration `yaml:"read_timeout"`
	WriteTimeout      Duration `yaml:"write_timeout"`
	ShutdownTimeout   Duration `yaml:"shutdown_timeout"`
	BearerToken       string   `yaml:"bearer_token"`
	BearerTokenFile   string   `yaml:"bearer_token_file"`
	AdminToken        string   `yaml:"admin_token"`
	AdminTokenFile    string   `yaml:"admin_token_file"`
	AllowLegacyBearer bool     `yaml:"allow_legacy_bearer"`
	WSTokenTTL        Duration `yaml:"ws_token_ttl"`
	WSTokenSecret     string   `yaml:"ws_token_secret"`
}

// GRPCConfig configures the daemon-facing gRPC server.
type GRPCConfig struct {
	Addr             string            `yaml:"addr"`
	MaxRecvMsgBytes  int               `yaml:"max_recv_msg_bytes"`
	MaxSendMsgBytes  int               `yaml:"max_send_msg_bytes"`
	ShutdownTimeout  Duration          `yaml:"shutdown_timeout"`
	DaemonTokens     map[string]string `yaml:"daemon_tokens"`
	DaemonTokensFile string            `yaml:"daemon_tokens_file"`
}

// GatewayConfig identifies one control / gateway process and its presence lease.
type GatewayConfig struct {
	InstanceID        string   `yaml:"instance_id"`
	Roles             []string `yaml:"roles"`
	HeartbeatInterval Duration `yaml:"heartbeat_interval"`
	LeaseTTL          Duration `yaml:"lease_ttl"`
}

// DatabaseConfig configures the PostgreSQL connection pool.
type DatabaseConfig struct {
	URL            string   `yaml:"url"`
	URLFile        string   `yaml:"url_file"`
	MaxConns       int32    `yaml:"max_conns"`
	MinConns       int32    `yaml:"min_conns"`
	ConnectTimeout Duration `yaml:"connect_timeout"`
	HealthTimeout  Duration `yaml:"health_timeout"`
}

// WorkflowConfig configures the RunTaskQueue worker runtime.
type WorkflowConfig struct {
	Workers       int      `yaml:"workers"`
	LeaseTTL      Duration `yaml:"lease_ttl"`
	PollInterval  Duration `yaml:"poll_interval"`
	MaxAttempts   int      `yaml:"max_attempts"`
	ReapInterval  Duration `yaml:"reap_interval"`
	ReapLimit     int      `yaml:"reap_limit"`
	RecoveryDelay Duration `yaml:"recovery_delay"`
	EventBuffer   int      `yaml:"event_buffer"`
}

// RetentionConfig configures offline data cleanup. It is disabled by default.
// Deleting a terminal run also deletes its events, tasks, and tool invocations
// through database foreign-key cascades.
type RetentionConfig struct {
	Enabled     bool `yaml:"enabled"`
	RunDays     int  `yaml:"run_days"`
	SessionDays int  `yaml:"session_days"`
	BatchSize   int  `yaml:"batch_size"`
}

// LogConfig configures structured logging.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Default returns the baseline configuration used before file/env overrides.
func Default() Config {
	return Config{
		ConfigVersion: 1,
		Environment:   "development",
		HTTP: HTTPConfig{
			Addr:            ":8080",
			ReadTimeout:     Duration(15 * time.Second),
			WriteTimeout:    Duration(30 * time.Second),
			ShutdownTimeout: Duration(10 * time.Second),
			WSTokenTTL:      Duration(10 * time.Minute),
		},
		GRPC: GRPCConfig{
			Addr:            ":9090",
			MaxRecvMsgBytes: 16 << 20,
			MaxSendMsgBytes: 16 << 20,
			ShutdownTimeout: Duration(10 * time.Second),
		},
		Gateway: GatewayConfig{
			InstanceID:        defaultGatewayInstanceID(),
			Roles:             []string{roles.All},
			HeartbeatInterval: Duration(10 * time.Second),
			LeaseTTL:          Duration(30 * time.Second),
		},
		Database: DatabaseConfig{
			MaxConns:       20,
			MinConns:       2,
			ConnectTimeout: Duration(5 * time.Second),
			HealthTimeout:  Duration(2 * time.Second),
		},
		Workflow: WorkflowConfig{
			Workers:       4,
			LeaseTTL:      Duration(30 * time.Second),
			PollInterval:  Duration(200 * time.Millisecond),
			MaxAttempts:   5,
			ReapInterval:  Duration(10 * time.Second),
			ReapLimit:     100,
			RecoveryDelay: Duration(10 * time.Second),
			EventBuffer:   256,
		},
		Retention: RetentionConfig{
			Enabled:     false,
			RunDays:     30,
			SessionDays: 7,
			BatchSize:   1000,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load reads, parses, applies env overrides, and validates a YAML file.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	return Parse(data)
}

// Parse parses YAML, applies LEVIER_* env overrides, and validates.
func Parse(data []byte) (Config, error) {
	cfg := Default()

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	if err := ApplyEnv(&cfg, os.LookupEnv); err != nil {
		return Config{}, err
	}
	if err := resolveSecretFiles(&cfg, os.LookupEnv); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	if strings.EqualFold(cfg.Environment, "production") {
		if err := cfg.ValidateProduction(); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

// ApplyEnv applies the supported LEVIER_* environment overrides.
func ApplyEnv(cfg *Config, lookup func(string) (string, bool)) error {
	if lookup == nil {
		return nil
	}

	stringOverride(lookup, "LEVIER_ENVIRONMENT", &cfg.Environment)
	if !envPresent(lookup, "LEVIER_ENVIRONMENT") {
		stringOverride(lookup, "LEVIER_ENV", &cfg.Environment)
	}
	stringOverride(lookup, "LEVIER_HTTP_ADDR", &cfg.HTTP.Addr)
	stringOverride(lookup, "LEVIER_HTTP_BEARER_TOKEN", &cfg.HTTP.BearerToken)
	stringOverride(lookup, "LEVIER_HTTP_BEARER_TOKEN_FILE", &cfg.HTTP.BearerTokenFile)
	stringOverride(lookup, "LEVIER_HTTP_ADMIN_TOKEN", &cfg.HTTP.AdminToken)
	stringOverride(lookup, "LEVIER_HTTP_ADMIN_TOKEN_FILE", &cfg.HTTP.AdminTokenFile)
	if err := boolOverride(lookup, "LEVIER_HTTP_ALLOW_LEGACY_BEARER", &cfg.HTTP.AllowLegacyBearer); err != nil {
		return err
	}
	stringOverride(lookup, "LEVIER_HTTP_WS_TOKEN_SECRET", &cfg.HTTP.WSTokenSecret)
	stringOverride(lookup, "LEVIER_GATEWAY_INSTANCE_ID", &cfg.Gateway.InstanceID)
	if value, ok := lookupEnv(lookup, "LEVIER_GATEWAY_ROLES"); ok {
		cfg.Gateway.Roles = splitList(value)
	}
	stringOverride(lookup, "LEVIER_GRPC_ADDR", &cfg.GRPC.Addr)
	stringOverride(lookup, "LEVIER_GRPC_DAEMON_TOKENS_FILE", &cfg.GRPC.DaemonTokensFile)
	stringOverride(lookup, "LEVIER_DATABASE_URL", &cfg.Database.URL)
	stringOverride(lookup, "LEVIER_DATABASE_URL_FILE", &cfg.Database.URLFile)
	stringOverride(lookup, "LEVIER_LOG_LEVEL", &cfg.Log.Level)
	stringOverride(lookup, "LEVIER_LOG_FORMAT", &cfg.Log.Format)

	if err := durationOverride(lookup, "LEVIER_HTTP_READ_TIMEOUT", &cfg.HTTP.ReadTimeout); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_HTTP_WRITE_TIMEOUT", &cfg.HTTP.WriteTimeout); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_HTTP_SHUTDOWN_TIMEOUT", &cfg.HTTP.ShutdownTimeout); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_HTTP_WS_TOKEN_TTL", &cfg.HTTP.WSTokenTTL); err != nil {
		return err
	}
	if err := intOverride(lookup, "LEVIER_GRPC_MAX_RECV_MSG_BYTES", &cfg.GRPC.MaxRecvMsgBytes); err != nil {
		return err
	}
	if err := intOverride(lookup, "LEVIER_GRPC_MAX_SEND_MSG_BYTES", &cfg.GRPC.MaxSendMsgBytes); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_GRPC_SHUTDOWN_TIMEOUT", &cfg.GRPC.ShutdownTimeout); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_GATEWAY_HEARTBEAT_INTERVAL", &cfg.Gateway.HeartbeatInterval); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_GATEWAY_LEASE_TTL", &cfg.Gateway.LeaseTTL); err != nil {
		return err
	}
	if err := int32Override(lookup, "LEVIER_DATABASE_MAX_CONNS", &cfg.Database.MaxConns); err != nil {
		return err
	}
	if err := int32Override(lookup, "LEVIER_DATABASE_MIN_CONNS", &cfg.Database.MinConns); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_DATABASE_CONNECT_TIMEOUT", &cfg.Database.ConnectTimeout); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_DATABASE_HEALTH_TIMEOUT", &cfg.Database.HealthTimeout); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_WORKFLOW_LEASE_TTL", &cfg.Workflow.LeaseTTL); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_WORKFLOW_POLL_INTERVAL", &cfg.Workflow.PollInterval); err != nil {
		return err
	}
	if err := intOverride(lookup, "LEVIER_WORKFLOW_WORKERS", &cfg.Workflow.Workers); err != nil {
		return err
	}
	if err := intOverride(lookup, "LEVIER_WORKFLOW_MAX_ATTEMPTS", &cfg.Workflow.MaxAttempts); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_WORKFLOW_REAP_INTERVAL", &cfg.Workflow.ReapInterval); err != nil {
		return err
	}
	if err := intOverride(lookup, "LEVIER_WORKFLOW_REAP_LIMIT", &cfg.Workflow.ReapLimit); err != nil {
		return err
	}
	if err := durationOverride(lookup, "LEVIER_WORKFLOW_RECOVERY_DELAY", &cfg.Workflow.RecoveryDelay); err != nil {
		return err
	}
	if err := intOverride(lookup, "LEVIER_WORKFLOW_EVENT_BUFFER", &cfg.Workflow.EventBuffer); err != nil {
		return err
	}
	if err := boolOverride(lookup, "LEVIER_RETENTION_ENABLED", &cfg.Retention.Enabled); err != nil {
		return err
	}
	if err := intOverride(lookup, "LEVIER_RETENTION_RUN_DAYS", &cfg.Retention.RunDays); err != nil {
		return err
	}
	if err := intOverride(lookup, "LEVIER_RETENTION_SESSION_DAYS", &cfg.Retention.SessionDays); err != nil {
		return err
	}
	if err := intOverride(lookup, "LEVIER_RETENTION_BATCH_SIZE", &cfg.Retention.BatchSize); err != nil {
		return err
	}
	return nil
}

func resolveSecretFiles(cfg *Config, lookup func(string) (string, bool)) error {
	if cfg.HTTP.AdminTokenFile != "" && !envPresent(lookup, "LEVIER_HTTP_ADMIN_TOKEN") {
		value, err := readSecretText(cfg.HTTP.AdminTokenFile)
		if err != nil {
			return fmt.Errorf("read http.admin_token_file: %w", err)
		}
		cfg.HTTP.AdminToken = value
	}
	if cfg.HTTP.BearerTokenFile != "" && !envPresent(lookup, "LEVIER_HTTP_BEARER_TOKEN") {
		value, err := readSecretText(cfg.HTTP.BearerTokenFile)
		if err != nil {
			return fmt.Errorf("read http.bearer_token_file: %w", err)
		}
		cfg.HTTP.BearerToken = value
	}
	if cfg.GRPC.DaemonTokensFile != "" {
		values, err := readSecretMap(cfg.GRPC.DaemonTokensFile)
		if err != nil {
			return fmt.Errorf("read grpc.daemon_tokens_file: %w", err)
		}
		cfg.GRPC.DaemonTokens = values
	}
	if cfg.Database.URLFile != "" && !envPresent(lookup, "LEVIER_DATABASE_URL") {
		value, err := readSecretText(cfg.Database.URLFile)
		if err != nil {
			return fmt.Errorf("read database.url_file: %w", err)
		}
		cfg.Database.URL = value
	}
	return nil
}

func envPresent(lookup func(string) (string, bool), key string) bool {
	if lookup == nil {
		return false
	}
	value, ok := lookupEnv(lookup, key)
	return ok && strings.TrimSpace(value) != ""
}

func readSecretText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return value, nil
}

func readSecretMap(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}
	result := make(map[string]string, len(raw))
	for key, value := range raw {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return nil, fmt.Errorf("contains an empty daemon id or token")
		}
		result[key] = value
	}
	return result, nil
}

// SecretFileWarnings reports secret files that are readable by group/other.
// Windows does not expose POSIX permission bits reliably, so the check is skipped.
func (c Config) SecretFileWarnings() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	paths := []string{
		c.HTTP.AdminTokenFile,
		c.HTTP.BearerTokenFile,
		c.GRPC.DaemonTokensFile,
		c.Database.URLFile,
	}
	var warnings []string
	for _, path := range paths {
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.Mode().Perm()&0o077 != 0 {
			warnings = append(warnings, fmt.Sprintf("%s permissions are %04o; want 0600", path, info.Mode().Perm()))
		}
	}
	return warnings
}

// ValidateProduction rejects development defaults and requires production secrets.
func (c Config) ValidateProduction() error {
	var problems []string
	if placeholderSecret(c.HTTP.AdminToken) {
		problems = append(problems, "http.admin_token is empty or still a development placeholder")
	}
	if c.HTTP.BearerToken != "" && placeholderSecret(c.HTTP.BearerToken) {
		problems = append(problems, "http.bearer_token is still a development placeholder")
	}
	if len(c.GRPC.DaemonTokens) == 0 {
		problems = append(problems, "grpc.daemon_tokens is required in production")
	}
	for id, token := range c.GRPC.DaemonTokens {
		if strings.TrimSpace(id) == "" || placeholderSecret(token) {
			problems = append(problems, fmt.Sprintf("grpc.daemon_tokens[%q] is empty or a development placeholder", id))
		}
	}
	if placeholderSecret(c.Database.URL) {
		problems = append(problems, "database.url is empty or contains a development placeholder")
	}
	problems = append(problems, c.SecretFileWarnings()...)
	if len(problems) > 0 {
		return fmt.Errorf("invalid production config: %s", strings.Join(problems, "; "))
	}
	return nil
}

func placeholderSecret(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "" || strings.Contains(value, "change-me") || value == "mcp-e2e-secret"
}

// Validate checks cross-field constraints and required values.
func (c Config) Validate() error {
	var problems []string

	if c.ConfigVersion != 1 {
		problems = append(problems, fmt.Sprintf("config_version must be 1, got %d", c.ConfigVersion))
	}
	switch strings.ToLower(c.Environment) {
	case "development", "production":
	default:
		problems = append(problems, "environment must be development or production")
	}
	if err := validateAddr("http.addr", c.HTTP.Addr); err != nil {
		problems = append(problems, err.Error())
	}
	if err := validateAddr("grpc.addr", c.GRPC.Addr); err != nil {
		problems = append(problems, err.Error())
	}
	if c.HTTP.ReadTimeout.Std() <= 0 {
		problems = append(problems, "http.read_timeout must be positive")
	}
	if c.HTTP.WriteTimeout.Std() <= 0 {
		problems = append(problems, "http.write_timeout must be positive")
	}
	if c.HTTP.ShutdownTimeout.Std() <= 0 {
		problems = append(problems, "http.shutdown_timeout must be positive")
	}
	if c.HTTP.WSTokenTTL.Std() <= 0 {
		problems = append(problems, "http.ws_token_ttl must be positive")
	}
	if strings.TrimSpace(c.Gateway.InstanceID) == "" {
		problems = append(problems, "gateway.instance_id is required")
	}
	if _, err := roles.Parse(c.Gateway.Roles); err != nil {
		problems = append(problems, "gateway.roles: "+err.Error())
	}
	if c.Gateway.HeartbeatInterval.Std() <= 0 {
		problems = append(problems, "gateway.heartbeat_interval must be positive")
	}
	if c.Gateway.LeaseTTL.Std() <= c.Gateway.HeartbeatInterval.Std() {
		problems = append(problems, "gateway.lease_ttl must be greater than gateway.heartbeat_interval")
	}
	if c.GRPC.ShutdownTimeout.Std() <= 0 {
		problems = append(problems, "grpc.shutdown_timeout must be positive")
	}
	if c.GRPC.MaxRecvMsgBytes <= 0 {
		problems = append(problems, "grpc.max_recv_msg_bytes must be positive")
	}
	if c.GRPC.MaxSendMsgBytes <= 0 {
		problems = append(problems, "grpc.max_send_msg_bytes must be positive")
	}
	if strings.TrimSpace(c.Database.URL) == "" {
		problems = append(problems, "database.url is required")
	}
	if c.Database.MaxConns <= 0 {
		problems = append(problems, "database.max_conns must be positive")
	}
	if c.Database.MinConns < 0 || c.Database.MinConns > c.Database.MaxConns {
		problems = append(problems, "database.min_conns must be between 0 and max_conns")
	}
	if c.Database.ConnectTimeout.Std() <= 0 {
		problems = append(problems, "database.connect_timeout must be positive")
	}
	if c.Database.HealthTimeout.Std() <= 0 {
		problems = append(problems, "database.health_timeout must be positive")
	}
	if c.Workflow.Workers <= 0 {
		problems = append(problems, "workflow.workers must be positive")
	}
	if c.Workflow.LeaseTTL.Std() <= 0 {
		problems = append(problems, "workflow.lease_ttl must be positive")
	}
	if c.Workflow.PollInterval.Std() <= 0 {
		problems = append(problems, "workflow.poll_interval must be positive")
	}
	if c.Workflow.MaxAttempts <= 0 {
		problems = append(problems, "workflow.max_attempts must be positive")
	}
	if c.Workflow.ReapInterval.Std() <= 0 {
		problems = append(problems, "workflow.reap_interval must be positive")
	}
	if c.Workflow.ReapLimit <= 0 {
		problems = append(problems, "workflow.reap_limit must be positive")
	}
	if c.Workflow.RecoveryDelay.Std() <= 0 {
		problems = append(problems, "workflow.recovery_delay must be positive")
	}
	if c.Workflow.EventBuffer <= 0 {
		problems = append(problems, "workflow.event_buffer must be positive")
	}
	if c.Retention.RunDays <= 0 {
		problems = append(problems, "retention.run_days must be positive")
	}
	if c.Retention.SessionDays <= 0 {
		problems = append(problems, "retention.session_days must be positive")
	}
	if c.Retention.BatchSize <= 0 || c.Retention.BatchSize > 10000 {
		problems = append(problems, "retention.batch_size must be between 1 and 10000")
	}
	switch strings.ToLower(c.Log.Level) {
	case "debug", "info", "warn", "error":
	default:
		problems = append(problems, "log.level must be one of debug, info, warn, error")
	}
	switch strings.ToLower(c.Log.Format) {
	case "json", "text":
	default:
		problems = append(problems, "log.format must be one of json, text")
	}

	if len(problems) > 0 {
		return fmt.Errorf("invalid config: %s", strings.Join(problems, "; "))
	}
	return nil
}

func validateAddr(name, addr string) error {
	if strings.TrimSpace(addr) == "" {
		return fmt.Errorf("%s is required", name)
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%s must be host:port: %w", name, err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("%s has invalid port %q", name, port)
	}
	return nil
}

const (
	envPrefix       = "LEVIER_"
	legacyEnvPrefix = "MIDDLEWARE_"
)

// lookupEnv reads the new LEVIER_* key first, then falls back to the legacy
// MIDDLEWARE_* key for one release.
func lookupEnv(lookup func(string) (string, bool), key string) (string, bool) {
	if value, ok := lookup(key); ok {
		return value, true
	}
	if strings.HasPrefix(key, envPrefix) {
		return lookup(legacyEnvPrefix + strings.TrimPrefix(key, envPrefix))
	}
	return "", false
}

func boolOverride(lookup func(string) (string, bool), key string, dst *bool) error {
	value, ok := lookupEnv(lookup, key)
	if !ok || value == "" {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("%s must be true or false", key)
	}
	*dst = parsed
	return nil
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func defaultGatewayInstanceID() string {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "levier"
	}
	return fmt.Sprintf("%s-%d", hostname, os.Getpid())
}

func stringOverride(lookup func(string) (string, bool), key string, dst *string) {
	if value, ok := lookupEnv(lookup, key); ok {
		*dst = value
	}
}

func durationOverride(lookup func(string) (string, bool), key string, dst *Duration) error {
	value, ok := lookupEnv(lookup, key)
	if !ok {
		return nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	*dst = Duration(parsed)
	return nil
}

func intOverride(lookup func(string) (string, bool), key string, dst *int) error {
	value, ok := lookupEnv(lookup, key)
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%s: invalid integer %q", key, value)
	}
	*dst = parsed
	return nil
}

func int32Override(lookup func(string) (string, bool), key string, dst *int32) error {
	value, ok := lookupEnv(lookup, key)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return fmt.Errorf("%s: invalid integer %q", key, value)
	}
	*dst = int32(parsed)
	return nil
}
