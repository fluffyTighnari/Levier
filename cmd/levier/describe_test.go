package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fluffyTighnari/levier/internal/selfdesc"
)

func TestRunDescribeJSON(t *testing.T) {
	doc := selfdesc.New(selfdesc.Options{Version: "v-test", Commit: "abc"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/meta" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := runDescribe([]string{"--server", server.URL, "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("describe --json = %d, stderr = %s", code, stderr.String())
	}
	var got selfdesc.Document
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decode describe JSON: %v (%s)", err, stdout.String())
	}
	if got.Service != selfdesc.ServiceName || got.Build.Version != "v-test" {
		t.Fatalf("document = %+v", got)
	}
}

func TestRunDescribeHuman(t *testing.T) {
	doc := selfdesc.New(selfdesc.Options{Version: "v-test", Commit: "abc"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(doc)
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := runDescribe([]string{"--server", server.URL}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("describe = %d, stderr = %s", code, stderr.String())
	}
	for _, want := range []string{"service      levier", "version      v-test", "api          v1", "ws protocol  1"} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("describe output missing %q: %s", want, stdout.String())
		}
	}
}

func TestRunDescribeRejectsUnsupportedMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"meta_version": 2,
			"service":      "levier",
		})
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	if code := runDescribe([]string{"--server", server.URL}, &stdout, &stderr); code != 1 {
		t.Fatalf("describe unsupported = %d, want 1", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("meta_version")) {
		t.Fatalf("stderr = %s", stderr.String())
	}
}

func TestRunDescribeHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	var stdout, stderr bytes.Buffer
	if code := runDescribe([]string{"--server", server.URL}, &stdout, &stderr); code != 1 {
		t.Fatalf("describe 404 = %d, want 1", code)
	}
}
