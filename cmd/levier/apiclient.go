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

type strategyRequest struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params,omitempty"`
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
	Runtime  string          `json:"runtime"`
	Strategy strategyRequest `json:"strategy"`
	Input    []chatMessage   `json:"input"`
	Model    modelRequest    `json:"model"`
	Timeouts timeoutRequest  `json:"timeouts,omitempty"`
}

type createRunResponse struct {
	RunID   string `json:"run_id"`
	Status  string `json:"status"`
	WSURL   string `json:"ws_url"`
	WSToken string `json:"ws_token"`
}

type runView struct {
	RunID     string          `json:"run_id"`
	AppID     string          `json:"app_id"`
	Runtime   string          `json:"runtime"`
	Strategy  strategyRequest `json:"strategy"`
	Status    string          `json:"status"`
	Attempt   int             `json:"attempt"`
	SessionID string          `json:"session_id,omitempty"`
	TraceID   string          `json:"trace_id,omitempty"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
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

func (c *apiClient) createApp(ctx context.Context, name string, multiagent bool) (appView, string, error) {
	var response createAppResponse
	err := c.do(ctx, http.MethodPost, "/admin/apps", map[string]any{
		"name":       name,
		"multiagent": multiagent,
	}, &response)
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

func (c *apiClient) createRun(ctx context.Context, request createRunRequest, idempotencyKey string) (createRunResponse, error) {
	var response createRunResponse
	headers := map[string]string{}
	if idempotencyKey != "" {
		headers["Idempotency-Key"] = idempotencyKey
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
