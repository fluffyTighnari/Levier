// Middleware — daemon protocol conformance check.
//
// Connects as a RuntimeChannel daemon, accepts one TaskAssign, emits events,
// then verifies cancel handling against a real Middleware instance.
//
// Usage:
//
//	go run ./cmd/mw-daemon-check
//	go run ./cmd/mw-daemon-check --grpc 127.0.0.1:9090 --http http://127.0.0.1:8080
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/id"
	"github.com/fluffyTighnari/middleware/internal/protocol"
	"github.com/fluffyTighnari/middleware/internal/testutil/fakedaemon"
)

type config struct {
	grpcAddr   string
	httpURL    string
	daemonID   string
	token      string
	adminToken string
	strategy   string
	timeout    time.Duration
	jsonOut    bool
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.grpcAddr, "grpc", "127.0.0.1:9090", "Middleware gRPC address")
	flag.StringVar(&cfg.httpURL, "http", "http://127.0.0.1:8080", "Middleware HTTP address")
	flag.StringVar(&cfg.daemonID, "daemon-id", "daemon-local-1", "pre-provisioned daemon id")
	flag.StringVar(&cfg.token, "token", "change-me", "daemon auth token")
	flag.StringVar(&cfg.adminToken, "admin-token", "admin-change-me", "admin token for app bootstrap")
	flag.StringVar(&cfg.strategy, "strategy", "echo", "strategy to advertise and run")
	flag.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "per-step timeout")
	flag.BoolVar(&cfg.jsonOut, "json", false, "print JSON result")
	flag.Parse()

	checks, err := run(cfg)
	result := map[string]any{"ok": err == nil, "checks": checks}
	if err != nil {
		result["error"] = err.Error()
	}
	if cfg.jsonOut {
		_ = json.NewEncoder(os.Stdout).Encode(result)
	} else {
		encoded, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(encoded))
	}
	if err != nil {
		os.Exit(1)
	}
}

func run(cfg config) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout*3)
	defer cancel()
	checks := map[string]string{}

	httpClient := &apiClient{base: strings.TrimRight(cfg.httpURL, "/"), admin: cfg.adminToken}
	app, err := httpClient.createApp("daemon-check-" + id.New())
	if err != nil {
		return checks, fmt.Errorf("create app: %w", err)
	}
	httpClient.appID = app.ID
	httpClient.appKey = app.Key

	// Phase 1: accept a task, ack, emit a completed run.
	d1, err := fakedaemon.Dial(ctx, cfg.grpcAddr, fakedaemon.Config{
		DaemonID:            cfg.daemonID,
		AuthToken:           cfg.token,
		MaxConcurrentRuns:   2,
		SupportedStrategies: []string{cfg.strategy},
		Version:             "daemon-check",
		Labels:              map[string]string{"pool": "daemon-check"},
		Accept:              true,
		AutoEcho:            true,
		HeartbeatInterval:   2 * time.Second,
	})
	if err != nil {
		return checks, fmt.Errorf("dial daemon: %w", err)
	}
	defer d1.Close()

	run1, err := httpClient.createRun(cfg.strategy)
	if err != nil {
		return checks, fmt.Errorf("create run: %w", err)
	}
	assign, err := waitAssignment(ctx, d1, cfg.timeout)
	if err != nil {
		return checks, fmt.Errorf("task assign: %w", err)
	}
	checks["hello"] = "ok"
	checks["task_assign"] = assign.GetTaskAssign().GetRunId()
	if assign.GetTaskAssign().GetRunId() != run1 {
		return checks, fmt.Errorf("assigned run %s, want %s", assign.GetTaskAssign().GetRunId(), run1)
	}
	if assign.GetTaskAssign().GetStrategy() != cfg.strategy {
		return checks, fmt.Errorf("assigned strategy %q", assign.GetTaskAssign().GetStrategy())
	}
	checks["ack"] = "accepted"
	if err := httpClient.waitStatus(run1, "completed", cfg.timeout); err != nil {
		return checks, err
	}
	checks["events"] = "running/completed"
	checks["completed"] = "ok"
	_ = d1.Close()

	// Phase 2: accept a task, then verify Cancel and stop with cancelled.
	assignments := make(chan *runtimev1.Envelope, 4)
	cancels := make(chan *runtimev1.Envelope, 4)
	d2, err := fakedaemon.Dial(ctx, cfg.grpcAddr, fakedaemon.Config{
		DaemonID:            cfg.daemonID,
		AuthToken:           cfg.token,
		MaxConcurrentRuns:   2,
		SupportedStrategies: []string{cfg.strategy},
		Version:             "daemon-check",
		Labels:              map[string]string{"pool": "daemon-check"},
		Accept:              true,
		HeartbeatInterval:   2 * time.Second,
	})
	if err != nil {
		return checks, fmt.Errorf("dial daemon (cancel phase): %w", err)
	}
	defer d2.Close()
	d2.SetOnMessage(func(_ context.Context, env *runtimev1.Envelope, daemon *fakedaemon.Daemon) error {
		if env.GetTaskAssign() != nil {
			ack, err := protocol.NewResponse(env, &runtimev1.TaskAck{Accepted: true})
			if err != nil {
				return err
			}
			if err := daemon.Send(ack); err != nil {
				return err
			}
			if err := daemon.SendEvent(env.GetRunId(), env.GetAttempt(), env.GetLeaseEpoch(), runtimev1.EventType_RUN_STATUS, map[string]any{"status": "running"}); err != nil {
				return err
			}
			select {
			case assignments <- env:
			default:
			}
		}
		if env.GetCancel() != nil {
			if err := daemon.SendEvent(env.GetRunId(), env.GetAttempt(), env.GetLeaseEpoch(), runtimev1.EventType_RUN_STATUS, map[string]any{"status": "cancelled"}); err != nil {
				return err
			}
			select {
			case cancels <- env:
			default:
			}
		}
		return nil
	})

	run2, err := httpClient.createRun(cfg.strategy)
	if err != nil {
		return checks, fmt.Errorf("create run (cancel phase): %w", err)
	}
	select {
	case <-assignments:
	case <-time.After(cfg.timeout):
		return checks, fmt.Errorf("cancel phase: no task assign")
	}
	if err := httpClient.cancelRun(run2, "daemon-check"); err != nil {
		return checks, fmt.Errorf("cancel run: %w", err)
	}
	select {
	case env := <-cancels:
		checks["cancel"] = env.GetCancel().GetReason()
	case <-time.After(cfg.timeout):
		return checks, fmt.Errorf("cancel phase: no cancel envelope")
	}
	if err := httpClient.waitStatus(run2, "cancelled", cfg.timeout); err != nil {
		return checks, err
	}
	checks["cancelled"] = "ok"

	return checks, nil
}

