// Levier — data maintenance commands.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/fluffyTighnari/levier/internal/config"
	"github.com/fluffyTighnari/levier/internal/db"
	"github.com/fluffyTighnari/levier/internal/maintenance"
)

func runMaintenance(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printMaintenanceUsage(stderr)
		return 2
	}
	switch args[0] {
	case "status":
		return runMaintenanceStatus(args[1:], stdout, stderr)
	case "prune":
		return runMaintenancePrune(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printMaintenanceUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown maintenance command: %s\n\n", args[0])
		printMaintenanceUsage(stderr)
		return 2
	}
}

func printMaintenanceUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier maintenance status [--config path] [--json]")
	fmt.Fprintln(out, "  levier maintenance prune [--config path] [--dry-run] [--yes] [--json]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "prune is a dry-run unless --yes is supplied; retention.enabled must be true to delete data.")
}

func runMaintenanceStatus(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("maintenance status", flag.ContinueOnError)
	configPath := flags.String("config", envOr("LEVIER_CONFIG", "levier.example.yaml"), "path to YAML configuration")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 15*time.Second, "database operation timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "maintenance status: unexpected argument: %s\n\n", flags.Arg(0))
		printMaintenanceUsage(stderr)
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 2
	}
	policy := retentionPolicy(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
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
		return 1
	}
	defer pool.Close()

	status, err := maintenance.GetStatus(ctx, pool, policy)
	if err != nil {
		fmt.Fprintf(stderr, "maintenance status: %v\n", err)
		return 1
	}
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(status); err != nil {
			fmt.Fprintf(stderr, "encode maintenance status: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(stdout, "retention: enabled=%t run_days=%d session_days=%d batch_size=%d\n",
		policy.Enabled, policy.RunDays, policy.SessionDays, policy.BatchSize)
	fmt.Fprintf(stdout, "runs:      total=%d active=%d terminal=%d eligible=%d\n",
		status.RunsTotal, status.RunsActive, status.RunsTerminal, status.RunsEligible)
	fmt.Fprintf(stdout, "oldest:    terminal=%s\n", formatAge(status.OldestTerminalAt))
	fmt.Fprintf(stdout, "events:    total=%d\n", status.EventsTotal)
	fmt.Fprintf(stdout, "tasks:     %s\n", formatStatusCounts(status.TasksByStatus))
	fmt.Fprintf(stdout, "invocations: %s\n", formatStatusCounts(status.InvocationsByStatus))
	fmt.Fprintf(stdout, "sessions:  total=%d active=%d paused=%d expired=%d eligible=%d\n",
		status.SessionsTotal, status.SessionsActive, status.SessionsPaused,
		status.SessionsExpired, status.SessionsEligible)
	fmt.Fprintf(stdout, "cutoff:    runs=%s sessions=%s\n",
		status.RunCutoff.Format(time.RFC3339), status.SessionCutoff.Format(time.RFC3339))
	return 0
}

func runMaintenancePrune(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("maintenance prune", flag.ContinueOnError)
	configPath := flags.String("config", envOr("LEVIER_CONFIG", "levier.example.yaml"), "path to YAML configuration")
	yes := flags.Bool("yes", false, "delete eligible data")
	dryRun := flags.Bool("dry-run", true, "show candidates without deleting (default)")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 60*time.Second, "database operation timeout")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "maintenance prune: unexpected argument: %s\n\n", flags.Arg(0))
		printMaintenanceUsage(stderr)
		return 2
	}
	dryRunSet := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "dry-run" {
			dryRunSet = true
		}
	})
	if *yes && dryRunSet && *dryRun {
		fmt.Fprintln(stderr, "maintenance prune: use --dry-run or --yes, not both")
		return 2
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(stderr, "load config: %v\n", err)
		return 2
	}
	policy := retentionPolicy(cfg)
	execute := *yes
	if *yes && !policy.Enabled {
		fmt.Fprintln(stderr, "prune refused: retention.enabled is false")
		fmt.Fprintln(stderr, "set retention.enabled=true, review the dry-run, then retry with --yes")
		return 1
	}
	if *dryRun && !*yes {
		execute = false
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
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
		return 1
	}
	defer pool.Close()

	result, err := maintenance.Prune(ctx, pool, policy, execute)
	if err != nil {
		if errors.Is(err, maintenance.ErrRetentionDisabled) {
			fmt.Fprintln(stderr, "prune refused: retention is disabled")
			return 1
		}
		fmt.Fprintf(stderr, "maintenance prune: %v\n", err)
		return 1
	}
	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintf(stderr, "encode maintenance result: %v\n", err)
			return 1
		}
		return 0
	}

	mode := "dry-run"
	if !result.DryRun {
		mode = "execute"
	}
	fmt.Fprintf(stdout, "mode:      %s\n", mode)
	fmt.Fprintf(stdout, "retention: enabled=%t run_days=%d session_days=%d batch_size=%d\n",
		result.Enabled, result.RunDays, result.SessionDays, result.BatchSize)
	fmt.Fprintf(stdout, "runs:      eligible=%d deleted=%d\n",
		result.RunsEligible, result.RunsDeleted)
	fmt.Fprintf(stdout, "sessions:  eligible=%d expired=%d deleted=%d\n",
		result.SessionsEligible, result.SessionsExpired, result.SessionsDeleted)
	fmt.Fprintf(stdout, "batches:   %d\n", result.Batches)
	fmt.Fprintf(stdout, "cutoff:    runs=%s sessions=%s\n",
		result.RunCutoff.Format(time.RFC3339), result.SessionCutoff.Format(time.RFC3339))
	if result.DryRun {
		fmt.Fprintln(stdout, "note:      no data was deleted; rerun with --yes after reviewing the plan")
	}
	return 0
}

func retentionPolicy(cfg config.Config) maintenance.Policy {
	return maintenance.Policy{
		Enabled:     cfg.Retention.Enabled,
		RunDays:     cfg.Retention.RunDays,
		SessionDays: cfg.Retention.SessionDays,
		BatchSize:   cfg.Retention.BatchSize,
	}
}

func formatAge(value *time.Time) string {
	if value == nil {
		return "none"
	}
	return fmt.Sprintf("%s (%s ago)", value.Format(time.RFC3339), time.Since(*value).Round(time.Second))
}

func formatStatusCounts(counts map[string]int64) string {
	if len(counts) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return strings.Join(parts, " ")
}
