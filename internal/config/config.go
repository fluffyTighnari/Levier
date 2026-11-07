// Package config loads and validates middleware runtime configuration.
//
// Middleware — internal/config
// Copyright 2026 The Middleware Authors.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the complete middleware runtime configuration.
type Config struct {
	HTTP     HTTPConfig     `yaml:"http"`
	GRPC     GRPCConfig     `yaml:"grpc"`
	Database DatabaseConfig `yaml:"database"`
	Workflow WorkflowConfig `yaml:"workflow"`
	Log      LogConfig      `yaml:"log"`
}

// HTTPConfig configures the App-facing HTTP server.
type HTTPConfig struct {
	Addr            string   `yaml:"addr"`
	ReadTimeout     Duration `yaml:"read_timeout"`
	WriteTimeout    Duration `yaml:"write_timeout"`
	ShutdownTimeout Duration `yaml:"shutdown_timeout"`
	BearerToken     string   `yaml:"bearer_token"`
}

// GRPCConfig configures the daemon-facing gRPC server.
type GRPCConfig struct {
	Addr            string            `yaml:"addr"`
	MaxRecvMsgBytes int               `yaml:"max_recv_msg_bytes"`
	MaxSendMsgBytes int               `yaml:"max_send_msg_bytes"`
	ShutdownTimeout Duration          `yaml:"shutdown_timeout"`
	DaemonTokens    map[string]string `yaml:"daemon_tokens"`
}

// DatabaseConfig configures the PostgreSQL connection pool.
type DatabaseConfig struct {
	URL            string   `yaml:"url"`
	MaxConns       int32    `yaml:"max_conns"`
	MinConns       int32    `yaml:"min_conns"`
	ConnectTimeout Duration `yaml:"connect_timeout"`
	HealthTimeout  Duration `yaml:"health_timeout"`
}

// WorkflowConfig configures the RunTaskQueue worker runtime.
type WorkflowConfig struct {
	Workers      int      `yaml:"workers"`
	LeaseTTL     Duration `yaml:"lease_ttl"`
	PollInterval Duration `yaml:"poll_interval"`
	MaxAttempts  int      `yaml:"max_attempts"`
}

// LogConfig configures structured logging.
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Default returns the baseline configuration used before file/env overrides.
func Default() Config {
	return Config{
		HTTP: HTTPConfig{
			Addr:            ":8080",
			ReadTimeout:     Duration(15 * time.Second),
			WriteTimeout:    Duration(30 * time.Second),
			ShutdownTimeout: Duration(10 * time.Second),
		},
		GRPC: GRPCConfig{
			Addr:            ":9090",
			MaxRecvMsgBytes: 16 << 20,
			MaxSendMsgBytes: 16 << 20,
			ShutdownTimeout: Duration(10 * time.Second),
		},
		Database: DatabaseConfig{
			MaxConns:       20,
			MinConns:       2,
			ConnectTimeout: Duration(5 * time.Second),
			HealthTimeout:  Duration(2 * time.Second),
		},
		Workflow: WorkflowConfig{
			Workers:      4,
			LeaseTTL:     Duration(30 * time.Second),
			PollInterval: Duration(200 * time.Millisecond),
			MaxAttempts:  5,
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

// Parse parses YAML, applies MIDDLEWARE_* env overrides, and validates.
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
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ApplyEnv applies the supported MIDDLEWARE_* environment overrides.
func ApplyEnv(cfg *Config, lookup func(string) (string, bool)) error {
	if lookup == nil {
		return nil
	}

	stringOverride(lookup, "MIDDLEWARE_HTTP_ADDR", &cfg.HTTP.Addr)
	stringOverride(lookup, "MIDDLEWARE_HTTP_BEARER_TOKEN", &cfg.HTTP.BearerToken)
	stringOverride(lookup, "MIDDLEWARE_GRPC_ADDR", &cfg.GRPC.Addr)
	stringOverride(lookup, "MIDDLEWARE_DATABASE_URL", &cfg.Database.URL)
	stringOverride(lookup, "MIDDLEWARE_LOG_LEVEL", &cfg.Log.Level)
	stringOverride(lookup, "MIDDLEWARE_LOG_FORMAT", &cfg.Log.Format)

	if err := durationOverride(lookup, "MIDDLEWARE_HTTP_SHUTDOWN_TIMEOUT", &cfg.HTTP.ShutdownTimeout); err != nil {
		return err
	}
	if err := durationOverride(lookup, "MIDDLEWARE_GRPC_SHUTDOWN_TIMEOUT", &cfg.GRPC.ShutdownTimeout); err != nil {
		return err
	}
	if err := durationOverride(lookup, "MIDDLEWARE_WORKFLOW_LEASE_TTL", &cfg.Workflow.LeaseTTL); err != nil {
		return err
	}
	if err := durationOverride(lookup, "MIDDLEWARE_WORKFLOW_POLL_INTERVAL", &cfg.Workflow.PollInterval); err != nil {
		return err
	}
	if err := intOverride(lookup, "MIDDLEWARE_WORKFLOW_WORKERS", &cfg.Workflow.Workers); err != nil {
		return err
	}
	if err := intOverride(lookup, "MIDDLEWARE_WORKFLOW_MAX_ATTEMPTS", &cfg.Workflow.MaxAttempts); err != nil {
		return err
	}
	return nil
}

// Validate checks cross-field constraints and required values.
func (c Config) Validate() error {
	var problems []string

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

func stringOverride(lookup func(string) (string, bool), key string, dst *string) {
	if value, ok := lookup(key); ok {
		*dst = value
	}
}

func durationOverride(lookup func(string) (string, bool), key string, dst *Duration) error {
	value, ok := lookup(key)
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
	value, ok := lookup(key)
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
