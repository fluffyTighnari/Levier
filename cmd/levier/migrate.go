package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fluffyTighnari/levier/internal/config"
	"github.com/fluffyTighnari/levier/internal/db"
)

type migrateStatusResult struct {
	Database string                `json:"database"`
	Current  int64                 `json:"current"`
	Applied  int                   `json:"applied"`
	Pending  []db.PendingMigration `json:"pending"`
}

type migrateUpResult struct {
	Database string `json:"database"`
	Before   int64  `json:"before"`
	After    int64  `json:"after"`
	Applied  int    `json:"applied"`
	Pending  int    `json:"pending"`
}

func runMigrate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printMigrateUsage(stderr)
		return 2
	}
	switch args[0] {
	case "status":
		return runMigrateStatus(args[1:], stdout, stderr)
	case "up":
		return runMigrateUp(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printMigrateUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown migrate command: %s\n\n", args[0])
		printMigrateUsage(stderr)
		return 2
	}
}

func printMigrateUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier migrate status [--config path] [--json]")
	fmt.Fprintln(out, "  levier migrate up [--config path] [--json]")
}

func runMigrateStatus(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("migrate status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", controlConfigValue(), "path to YAML configuration")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 30*time.Second, "database operation timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}

	cfg, pool, code := openMigrationPool(*configPath, *timeout, stderr)
	if pool == nil {
		return code
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	state, err := db.Status(ctx, pool)
	if err != nil {
		fmt.Fprintf(stderr, "migration status: %v\n", err)
		return 1
	}
	result := migrateStatusResult{
		Database: redactDatabaseURL(cfg.Database.URL),
		Current:  state.Current,
		Applied:  len(state.Applied),
		Pending:  state.Pending,
	}
	if *asJSON {
		return encodeJSON(stdout, result, stderr)
	}
	fmt.Fprintf(stdout, "database: %s\n", result.Database)
	fmt.Fprintf(stdout, "current: %d\n", result.Current)
	fmt.Fprintf(stdout, "applied: %d\n", result.Applied)
	fmt.Fprintf(stdout, "pending: %d\n", len(result.Pending))
	for _, pending := range result.Pending {
		fmt.Fprintf(stdout, "  %s\n", pending.Filename)
	}
	return 0
}

func runMigrateUp(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("migrate up", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", controlConfigValue(), "path to YAML configuration")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 2*time.Minute, "database operation timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}

	cfg, pool, code := openMigrationPool(*configPath, *timeout, stderr)
	if pool == nil {
		return code
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	before, err := db.Status(ctx, pool)
	if err != nil {
		fmt.Fprintf(stderr, "migration status: %v\n", err)
		return 1
	}
	database := redactDatabaseURL(cfg.Database.URL)
	if !*asJSON {
		fmt.Fprintf(stdout, "database: %s\n", database)
		fmt.Fprintf(stdout, "pending: %d\n", len(before.Pending))
		for _, pending := range before.Pending {
			fmt.Fprintf(stdout, "  %s\n", pending.Filename)
		}
	}
	if len(before.Pending) == 0 {
		result := migrateUpResult{Database: database, Before: before.Current, After: before.Current}
		if *asJSON {
			return encodeJSON(stdout, result, stderr)
		}
		fmt.Fprintln(stdout, "already up to date")
		return 0
	}

	if err := db.Migrate(ctx, pool); err != nil {
		fmt.Fprintf(stderr, "migrate: %v\n", err)
		return 1
	}
	after, err := db.Status(ctx, pool)
	if err != nil {
		fmt.Fprintf(stderr, "migration status after up: %v\n", err)
		return 1
	}
	applied := len(before.Pending) - len(after.Pending)
	if applied < 0 {
		applied = 0
	}
	result := migrateUpResult{
		Database: database,
		Before:   before.Current,
		After:    after.Current,
		Applied:  applied,
		Pending:  len(after.Pending),
	}
	if *asJSON {
		return encodeJSON(stdout, result, stderr)
	}
	fmt.Fprintf(stdout, "applied: %d\n", result.Applied)
	fmt.Fprintf(stdout, "current: %d\n", result.After)
	return 0
}

func openMigrationPool(configPath string, timeout time.Duration, stderr io.Writer) (config.Config, *pgxpool.Pool, int) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return config.Config{}, nil, 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	pool, err := db.Open(ctx, db.Config{
		URL:            cfg.Database.URL,
		MaxConns:       cfg.Database.MaxConns,
		MinConns:       cfg.Database.MinConns,
		ConnectTimeout: cfg.Database.ConnectTimeout.Std(),
		HealthTimeout:  cfg.Database.HealthTimeout.Std(),
	})
	if err != nil {
		fmt.Fprintf(stderr, "connect database: %v\n", err)
		return config.Config{}, nil, 1
	}
	return cfg, pool, 0
}

func redactDatabaseURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return "invalid database url"
	}
	return parsed.Redacted()
}
