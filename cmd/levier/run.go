package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type runSubmitResult struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

type runWatchResult struct {
	RunID        string `json:"run_id"`
	Status       string `json:"status"`
	Attempt      int    `json:"attempt"`
	FinalMessage string `json:"final_message,omitempty"`
	Events       int    `json:"events"`
}

func runRun(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRunUsage(stderr)
		return 2
	}
	switch args[0] {
	case "submit":
		return runRunSubmit(args[1:], stdout, stderr)
	case "watch":
		return runRunWatch(args[1:], stdout, stderr)
	case "cancel":
		return runRunCancel(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printRunUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown run command: %s\n\n", args[0])
		printRunUsage(stderr)
		return 2
	}
}

func printRunUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier run submit --prompt <text> [--strategy react] [--watch]")
	fmt.Fprintln(out, "  levier run watch --run-id <id>")
	fmt.Fprintln(out, "  levier run cancel --run-id <id> --yes")
}

func runRunSubmit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("run submit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	credentialsPath := flags.String("credentials", os.Getenv("LEVIER_CREDENTIALS"), "credentials file")
	server := flags.String("server", envOr("LEVIER_SERVER", "http://127.0.0.1:8080"), "Levier HTTP address")
	prompt := flags.String("prompt", "", "user prompt")
	strategy := flags.String("strategy", "react", "strategy: react or echo")
	provider := flags.String("provider", "deepseek", "model provider")
	model := flags.String("model", "deepseek-flash", "model name")
	maxSteps := flags.Int("max-steps", 8, "maximum ReAct steps")
	runMS := flags.Int64("run-ms", 60000, "run timeout in milliseconds")
	toolMS := flags.Int64("tool-ms", 10000, "per-tool timeout in milliseconds")
	idempotencyKey := flags.String("idempotency-key", "", "optional Idempotency-Key")
	watch := flags.Bool("watch", false, "watch until terminal")
	interval := flags.Duration("interval", 500*time.Millisecond, "watch poll interval")
	watchTimeout := flags.Duration("watch-timeout", 30*time.Minute, "watch timeout")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if *credentialsPath == "" {
		fmt.Fprintln(stderr, "credentials path is required: --credentials or LEVIER_CREDENTIALS")
		return 2
	}
	if strings.TrimSpace(*prompt) == "" {
		fmt.Fprintln(stderr, "prompt is required: --prompt")
		return 2
	}
	credentials, err := readCredentials(*credentialsPath)
	if err != nil {
		fmt.Fprintf(stderr, "read credentials: %v\n", err)
		return 2
	}

	request := createRunRequest{
		Runtime: "normal",
		Strategy: strategyRequest{
			Name:   *strategy,
			Params: map[string]any{"max_steps": *maxSteps},
		},
		Input: []chatMessage{{Role: "user", Content: *prompt}},
		Model: modelRequest{Provider: *provider, Model: *model},
		Timeouts: timeoutRequest{
			RunMS:  *runMS,
			ToolMS: *toolMS,
		},
	}
	client := newAPIClient(*server, credentials.APIKey, *timeout)
	submitted, err := client.createRun(context.Background(), request, *idempotencyKey)
	if err != nil {
		return reportAPIError(err, stderr)
	}

	if *watch {
		if !*asJSON {
			fmt.Fprintf(stdout, "run submitted: %s\n", submitted.RunID)
		}
		return watchRun(client, submitted.RunID, *interval, *watchTimeout, *asJSON, stdout, stderr)
	}

	if *asJSON {
		return encodeJSON(stdout, runSubmitResult{RunID: submitted.RunID, Status: submitted.Status}, stderr)
	}
	fmt.Fprintf(stdout, "run submitted: %s\n", submitted.RunID)
	fmt.Fprintf(stdout, "status: %s\n", submitted.Status)
	fmt.Fprintf(stdout, "next: levier run watch --run-id %s --credentials %s\n", submitted.RunID, *credentialsPath)
	return 0
}

func runRunWatch(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("run watch", flag.ContinueOnError)
	flags.SetOutput(stderr)
	credentialsPath := flags.String("credentials", os.Getenv("LEVIER_CREDENTIALS"), "credentials file")
	server := flags.String("server", envOr("LEVIER_SERVER", "http://127.0.0.1:8080"), "Levier HTTP address")
	runID := flags.String("run-id", "", "run id")
	interval := flags.Duration("interval", 500*time.Millisecond, "poll interval")
	watchTimeout := flags.Duration("timeout", 30*time.Minute, "watch timeout")
	requestTimeout := flags.Duration("request-timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if *credentialsPath == "" {
		fmt.Fprintln(stderr, "credentials path is required: --credentials or LEVIER_CREDENTIALS")
		return 2
	}
	if *runID == "" {
		fmt.Fprintln(stderr, "run id is required: --run-id")
		return 2
	}
	credentials, err := readCredentials(*credentialsPath)
	if err != nil {
		fmt.Fprintf(stderr, "read credentials: %v\n", err)
		return 2
	}
	client := newAPIClient(*server, credentials.APIKey, *requestTimeout)
	return watchRun(client, *runID, *interval, *watchTimeout, *asJSON, stdout, stderr)
}

