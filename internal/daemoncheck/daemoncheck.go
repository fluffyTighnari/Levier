// Package daemoncheck runs the daemon protocol conformance check.
//
// Levier — internal/daemoncheck
// Copyright 2026 The Levier Authors.
package daemoncheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/id"
	"github.com/fluffyTighnari/levier/internal/protocol"
	"github.com/fluffyTighnari/levier/internal/testutil/fakedaemon"
)

// Config describes one daemon conformance check.
type Config struct {
	GRPCAddr   string
	HTTPURL    string
	DaemonID   string
	Token      string
	AdminToken string
	Strategy   string
	Timeout    time.Duration
}

// HTTPError reports a non-2xx HTTP response from the Levier control plane.
type HTTPError struct {
	Status int
	Err    error
}

func (e *HTTPError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("status %d: %v", e.Status, e.Err)
	}
	return fmt.Sprintf("status %d", e.Status)
}

func (e *HTTPError) Unwrap() error { return e.Err }

// Run performs one daemon protocol conformance check against a running
// Levier server. It creates a temporary App and two Runs.
func Run(cfg Config) (map[string]string, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = "127.0.0.1:9090"
	}
	if cfg.HTTPURL == "" {
		cfg.HTTPURL = "http://127.0.0.1:8080"
	}
	if cfg.DaemonID == "" {
		cfg.DaemonID = "daemon-local-1"
	}
	if cfg.Strategy == "" {
		cfg.Strategy = "echo"
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout*3)
	defer cancel()
	checks := map[string]string{}

	// Phase 1: accept a task, ack, emit a completed run.
	d1, err := fakedaemon.Dial(ctx, cfg.GRPCAddr, fakedaemon.Config{
		DaemonID:            cfg.DaemonID,
		AuthToken:           cfg.Token,
		MaxConcurrentRuns:   2,
		SupportedStrategies: []string{cfg.Strategy},
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
	if err := waitDaemonReady(d1); err != nil {
		return checks, fmt.Errorf("dial daemon: %w", err)
	}

	httpClient := &apiClient{ctx: ctx, base: strings.TrimRight(cfg.HTTPURL, "/"), admin: cfg.AdminToken}
	app, err := httpClient.createApp("daemon-check-" + id.New())
	if err != nil {
		return checks, fmt.Errorf("create app: %w", err)
	}
	httpClient.appKey = app.Key

	run1, err := httpClient.createRun(cfg.Strategy)
	if err != nil {
		return checks, fmt.Errorf("create run: %w", err)
	}
	assign, err := waitAssignment(ctx, d1, cfg.Timeout)
	if err != nil {
		return checks, fmt.Errorf("task assign: %w", err)
	}
	checks["hello"] = "ok"
	checks["task_assign"] = assign.GetTaskAssign().GetRunId()
	if assign.GetTaskAssign().GetRunId() != run1 {
		return checks, fmt.Errorf("assigned run %s, want %s", assign.GetTaskAssign().GetRunId(), run1)
	}
	if assign.GetTaskAssign().GetStrategy() != cfg.Strategy {
		return checks, fmt.Errorf("assigned strategy %q", assign.GetTaskAssign().GetStrategy())
	}
	checks["ack"] = "accepted"
	if err := httpClient.waitStatus(run1, "completed", cfg.Timeout); err != nil {
		return checks, err
	}
	checks["events"] = "running/completed"
	checks["completed"] = "ok"
	_ = d1.Close()

	// Phase 2: accept a task, then verify Cancel and stop with cancelled.
	assignments := make(chan *runtimev1.Envelope, 4)
	cancels := make(chan *runtimev1.Envelope, 4)
	d2, err := fakedaemon.Dial(ctx, cfg.GRPCAddr, fakedaemon.Config{
		DaemonID:            cfg.DaemonID,
		AuthToken:           cfg.Token,
		MaxConcurrentRuns:   2,
		SupportedStrategies: []string{cfg.Strategy},
		Version:             "daemon-check",
		Labels:              map[string]string{"pool": "daemon-check"},
		Accept:              true,
		HeartbeatInterval:   2 * time.Second,
	})
	if err != nil {
		return checks, fmt.Errorf("dial daemon (cancel phase): %w", err)
	}
	defer d2.Close()
	if err := waitDaemonReady(d2); err != nil {
		return checks, fmt.Errorf("dial daemon (cancel phase): %w", err)
	}
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

	run2, err := httpClient.createRun(cfg.Strategy)
	if err != nil {
		return checks, fmt.Errorf("create run (cancel phase): %w", err)
	}
	select {
	case <-assignments:
	case <-time.After(cfg.Timeout):
		return checks, fmt.Errorf("cancel phase: no task assign")
	}
	if err := httpClient.cancelRun(run2, "daemon-check"); err != nil {
		return checks, fmt.Errorf("cancel run: %w", err)
	}
	select {
	case env := <-cancels:
		checks["cancel"] = env.GetCancel().GetReason()
	case <-time.After(cfg.Timeout):
		return checks, fmt.Errorf("cancel phase: no cancel envelope")
	}
	if err := httpClient.waitStatus(run2, "cancelled", cfg.Timeout); err != nil {
		return checks, err
	}
	checks["cancelled"] = "ok"

	return checks, nil
}

func waitDaemonReady(daemon *fakedaemon.Daemon) error {
	return daemon.WaitReady(500 * time.Millisecond)
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
	ctx    context.Context
	base   string
	admin  string
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
	req, err := http.NewRequestWithContext(c.ctx, method, c.base+path, reader)
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
		return appCredentials{}, &HTTPError{Status: status, Err: err}
	}
	key := created.APIKey
	if key == "" {
		var rotated struct {
			APIKey string `json:"api_key"`
		}
		status, err = c.do(http.MethodPost, "/admin/apps/"+created.App.AppID+"/rotate-key", c.admin, nil, &rotated)
		if err != nil || status >= 300 {
			return appCredentials{}, fmt.Errorf("rotate key: %w", &HTTPError{Status: status, Err: err})
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
		return "", &HTTPError{Status: status, Err: err}
	}
	return created.RunID, nil
}

func (c *apiClient) cancelRun(runID, reason string) error {
	status, err := c.do(http.MethodPost, "/api/v1/runs/"+runID+"/cancel", c.appKey, map[string]any{"reason": reason}, nil)
	if err != nil || status >= 300 {
		return &HTTPError{Status: status, Err: err}
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
			return fmt.Errorf("get run: %w", &HTTPError{Status: status, Err: err})
		}
		last = view.Status
		if last == want {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("run %s ended as %q, want %q", runID, last, want)
}
