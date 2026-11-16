package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func serverBase(value string) string {
	return strings.TrimRight(value, "/")
}

func httpGet(ctx context.Context, client *http.Client, url, token string) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return response.StatusCode, nil, fmt.Errorf("read response: %w", err)
	}
	return response.StatusCode, body, nil
}