func watchRun(client *apiClient, runID string, interval, timeout time.Duration, asJSON bool, stdout, stderr io.Writer) int {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	lastStatus := ""
	for {
		run, err := client.getRun(ctx, runID)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Fprintf(stderr, "run watch timed out; run_id=%s last_status=%s\n", runID, lastStatus)
				return 5
			}
			return reportAPIError(err, stderr)
		}
		if !asJSON && run.Status != lastStatus {
			fmt.Fprintf(stdout, "status: %s\n", run.Status)
			lastStatus = run.Status
		}
		if isTerminalStatus(run.Status) {
			return finishWatch(client, run, asJSON, stdout, stderr)
		}

		select {
		case <-ctx.Done():
			fmt.Fprintf(stderr, "run watch timed out; run_id=%s last_status=%s\n", runID, run.Status)
			return 5
		case <-time.After(interval):
		}
	}
}

func finishWatch(client *apiClient, run runView, asJSON bool, stdout, stderr io.Writer) int {
	events, err := client.listRunEvents(context.Background(), run.RunID, 0, 0, 1000)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	result := runWatchResult{RunID: run.RunID, Status: run.Status, Attempt: run.Attempt, Events: len(events.Events)}
	for _, event := range events.Events {
		if event.EventType != "MESSAGE_COMPLETED" {
			continue
		}
		if content, ok := event.Data["content"].(string); ok {
			result.FinalMessage = content
		}
	}

	if asJSON {
		return encodeJSON(stdout, result, stderr)
	}
	if result.FinalMessage != "" {
		fmt.Fprintf(stdout, "final: %s\n", result.FinalMessage)
	}
	return terminalExitCode(run.Status)
}

func runRunCancel(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("run cancel", flag.ContinueOnError)
	flags.SetOutput(stderr)
	credentialsPath := flags.String("credentials", os.Getenv("LEVIER_CREDENTIALS"), "credentials file")
	server := flags.String("server", envOr("LEVIER_SERVER", "http://127.0.0.1:8080"), "Levier HTTP address")
	runID := flags.String("run-id", "", "run id")
	yes := flags.Bool("yes", false, "confirm cancellation")
	timeout := flags.Duration("timeout", 10*time.Second, "per-request timeout")
	asJSON := flags.Bool("json", false, "print JSON")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	if *credentialsPath == "" {
		fmt.Fprintln(stderr, "credentials path is required: --credentials or LEVIER_CREDENTIALS")
		return 2
	}
	if *runID == "" {
		fmt.Fprintln(stderr, "run id is required: --run-id")
		return 2
	}
	credentials, err := readCredentials(*credentialsPath)
	if err != nil {
		fmt.Fprintf(stderr, "read credentials: %v\n", err)
		return 2
	}
	client := newAPIClient(*server, credentials.APIKey, *timeout)
	run, err := client.getRun(context.Background(), *runID)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if isTerminalStatus(run.Status) {
		fmt.Fprintf(stderr, "run %s is already %s\n", run.RunID, run.Status)
		return 5
	}
	if !requireConfirmation(*yes, fmt.Sprintf("Cancel run %s (current status %s)?", run.RunID, run.Status), stdout, stderr) {
		return 5
	}
	cancelled, err := client.cancelRun(context.Background(), *runID)
	if err != nil {
		return reportAPIError(err, stderr)
	}
	if *asJSON {
		return encodeJSON(stdout, runSubmitResult{RunID: cancelled.RunID, Status: cancelled.Status}, stderr)
	}
	fmt.Fprintf(stdout, "run cancelled: %s\n", cancelled.RunID)
	fmt.Fprintf(stdout, "status: %s\n", cancelled.Status)
	return 0
}

func isTerminalStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "timeout":
		return true
	default:
		return false
	}
}

func terminalExitCode(status string) int {
	switch status {
	case "completed":
		return 0
	case "cancelled", "timeout":
		return 5
	default:
		return 1
	}
}

func encodeJSON(out io.Writer, value any, stderr io.Writer) int {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "encode JSON: %v\n", err)
		return 1
	}
	return 0
}
