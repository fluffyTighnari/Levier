package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/fluffyTighnari/levier/internal/config"
	"github.com/fluffyTighnari/levier/internal/db"
)

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

type doctorReport struct {
	OK     bool          `json:"ok"`
	Config string        `json:"config"`
	Server string        `json:"server"`
	Checks []doctorCheck `json:"checks"`
}

func runDoctor(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", envOr("LEVIER_CONFIG", "levier.example.yaml"), "path to YAML configuration")
	server := flags.String("server", envOr("LEVIER_SERVER", "http://127.0.0.1:8080"), "Levier HTTP address")
	credentials := flags.String("credentials", os.Getenv("LEVIER_CREDENTIALS"), "optional App credentials file")
	asJSON := flags.Bool("json", false, "print JSON")
	timeout := flags.Duration("timeout", 5*time.Second, "per-request timeout")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	report := doctorReport{Config: *configPath, Server: serverBase(*server)}
	client := &http.Client{Timeout: *timeout}
	ctx := context.Background()

	cfg, cfgErr := config.Load(*configPath)
	if cfgErr != nil {
		report.add("config parse", "fail", cfgErr.Error(), "check --config path or LEVIER_CONFIG")
	} else {
		report.add("config parse", "ok", *configPath, "")
	}

	if cfgErr == nil {
		pool, err := db.Open(ctx, db.Config{
			URL:            cfg.Database.URL,
			MaxConns:       cfg.Database.MaxConns,
			MinConns:       cfg.Database.MinConns,
			ConnectTimeout: cfg.Database.ConnectTimeout.Std(),
			HealthTimeout:  cfg.Database.HealthTimeout.Std(),
		})
		if err != nil {
			report.add("database connect", "fail", err.Error(), "check PostgreSQL and LEVIER_DATABASE_URL")
			report.add("migrations", "skip", "database unavailable", "")
		} else {
			report.add("database connect", "ok", "connected", "")
			defer pool.Close()

			var version int64
			var count int
			queryCtx, cancel := context.WithTimeout(ctx, *timeout)
			err := pool.QueryRow(queryCtx, "SELECT COALESCE(max(version),0), count(*) FROM schema_migrations").Scan(&version, &count)
			cancel()
			if err != nil {
				report.add("migrations", "fail", err.Error(), "run levier migrate up")
			} else {
				report.add("migrations", "ok", fmt.Sprintf("version %d (%d applied)", version, count), "")
			}
		}
	} else {
		report.add("database connect", "skip", "config not loaded", "")
		report.add("migrations", "skip", "config not loaded", "")
	}

	if code, _, err := httpGet(ctx, client, report.Server+"/healthz", ""); err != nil {
		report.add("HTTP /healthz", "fail", err.Error(), "start levier serve or check --server")
	} else if code != http.StatusOK {
		report.add("HTTP /healthz", "fail", fmt.Sprintf("status %d", code), "start levier serve or check --server")
	} else {
		report.add("HTTP /healthz", "ok", "200", "")
	}

	if code, _, err := httpGet(ctx, client, report.Server+"/readyz", ""); err != nil {
		report.add("HTTP /readyz", "fail", err.Error(), "check database and /readyz")
	} else if code != http.StatusOK {
		report.add("HTTP /readyz", "fail", fmt.Sprintf("status %d", code), "check database and /readyz")
	} else {
		report.add("HTTP /readyz", "ok", "200", "")
	}

	adminToken := os.Getenv("LEVIER_ADMIN_TOKEN")
	if adminToken == "" && cfgErr == nil {
		adminToken = cfg.HTTP.AdminToken
	}
	if adminToken == "" {
		report.add("admin auth", "fail", "no admin token", "set LEVIER_ADMIN_TOKEN or http.admin_token")
	} else if code, _, err := httpGet(ctx, client, report.Server+"/admin/mcp", adminToken); err != nil {
		report.add("admin auth", "fail", err.Error(), "check --server")
	} else if code != http.StatusOK {
		report.add("admin auth", "fail", fmt.Sprintf("status %d", code), "check LEVIER_ADMIN_TOKEN / http.admin_token")
	} else {
		report.add("admin auth", "ok", "200", "")
	}

	if *credentials == "" {
		report.add("app credentials", "skip", "no --credentials / LEVIER_CREDENTIALS", "")
	} else if app, err := readCredentials(*credentials); err != nil {
		report.add("app credentials", "fail", err.Error(), "check credentials file")
	} else if code, _, err := httpGet(ctx, client, report.Server+"/api/v1/apps/"+url.PathEscape(app.AppID), app.APIKey); err != nil {
		report.add("app credentials", "fail", err.Error(), "check --server")
	} else if code != http.StatusOK {
		report.add("app credentials", "fail", fmt.Sprintf("status %d", code), "check app key or rotate it")
	} else {
		report.add("app credentials", "ok", app.AppID, "")
	}

	report.OK = true
	for _, check := range report.Checks {
		if check.Status == "fail" {
			report.OK = false
			break
		}
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintf(stderr, "encode doctor: %v\n", err)
			return 1
		}
	} else {
		total := len(report.Checks)
		for index, check := range report.Checks {
			fmt.Fprintf(stdout, "[%d/%d] %-20s %s", index+1, total, check.Name, check.Status)
			if check.Message != "" {
				fmt.Fprintf(stdout, "  %s", check.Message)
			}
			fmt.Fprintln(stdout)
			if check.Status == "fail" && check.Fix != "" {
				fmt.Fprintf(stdout, "      fix: %s\n", check.Fix)
			}
		}
	}

	if !report.OK {
		return 1
	}
	return 0
}

func (r *doctorReport) add(name, status, message, fix string) {
	r.Checks = append(r.Checks, doctorCheck{Name: name, Status: status, Message: message, Fix: fix})
}
