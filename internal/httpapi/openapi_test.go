// Middleware — internal/httpapi
//
// OpenAPI skeleton syntax test.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIYAML(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "api", "openapi.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var doc struct {
		OpenAPI string         `yaml:"openapi"`
		Paths   map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse OpenAPI: %v", err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Fatalf("openapi = %q, want 3.1.0", doc.OpenAPI)
	}
	for _, path := range []string{"/runs", "/runs/{run_id}", "/runs/{run_id}/events", "/admin/apps", "/ws"} {
		if _, ok := doc.Paths[path]; !ok {
			t.Fatalf("OpenAPI missing path %q", path)
		}
	}
}
