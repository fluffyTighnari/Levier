package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fluffyTighnari/levier/internal/runtime/connection"
	"github.com/fluffyTighnari/levier/internal/state/runstore"
	"github.com/fluffyTighnari/levier/internal/state/workflow"
)

type fakeDaemonSource struct {
	daemons []connection.DaemonInfo
}

func (f *fakeDaemonSource) List() []connection.DaemonInfo {
	return f.daemons
}

func TestAdminRunAndTaskSurface(t *testing.T) {
	store := newFakeStore()
	queue := &fakeQueue{}
	apps := newFakeApps()
	run, err := store.CreateRun(context.Background(), runstore.Run{
		RunID:       "run-ops",
		AppID:       "app-ops",
		RuntimeName: "echo1",
		Status:      runstore.StatusRunning,
	})
	if err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	queue.tasks = append(queue.tasks, workflow.Task{
		ID:     "task-ops",
		RunID:  run.RunID,
		Type:   workflow.TaskDispatch,
		Status: workflow.StatusDead,
	})
	canceller := &fakeCanceller{run: run}
	server := NewServer(store, queue, Config{
		AdminToken: "admin-token",
		Canceller:  canceller,
		Daemons: &fakeDaemonSource{daemons: []connection.DaemonInfo{{
			DaemonID:          "daemon-1",
			RuntimeName:       "echo1",
			Status:            "online",
			MaxConcurrentRuns: 4,
			RunningCount:      1,
			Healthy:           true,
		}}},
	}, apps)
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	resp := request(t, http.MethodGet, httpServer.URL+"/admin/runs", "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("runs status = %d", resp.StatusCode)
	}
	var page AdminRunListResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	resp.Body.Close()
	if len(page.Runs) != 1 || page.Runs[0].RunID != run.RunID {
		t.Fatalf("runs = %+v", page.Runs)
	}

	resp = request(t, http.MethodGet, httpServer.URL+"/admin/runs/"+run.RunID, "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("run detail status = %d", resp.StatusCode)
	}
	var detail AdminRunView
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode run detail: %v", err)
	}
	resp.Body.Close()
	if len(detail.Tasks) != 1 || detail.Tasks[0].TaskID != "task-ops" {
		t.Fatalf("run detail = %+v", detail)
	}

	resp = request(t, http.MethodGet, httpServer.URL+"/admin/daemons", "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("daemons status = %d", resp.StatusCode)
	}
	var daemons AdminDaemonListResponse
	if err := json.NewDecoder(resp.Body).Decode(&daemons); err != nil {
		t.Fatalf("decode daemons: %v", err)
	}
	resp.Body.Close()
	if len(daemons.Daemons) != 1 || daemons.Daemons[0].Available != 3 {
		t.Fatalf("daemons = %+v", daemons.Daemons)
	}

	resp = request(t, http.MethodGet, httpServer.URL+"/admin/tasks?status=DEAD&run_id="+run.RunID, "admin-token", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tasks status = %d", resp.StatusCode)
	}
	var tasks AdminTaskListResponse
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		t.Fatalf("decode tasks: %v", err)
	}
	resp.Body.Close()
	if len(tasks.Tasks) != 1 || tasks.Tasks[0].TaskID != "task-ops" {
		t.Fatalf("tasks = %+v", tasks.Tasks)
	}

	resp = request(t, http.MethodPost, httpServer.URL+"/admin/runs/"+run.RunID+"/cancel", "admin-token", []byte(`{"reason":"ops test"}`))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("cancel status = %d", resp.StatusCode)
	}
	var cancelled CancelRunResponse
	if err := json.NewDecoder(resp.Body).Decode(&cancelled); err != nil {
		t.Fatalf("decode cancel: %v", err)
	}
	resp.Body.Close()
	if cancelled.Status != RunStatusCancelled || canceller.reason != "ops test" {
		t.Fatalf("cancelled = %+v, reason=%q", cancelled, canceller.reason)
	}

	resp = request(t, http.MethodGet, httpServer.URL+"/admin/runs", "wrong", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}
