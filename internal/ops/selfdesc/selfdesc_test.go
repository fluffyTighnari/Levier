package selfdesc

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestNewDefaults(t *testing.T) {
	doc := New(Options{})
	if doc.MetaVersion != MetaVersion || doc.Service != ServiceName {
		t.Fatalf("identity = %d/%q", doc.MetaVersion, doc.Service)
	}
	if doc.Build.Version != "dev" || doc.Build.Commit != "unknown" || doc.Build.Built != "unknown" {
		t.Fatalf("build = %+v", doc.Build)
	}
	if doc.Build.Go == "" || doc.Build.Platform == "" {
		t.Fatalf("runtime build fields = %+v", doc.Build)
	}
	if doc.API.HTTPVersion != "v1" || doc.API.WSProtocol != 1 {
		t.Fatalf("api = %+v", doc.API)
	}
	if len(doc.Capabilities.RuntimeTypes) != 1 || doc.Capabilities.RuntimeTypes[0] != "normal" {
		t.Fatalf("runtime types = %v", doc.Capabilities.RuntimeTypes)
	}
	if doc.Limits.MaxBodyBytes != DefaultMaxBodyBytes {
		t.Fatalf("max body = %d", doc.Limits.MaxBodyBytes)
	}
	if doc.Limits.DefaultRunTimeoutMS != DefaultRunTimeoutMS ||
		doc.Limits.DefaultToolTimeoutMS != DefaultToolTimeoutMS ||
		doc.Limits.MaxToolTimeoutMS != DefaultMaxToolTimeoutMS {
		t.Fatalf("timeouts = %+v", doc.Limits)
	}
	if doc.Limits.WSTokenTTLSeconds != int64(DefaultWSTokenTTL/time.Second) {
		t.Fatalf("ws ttl = %d", doc.Limits.WSTokenTTLSeconds)
	}
	if doc.Endpoints.Runs != "/api/v1/runs" || doc.Endpoints.WebSocket != "/api/v1/ws" ||
		doc.Endpoints.RunResume != "/api/v1/runs/{run_id}/resume" {
		t.Fatalf("endpoints = %+v", doc.Endpoints)
	}
}

func TestNewAppliesLimits(t *testing.T) {
	doc := New(Options{
		Version:              "v1.2.3",
		Commit:               "abc1234",
		Built:                "2026-11-18T00:00:00Z",
		MaxBodyBytes:         2048,
		DefaultRunTimeoutMS:  1000,
		DefaultToolTimeoutMS: 2000,
		MaxToolTimeoutMS:     3000,
		WSTokenTTL:           90 * time.Second,
	})
	if doc.Build.Version != "v1.2.3" || doc.Build.Commit != "abc1234" {
		t.Fatalf("build = %+v", doc.Build)
	}
	if doc.Limits.MaxBodyBytes != 2048 ||
		doc.Limits.DefaultRunTimeoutMS != 1000 ||
		doc.Limits.DefaultToolTimeoutMS != 2000 ||
		doc.Limits.MaxToolTimeoutMS != 3000 ||
		doc.Limits.WSTokenTTLSeconds != 90 {
		t.Fatalf("limits = %+v", doc.Limits)
	}
}

func TestDocumentJSONIsPublicAndStable(t *testing.T) {
	doc := New(Options{Version: "v1", Commit: "abc", Built: "now"})
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(encoded)
	for _, want := range []string{
		`"meta_version":1`,
		`"service":"levier"`,
		`"runtime_types":["normal"]`,
		`"ws_protocol":1`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metadata missing %q: %s", want, text)
		}
	}
	if strings.Contains(text, `"admin_token"`) {
		t.Fatalf("public metadata contains secret field: %s", text)
	}
	for _, secret := range []string{"change-me", "admin-change-me", "postgres://", "secret-token"} {
		if strings.Contains(text, secret) {
			t.Fatalf("metadata leaked %q: %s", secret, text)
		}
	}
}

func TestPublicSchemaIsValidJSON(t *testing.T) {
	data, err := os.ReadFile("../../../public/service-meta/v1/schema.json")
	if err != nil {
		t.Fatalf("read public schema: %v", err)
	}
	if !json.Valid(data) {
		t.Fatal("public schema is not valid JSON")
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode public schema: %v", err)
	}
	if schema["$id"] != "https://levier.dev/service-meta/v1/schema.json" {
		t.Fatalf("schema $id = %v", schema["$id"])
	}
}

func TestPublicExampleMatchesDocumentShape(t *testing.T) {
	data, err := os.ReadFile("../../../public/service-meta/v1/example.json")
	if err != nil {
		t.Fatalf("read public example: %v", err)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("decode public example: %v", err)
	}
	if doc.MetaVersion != MetaVersion || doc.Service != ServiceName {
		t.Fatalf("example identity = %d/%q", doc.MetaVersion, doc.Service)
	}
	if doc.API.HTTPVersion != "v1" || doc.API.WSProtocol != 1 {
		t.Fatalf("example API = %+v", doc.API)
	}
}
