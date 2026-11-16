package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
