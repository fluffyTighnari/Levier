// Levier — internal/httpapi
//
// OpenAPI syntax and HTTP route consistency test.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIMatchesHTTPRoutes(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "api", "openapi.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var doc struct {
		OpenAPI string                    `yaml:"openapi"`
		Paths   map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse OpenAPI: %v", err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Fatalf("openapi = %q, want 3.1.0", doc.OpenAPI)
	}

	expected := map[string][]string{
		"/healthz":                        {"get"},
		"/readyz":                         {"get"},
		"/admin/apps":                     {"post"},
		"/admin/apps/{app_id}/rotate-key": {"post"},
		"/apps":                           {"post"},
		"/apps/{app_id}":                  {"get"},
		"/runs":                           {"post"},
		"/runs/{run_id}":                  {"get"},
		"/runs/{run_id}/events":           {"get"},
		"/runs/{run_id}/cancel":           {"post"},
		"/sessions/{session_id}":          {"get"},
		"/sessions/{session_id}/events":   {"get"},
		"/ws-token":                       {"post"},
		"/ws":                             {"get"},
	}
	if len(doc.Paths) != len(expected) {
		t.Fatalf("OpenAPI paths = %v, want %d paths", pathKeys(doc.Paths), len(expected))
	}
	for path, methods := range expected {
		documented, ok := doc.Paths[path]
		if !ok {
			t.Fatalf("OpenAPI missing path %q", path)
		}
		actual := operationKeys(documented)
		if !equalStrings(actual, methods) {
			t.Fatalf("OpenAPI %s methods = %v, want %v", path, actual, methods)
		}
	}
}

func pathKeys(paths map[string]map[string]any) []string {
	keys := make([]string, 0, len(paths))
	for key := range paths {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func operationKeys(path map[string]any) []string {
	keys := make([]string, 0, len(path))
	for key := range path {
		switch key {
		case "get", "post", "put", "patch", "delete", "head", "options":
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