func waitAssignment(ctx context.Context, daemon *fakedaemon.Daemon, timeout time.Duration) (*runtimev1.Envelope, error) {
	select {
	case env := <-daemon.Assignments():
		return env, nil
	case <-time.After(timeout):
		return nil, context.DeadlineExceeded
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type appCredentials struct {
	ID  string
	Key string
}

type apiClient struct {
	base   string
	admin  string
	appID  string
	appKey string
	client http.Client
}

func (c *apiClient) do(method, path, token string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return resp.StatusCode, nil
}

func (c *apiClient) createApp(name string) (appCredentials, error) {
	var created struct {
		App struct {
			AppID string `json:"app_id"`
		} `json:"app"`
		APIKey string `json:"api_key"`
	}
	status, err := c.do(http.MethodPost, "/admin/apps", c.admin, map[string]any{"name": name}, &created)
	if err != nil || status >= 300 {
		return appCredentials{}, fmt.Errorf("status %d: %v", status, err)
	}
	key := created.APIKey
	if key == "" {
		var rotated struct {
			APIKey string `json:"api_key"`
		}
		status, err = c.do(http.MethodPost, "/admin/apps/"+created.App.AppID+"/rotate-key", c.admin, nil, &rotated)
		if err != nil || status >= 300 {
			return appCredentials{}, fmt.Errorf("rotate key: status %d: %v", status, err)
		}
		key = rotated.APIKey
	}
	return appCredentials{ID: created.App.AppID, Key: key}, nil
}

func (c *apiClient) createRun(strategy string) (string, error) {
	var created struct {
		RunID string `json:"run_id"`
	}
	status, err := c.do(http.MethodPost, "/api/v1/runs", c.appKey, map[string]any{
		"runtime":  "normal",
		"strategy": map[string]any{"name": strategy, "params": map[string]any{"max_steps": 2}},
		"input":    []map[string]any{{"role": "user", "content": "daemon-check"}},
		"model":    map[string]any{"provider": "anthropic", "model": "claude-test"},
		"timeouts": map[string]any{"run_ms": 30000, "tool_ms": 10000},
	}, &created)
	if err != nil || status >= 300 {
		return "", fmt.Errorf("status %d: %v", status, err)
	}
	return created.RunID, nil
}

func (c *apiClient) cancelRun(runID, reason string) error {
	status, err := c.do(http.MethodPost, "/api/v1/runs/"+runID+"/cancel", c.appKey, map[string]any{"reason": reason}, nil)
	if err != nil || status >= 300 {
		return fmt.Errorf("status %d: %v", status, err)
	}
	return nil
}

func (c *apiClient) waitStatus(runID, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		var view struct {
			Status string `json:"status"`
		}
		status, err := c.do(http.MethodGet, "/api/v1/runs/"+runID, c.appKey, nil, &view)
		if err != nil || status >= 300 {
			return fmt.Errorf("get run: status %d: %v", status, err)
		}
		last = view.Status
		if last == want {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("run %s ended as %q, want %q", runID, last, want)
}
