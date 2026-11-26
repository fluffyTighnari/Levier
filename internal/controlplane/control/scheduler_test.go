// Levier — internal/controlplane/control
//
// Scheduler table tests.
//
// Copyright 2026 The Levier Authors.
package control

import (
	"testing"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
)

func candidate(id string, runtimeType runtimev1.RuntimeType, running, max int32, labels map[string]string, runtimeName string) Candidate {
	return Candidate{
		DaemonID: id, RuntimeName: runtimeName, RuntimeType: runtimeType, Healthy: true,
		RunningCount: running, MaxConcurrentRuns: max,
		Labels: labels,
	}
}

func TestSelectFiltersAndLeastLoaded(t *testing.T) {
	candidates := []Candidate{
		candidate("d1", runtimev1.RuntimeType_NORMAL, 3, 4, map[string]string{"pool": "a"}, "echo1"),
		candidate("d2", runtimev1.RuntimeType_NORMAL, 0, 4, map[string]string{"pool": "a"}, "echo1"),
		candidate("d3", runtimev1.RuntimeType_SANDBOX, 0, 4, map[string]string{"pool": "b"}, "echo1"),
		candidate("d4", runtimev1.RuntimeType_NORMAL, 0, 4, map[string]string{"pool": "b"}, "react1"),
	}

	tests := []struct {
		name    string
		request PlacementRequest
		wantID  string
		wantOK  bool
	}{
		{
			name:    "least loaded normal echo pool a",
			request: PlacementRequest{Runtime: runtimev1.RuntimeType_NORMAL, RuntimeName: "echo1", Labels: map[string]string{"pool": "a"}},
			wantID:  "d2", wantOK: true,
		},
		{
			name:    "runtime filter",
			request: PlacementRequest{Runtime: runtimev1.RuntimeType_SANDBOX, RuntimeName: "echo1"},
			wantID:  "d3", wantOK: true,
		},
		{
			name:    "runtime name filter",
			request: PlacementRequest{Runtime: runtimev1.RuntimeType_NORMAL, RuntimeName: "react1"},
			wantID:  "d4", wantOK: true,
		},
		{
			name:    "labels filter",
			request: PlacementRequest{Runtime: runtimev1.RuntimeType_NORMAL, RuntimeName: "echo1", Labels: map[string]string{"pool": "b"}},
			wantID:  "", wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := Select(candidates, tt.request)
			if id != tt.wantID || ok != tt.wantOK {
				t.Fatalf("Select() = %q, %v; want %q, %v", id, ok, tt.wantID, tt.wantOK)
			}
		})
	}
}

func TestSelectFullCapacity(t *testing.T) {
	candidates := []Candidate{
		candidate("d1", runtimev1.RuntimeType_NORMAL, 4, 4, nil, "echo1"),
		candidate("d2", runtimev1.RuntimeType_NORMAL, 2, 2, nil, "echo1"),
	}
	if id, ok := Select(candidates, PlacementRequest{
		Runtime: runtimev1.RuntimeType_NORMAL, RuntimeName: "echo1",
	}); ok {
		t.Fatalf("Select() = %q, true; want no daemon", id)
	}
}

func TestSelectStickyAndFallback(t *testing.T) {
	candidates := []Candidate{
		candidate("d1", runtimev1.RuntimeType_LONGTERM, 3, 4, nil, "react1"),
		candidate("d2", runtimev1.RuntimeType_LONGTERM, 0, 4, nil, "react1"),
	}

	id, ok := Select(candidates, PlacementRequest{
		Runtime: runtimev1.RuntimeType_LONGTERM, RuntimeName: "react1", StickyDaemonID: "d1",
	})
	if !ok || id != "d1" {
		t.Fatalf("sticky Select() = %q, %v; want d1", id, ok)
	}

	full := []Candidate{
		candidate("d1", runtimev1.RuntimeType_LONGTERM, 4, 4, nil, "react1"),
		candidate("d2", runtimev1.RuntimeType_LONGTERM, 0, 4, nil, "react1"),
	}
	id, ok = Select(full, PlacementRequest{
		Runtime: runtimev1.RuntimeType_LONGTERM, RuntimeName: "react1", StickyDaemonID: "d1",
	})
	if !ok || id != "d2" {
		t.Fatalf("sticky fallback = %q, %v; want d2", id, ok)
	}

	unhealthy := []Candidate{
		{DaemonID: "d1", RuntimeType: runtimev1.RuntimeType_LONGTERM, RuntimeName: "react1", MaxConcurrentRuns: 4, Healthy: false},
		candidate("d2", runtimev1.RuntimeType_LONGTERM, 0, 4, nil, "react1"),
	}
	id, ok = Select(unhealthy, PlacementRequest{
		Runtime: runtimev1.RuntimeType_LONGTERM, RuntimeName: "react1", StickyDaemonID: "d1",
	})
	if !ok || id != "d2" {
		t.Fatalf("unhealthy sticky fallback = %q, %v; want d2", id, ok)
	}
}
