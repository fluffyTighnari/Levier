package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderListAndUpsert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/admin/model-providers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"providers": []map[string]any{{
					"name":              "deepseek",
					"adapter":           "openai_compatible",
					"base_url":          "https://api.deepseek.com",
					"default_model":     "deepseek-flash",
					"models":            []string{"deepseek-flash"},
					"secret_ref":        "deepseek_api_key",
					"secret_configured": true,
					"enabled":           true,
					"default":           true,
				}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/admin/model-providers":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			body["secret_configured"] = true
			_ = json.NewEncoder(w).Encode(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	if code := runProvider([]string{
		"list", "--server", server.URL, "--admin-token", "admin", "--json",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("provider list = %d, stderr = %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "secret-key") || !strings.Contains(stdout.String(), "deepseek") {
		t.Fatalf("provider list output = %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := runProvider([]string{
		"add",
		"--server", server.URL,
		"--admin-token", "admin",
		"--name", "deepseek",
		"--adapter", "openai",
		"--base-url", "https://api.deepseek.com",
		"--default-model", "deepseek-flash",
		"--models", "deepseek-flash,deepseek-reasoner",
		"--secret-ref", "deepseek_api_key",
		"--default",
		"--json",
	}, &stdout, &stderr); code != 0 {
		t.Fatalf("provider add = %d, stderr = %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "deepseek") {
		t.Fatalf("provider add output = %s", stdout.String())
	}
}

func TestProviderRequiresAdminToken(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runProviderList(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("provider list without admin = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "admin token is required") {
		t.Fatalf("stderr = %s", stderr.String())
	}
}
