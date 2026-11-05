// Middleware — internal/protocol
//
// Cross-language RuntimeChannel golden envelope test.
//
// Copyright 2026 The Middleware Authors.
package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestContractGolden(t *testing.T) {
	path := filepath.Join("..", "..", "tests", "contract", "envelopes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden fixtures: %v", err)
	}

	var fixtures struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("decode golden fixtures: %v", err)
	}
	if len(fixtures.Messages) == 0 {
		t.Fatal("golden fixtures are empty")
	}

	for i, raw := range fixtures.Messages {
		var env runtimev1.Envelope
		if err := protojson.Unmarshal(raw, &env); err != nil {
			t.Fatalf("fixture[%d] protojson.Unmarshal: %v", i, err)
		}
		if err := Validate(&env); err != nil {
			t.Fatalf("fixture[%d] Validate: %v", i, err)
		}

		canonical, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&env)
		if err != nil {
			t.Fatalf("fixture[%d] protojson.Marshal: %v", i, err)
		}

		var want, got any
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatalf("fixture[%d] decode want: %v", i, err)
		}
		if err := json.Unmarshal(canonical, &got); err != nil {
			t.Fatalf("fixture[%d] decode got: %v", i, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("fixture[%d] round-trip mismatch\n got: %s\nwant: %s", i, canonical, raw)
		}
	}
}
