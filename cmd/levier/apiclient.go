package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type appView struct {
	AppID      string `json:"app_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Multiagent bool   `json:"multiagent"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type createAppResponse struct {
	App    appView `json:"app"`
	APIKey string  `json:"api_key"`
}

type rotateKeyResponse struct {
	APIKey string `json:"api_key"`
}

type appListResponse struct {
	Apps []appView `json:"apps"`
}

type wsTokenResponse struct {
	WSURL     string `json:"ws_url"`
	WSToken   string `json:"ws_token"`
	ExpiresIn int64  `json:"expires_in"`
}

type adminTaskView struct {
	TaskID         string `json:"task_id"`
	RunID          string `json:"run_id"`
	TaskType       string `json:"task_type"`
	Status         string `json:"status"`
	Attempt        int    `json:"attempt"`
	MaxAttempts    int    `json:"max_attempts"`
	AvailableAt    string `json:"available_at"`
	LeaseOwner     string `json:"lease_owner,omitempty"`
	LeaseExpiresAt string `json:"lease_expires_at,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type adminRunView struct {
	RunID            string          `json:"run_id"`
	AppID            string          `json:"app_id"`
	Runtime          string          `json:"runtime"`
	RuntimeName      string          `json:"runtime_name"`
	Status           string          `json:"status"`
	Attempt          int             `json:"attempt"`
	SessionID        string          `json:"session_id,omitempty"`
	TraceID          string          `json:"trace_id,omitempty"`
	AssignedDaemonID string          `json:"assigned_daemon_id,omitempty"`
	LeaseEpoch       int64           `json:"lease_epoch"`
	AssignedAt       string          `json:"assigned_at,omitempty"`
	LastEventAttempt int             `json:"last_event_attempt"`
	LastEventSeq     int64           `json:"last_event_seq"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
	Tasks            []adminTaskView `json:"tasks,omitempty"`
}

type adminRunListResponse struct {
	Runs       []adminRunView `json:"runs"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type adminTaskListResponse struct {
	Tasks []adminTaskView `json:"tasks"`
}

type adminDaemonView struct {
	DaemonID          string `json:"daemon_id"`
	RuntimeName       string `json:"runtime_name"`
	Status            string `json:"status"`
	RuntimeType       string `json:"runtime_type"`
	MaxConcurrentRuns int32  `json:"max_concurrent_runs"`
	Running           int32  `json:"running"`
	Reserved          int32  `json:"reserved"`
	Available         int32  `json:"available"`
	Healthy           bool   `json:"healthy"`
	ConnectedAt       string `json:"connected_at"`
	LastHeartbeat     string `json:"last_heartbeat"`
}

type adminDaemonListResponse struct {
	Daemons []adminDaemonView `json:"daemons"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type modelRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type timeoutRequest struct {
	RunMS  int64 `json:"run_ms,omitempty"`
	ToolMS int64 `json:"tool_ms,omitempty"`
}

type createRunRequest struct {
	RuntimeName string         `json:"runtime_name"`
	Runtime     string         `json:"runtime,omitempty"`
	Params      map[string]any `json:"params,omitempty"`
	Input       []chatMessage  `json:"input"`
	Model       *modelRequest  `json:"model,omitempty"`
	Timeouts    timeoutRequest `json:"timeouts,omitempty"`
}

type createRunResponse struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	WSURL   string `json:"ws_url"`
	WSToken string `json:"ws_token"`
}

type runView struct {
	RunID       string `json:"run_id"`
	AppID       string `json:"app_id"`
	RuntimeName string `json:"runtime_name"`
	Status      string `json:"status"`
	Attempt     int    `json:"attempt"`
	SessionID   string `json:"session_id,omitempty"`
	TraceID     string `json:"trace_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type runEvent struct {
	RunID     string         `json:"run_id"`
	Attempt   int            `json:"attempt"`
	Seq       int64          `json:"seq"`
	Source    string         `json:"source"`
	EventType string         `json:"event_type"`
	Data      map[string]any `json:"data"`
	TsMS      int64          `json:"ts_ms"`
}

type eventListResponse struct {
	Events      []runEvent `json:"events"`
	CaughtUp    bool       `json:"caught_up"`
	NextAttempt int        `json:"next_attempt"`
	NextSeq     int64      `json:"next_seq"`
}

type cancelRunResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

type apiError struct {
	Status    int
	Code      string
	Message   string
	RequestID string
	Retryable bool
}

func (e *apiError) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("%s (%s, request_id=%s)", e.Message, e.Code, e.RequestID)
	}
	return fmt.Sprintf("%s (%s)", e.Message, e.Code)
}

type apiClient struct {
	base   string
	token  string
	client *http.Client
}

func newAPIClient(base, token string, timeout time.Duration) *apiClient {
	return &apiClient{
		base:   serverBase(base),
		token:  token,
		client: &http.Client{Timeout: timeout},
	}
}

func (c *apiClient) do(ctx context.Context, method, path string, body, out any) error {
	return c.doWithHeaders(ctx, method, path, body, out, nil)
}

func (c *apiClient) doWithHeaders(ctx context.Context, method, path string, body, out any, headers map[string]string) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		httpError := &apiError{
			Status:    response.StatusCode,
			Code:      fmt.Sprintf("HTTP_%d", response.StatusCode),
			Message:   strings.TrimSpace(string(payload)),
			RequestID: response.Header.Get("X-Request-ID"),
		}
		var envelope struct {
			Error struct {
				Code      string `json:"code"`
				Message   string `json:"message"`
				Retryable bool   `json:"retryable"`
				RequestID string `json:"request_id"`
			} `json:"error"`
		}
		if json.Unmarshal(payload, &envelope) == nil && envelope.Error.Code != "" {
			httpError.Code = envelope.Error.Code
			httpError.Message = envelope.Error.Message
			httpError.Retryable = envelope.Error.Retryable
			if envelope.Error.RequestID != "" {
				httpError.RequestID = envelope.Error.RequestID
			}
		}
		return httpError
	}

	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

type modelProviderView struct {
	Name             string   `json:"name"`
	Adapter          string   `json:"adapter"`
	BaseURL          string   `json:"base_url"`
	DefaultModel     string   `json:"default_model"`
	Models           []string `json:"models"`
	SecretRef        string   `json:"secret_ref"`
	SecretConfigured bool     `json:"secret_configured"`
	Enabled          bool     `json:"enabled"`
	Default          bool     `json:"default"`
}

type modelProviderListResponse struct {
	Providers []modelProviderView `json:"providers"`
}

func (c *apiClient) listModelProviders(ctx context.Context) ([]modelProviderView, error) {
	var response modelProviderListResponse
	if err := c.do(ctx, http.MethodGet, "/admin/model-providers", nil, &response); err != nil {
		return nil, err
	}
	return response.Providers, nil
}

func (c *apiClient) getModelProvider(ctx context.Context, name string) (modelProviderView, error) {
	var response modelProviderView
	err := c.do(ctx, http.MethodGet, "/admin/model-providers/"+url.PathEscape(name), nil, &response)
	return response, err
}

func (c *apiClient) upsertModelProvider(ctx context.Context, payload map[string]any) (modelProviderView, error) {
	var response modelProviderView
	err := c.do(ctx, http.MethodPost, "/admin/model-providers", payload, &response)
	return response, err
}

func (c *apiClient) putModelProviderSecret(ctx context.Context, name, apiKey string) (modelProviderView, error) {
	var response modelProviderView
	err := c.do(ctx, http.MethodPut, "/admin/model-providers/"+url.PathEscape(name)+"/secret", map[string]string{
		"api_key": apiKey,
	}, &response)
	return response, err
}

func (c *apiClient) deleteModelProvider(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/admin/model-providers/"+url.PathEscape(name), nil, nil)
}

func (c *apiClient) checkModelProvider(ctx context.Context, name string) (modelProviderView, error) {
	var response modelProviderView
	err := c.do(ctx, http.MethodPost, "/admin/model-providers/"+url.PathEscape(name)+"/check", nil, &response)
	return response, err
}

type mcpServerView struct {
	Name         string   `json:"name"`
	Transport    string   `json:"transport"`
	AllowedTools []string `json:"allowed_tools"`
	SecretRefs   []string `json:"secret_refs"`
}

type mcpServerListResponse struct {
	Servers []mcpServerView `json:"servers"`
}

func (c *apiClient) listMCPServers(ctx context.Context) ([]mcpServerView, error) {
	var response mcpServerListResponse
	if err := c.do(ctx, http.MethodGet, "/admin/mcp", nil, &response); err != nil {
		return nil, err
	}
	return response.Servers, nil
}

func (c *apiClient) upsertMCPServer(ctx context.Context, payload map[string]any) (mcpServerView, error) {
	var response mcpServerView
	err := c.do(ctx, http.MethodPost, "/admin/mcp", payload, &response)
	return response, err
}

func (c *apiClient) deleteMCPServer(ctx context.Context, name string, force bool) error {
	path := "/admin/mcp/" + url.PathEscape(name)
	if force {
		path += "?force=true"
	}
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *apiClient) createApp(ctx context.Context, name string, multiagent bool, mcpRefs []string) (appView, string, error) {
	var response createAppResponse
	body := map[string]any{
		"name":       name,
		"multiagent": multiagent,
	}
	if len(mcpRefs) > 0 {
		body["mcp_refs"] = mcpRefs
	}
	err := c.do(ctx, http.MethodPost, "/admin/apps", body, &response)
	return response.App, response.APIKey, err
}

func (c *apiClient) rotateAppKey(ctx context.Context, appID string) (string, error) {
	var response rotateKeyResponse
	err := c.do(ctx, http.MethodPost, "/admin/apps/"+url.PathEscape(appID)+"/rotate-key", nil, &response)
	return response.APIKey, err
}

func (c *apiClient) getApp(ctx context.Context, appID string) (appView, error) {
	var response appView
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+url.PathEscape(appID), nil, &response)
	return response, err
}

func (c *apiClient) listApps(ctx context.Context) ([]appView, error) {
	var response appListResponse
	err := c.do(ctx, http.MethodGet, "/admin/apps", nil, &response)
	return response.Apps, err
}

func (c *apiClient) issueWSToken(ctx context.Context) (wsTokenResponse, error) {
	var response wsTokenResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/ws-token", nil, &response)
	return response, err
}

func (c *apiClient) listAdminRuns(ctx context.Context, query url.Values) (adminRunListResponse, error) {
	var response adminRunListResponse
	err := c.do(ctx, http.MethodGet, "/admin/runs?"+query.Encode(), nil, &response)
	return response, err
}

func (c *apiClient) getAdminRun(ctx context.Context, runID string) (adminRunView, error) {
	var response adminRunView
	err := c.do(ctx, http.MethodGet, "/admin/runs/"+url.PathEscape(runID), nil, &response)
	return response, err
}

func (c *apiClient) cancelAdminRun(ctx context.Context, runID, reason string) (cancelRunResponse, error) {
	var response cancelRunResponse
	err := c.do(ctx, http.MethodPost, "/admin/runs/"+url.PathEscape(runID)+"/cancel", map[string]string{
		"reason": reason,
	}, &response)
	return response, err
}

func (c *apiClient) listAdminDaemons(ctx context.Context) (adminDaemonListResponse, error) {
	var response adminDaemonListResponse
	err := c.do(ctx, http.MethodGet, "/admin/daemons", nil, &response)
	return response, err
}

func (c *apiClient) listAdminTasks(ctx context.Context, query url.Values) (adminTaskListResponse, error) {
	var response adminTaskListResponse
	err := c.do(ctx, http.MethodGet, "/admin/tasks?"+query.Encode(), nil, &response)
	return response, err
}

func (c *apiClient) createRun(
	ctx context.Context,
	request createRunRequest,
	idempotencyKey string,
	requestID string,
) (createRunResponse, error) {
	var response createRunResponse
	headers := map[string]string{}
	if idempotencyKey != "" {
		headers["Idempotency-Key"] = idempotencyKey
	}
	if requestID != "" {
		headers["X-Request-ID"] = requestID
	}
	err := c.doWithHeaders(ctx, http.MethodPost, "/api/v1/runs", request, &response, headers)
	return response, err
}

func (c *apiClient) getRun(ctx context.Context, runID string) (runView, error) {
	var response runView
	err := c.do(ctx, http.MethodGet, "/api/v1/runs/"+url.PathEscape(runID), nil, &response)
	return response, err
}

func (c *apiClient) listRunEvents(ctx context.Context, runID string, afterAttempt int, afterSeq int64, limit int) (eventListResponse, error) {
	query := url.Values{}
	query.Set("after_attempt", strconv.Itoa(afterAttempt))
	query.Set("after_seq", strconv.FormatInt(afterSeq, 10))
	query.Set("limit", strconv.Itoa(limit))
	var response eventListResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/runs/"+url.PathEscape(runID)+"/events?"+query.Encode(), nil, &response)
	return response, err
}

func (c *apiClient) cancelRun(ctx context.Context, runID string) (cancelRunResponse, error) {
	var response cancelRunResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/runs/"+url.PathEscape(runID)+"/cancel", nil, &response)
	return response, err
}
