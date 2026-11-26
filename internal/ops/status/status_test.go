// Levier — status overview tests.
//
// Copyright 2026 The Levier Authors.
package status

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/runtime/connection"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
	"github.com/fluffyTighnari/levier/internal/state/workflow"
)

type fakeDaemons struct{ infos []connection.DaemonInfo }

func (f fakeDaemons) List() []connection.DaemonInfo { return f.infos }

type fakeApps struct{ stats connection.AppStats }

func (f fakeApps) Stats() connection.AppStats { return f.stats }

type fakeRuns struct {
	stats runstore.Stats
	apps  int64
	err   error
}

func (f fakeRuns) Stats(context.Context) (runstore.Stats, error) { return f.stats, f.err }
func (f fakeRuns) CountApps(context.Context) (int64, error)      { return f.apps, f.err }

type fakeWorkflow struct {
	stats workflow.Stats
	err   error
}

func (f fakeWorkflow) Stats(context.Context) (workflow.Stats, error) { return f.stats, f.err }

func TestSnapshotAggregatesState(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	service := New(Options{
		Now:       func() time.Time { return now },
		StartedAt: now.Add(-90 * time.Second),
		Version:   "v-test",
		Commit:    "abc1234",
		Health:    func(context.Context) error { return nil },
		Hub: fakeDaemons{infos: []connection.DaemonInfo{{
			DaemonID:          "daemon-secret-id",
			RuntimeType:       runtimev1.RuntimeType_NORMAL,
			MaxConcurrentRuns: 4,
			RunningCount:      1,
			Reserved:          2,
			Healthy:           true,
		}}},
		AppHub: fakeApps{stats: connection.AppStats{Apps: 2, Connections: 3}},
		Runs: fakeRuns{
			stats: runstore.Stats{
				Total: 5,
				ByStatus: map[string]int64{
					runstore.StatusQueued:    1,
					runstore.StatusAssigned:  1,
					runstore.StatusRunning:   1,
					runstore.StatusCompleted: 2,
				},
			},
			apps: 2,
		},
		Workflow: fakeWorkflow{stats: workflow.Stats{
			Total:            4,
			ByStatus:         map[workflow.Status]int64{workflow.StatusPending: 1, workflow.StatusLeased: 1, workflow.StatusDone: 2},
			OldestPendingAge: 7 * time.Second,
		}},
	})

	snapshot := service.Snapshot(context.Background())
	if snapshot.StatusVersion != Version || snapshot.Service != ServiceName {
		t.Fatalf("identity = %d/%q", snapshot.StatusVersion, snapshot.Service)
	}
	if snapshot.UptimeSeconds != 90 || snapshot.AsOf != now.Format(time.RFC3339) {
		t.Fatalf("time = %d/%q", snapshot.UptimeSeconds, snapshot.AsOf)
	}
	if snapshot.Health.Readiness != "ok" || snapshot.Health.Database != "up" {
		t.Fatalf("health = %+v", snapshot.Health)
	}
	if snapshot.Daemons.Connected != 1 || snapshot.Daemons.Healthy != 1 ||
		snapshot.Daemons.Capacity != 4 || snapshot.Daemons.Used != 2 ||
		snapshot.Daemons.Available != 2 {
		t.Fatalf("daemons = %+v", snapshot.Daemons)
	}
	if snapshot.Daemons.ByRuntime["normal"].Used != 2 {
		t.Fatalf("daemon detail = %+v", snapshot.Daemons)
	}
	if snapshot.Apps.Registered != 2 || snapshot.Apps.WebSocketConnections != 3 {
		t.Fatalf("apps = %+v", snapshot.Apps)
	}
	if snapshot.Runs.Total != 5 || snapshot.Runs.Active != 2 ||
		snapshot.Runs.ByStatus[runstore.StatusCompleted] != 2 {
		t.Fatalf("runs = %+v", snapshot.Runs)
	}
	if snapshot.Workflow.Total != 4 || snapshot.Workflow.Pending != 1 ||
		snapshot.Workflow.Leased != 1 || snapshot.Workflow.Done != 2 ||
		snapshot.Workflow.OldestPendingSeconds != 7 {
		t.Fatalf("workflow = %+v", snapshot.Workflow)
	}
}

func TestSnapshotDegradesWhenDatabaseUnavailable(t *testing.T) {
	service := New(Options{
		Health: func(context.Context) error { return errors.New("db down") },
		Hub: fakeDaemons{infos: []connection.DaemonInfo{{
			RuntimeType:       runtimev1.RuntimeType_NORMAL,
			MaxConcurrentRuns: 1,
			Healthy:           true,
		}}},
		Runs: fakeRuns{err: errors.New("should not be queried")},
	})
	snapshot := service.Snapshot(context.Background())
	if snapshot.Health.Readiness != "degraded" || snapshot.Health.Database != "down" {
		t.Fatalf("health = %+v", snapshot.Health)
	}
	if snapshot.Daemons.Connected != 1 {
		t.Fatalf("daemons = %+v", snapshot.Daemons)
	}
	if snapshot.Runs.Total != 0 || snapshot.Runs.ByStatus[runstore.StatusQueued] != 0 {
		t.Fatalf("runs = %+v", snapshot.Runs)
	}
}

func TestSnapshotDoesNotExposeIdentifiersOrSecrets(t *testing.T) {
	service := New(Options{
		Hub: fakeDaemons{infos: []connection.DaemonInfo{{
			DaemonID:          "daemon-secret-id",
			RuntimeType:       runtimev1.RuntimeType_NORMAL,
			MaxConcurrentRuns: 1,
		}}},
	})
	data, err := json.Marshal(service.Snapshot(context.Background()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	for _, forbidden := range []string{"daemon_id", "app_id", "run_id", "daemon-secret-id", "admin-change-me"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("status leaked %q: %s", forbidden, text)
		}
	}
}

func TestDefaultSnapshotShape(t *testing.T) {
	snapshot := DefaultSnapshot()
	if snapshot.StatusVersion != Version || snapshot.Service != ServiceName {
		t.Fatalf("identity = %d/%q", snapshot.StatusVersion, snapshot.Service)
	}
	if snapshot.Runs.ByStatus[runstore.StatusQueued] != 0 {
		t.Fatalf("run statuses = %+v", snapshot.Runs.ByStatus)
	}
}

func TestPublicStatusSchemaAndExample(t *testing.T) {
	schemaData, err := os.ReadFile("../../../public/service-status/v1/schema.json")
	if err != nil {
		t.Fatalf("read public status schema: %v", err)
	}
	if !json.Valid(schemaData) {
		t.Fatal("public status schema is not valid JSON")
	}
	exampleData, err := os.ReadFile("../../../public/service-status/v1/example.json")
	if err != nil {
		t.Fatalf("read public status example: %v", err)
	}
	var example Snapshot
	if err := json.Unmarshal(exampleData, &example); err != nil {
		t.Fatalf("decode public status example: %v", err)
	}
	if example.StatusVersion != Version || example.Service != ServiceName {
		t.Fatalf("example identity = %d/%q", example.StatusVersion, example.Service)
	}
}

func DefaultSnapshot() Snapshot {
	return New(Options{}).Snapshot(context.Background())
}
