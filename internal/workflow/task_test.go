// Levier — internal/workflow
//
// Task model tests.
//
// Copyright 2026 The Levier Authors.
package workflow

import (
	"strings"
	"testing"
	"time"

	"github.com/fluffyTighnari/levier/internal/id"
)

func TestNewTaskDefaults(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	task := NewTask("run-1", TaskDispatch, now, 0)

	if task.Status != StatusPending {
		t.Fatalf("Status = %q", task.Status)
	}
	if task.MaxAttempts != 5 {
		t.Fatalf("MaxAttempts = %d, want 5", task.MaxAttempts)
	}
	if task.AvailableAt != now {
		t.Fatalf("AvailableAt = %v", task.AvailableAt)
	}
	if _, err := id.Parse(task.ID); err != nil {
		t.Fatalf("task ID is not a ULID: %v", err)
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestTaskTypeDefaults(t *testing.T) {
	tests := map[TaskType]int{
		TaskDispatch:  5,
		TaskRetry:     3,
		TaskTimeout:   3,
		TaskReconcile: 3,
	}
	for taskType, want := range tests {
		if got := taskType.DefaultMaxAttempts(); got != want {
			t.Fatalf("%s DefaultMaxAttempts() = %d, want %d", taskType, got, want)
		}
		if !taskType.Valid() {
			t.Fatalf("%s Valid() = false", taskType)
		}
	}
	if TaskType("NOPE").Valid() {
		t.Fatal("unknown task type is valid")
	}
}

func TestStatusPredicates(t *testing.T) {
	if !StatusPending.Active() || !StatusLeased.Active() {
		t.Fatal("pending/leased should be active")
	}
	if StatusDone.Active() || StatusDead.Active() || StatusCancelled.Active() {
		t.Fatal("terminal status should not be active")
	}
	if !StatusDone.Terminal() || !StatusDead.Terminal() || !StatusCancelled.Terminal() {
		t.Fatal("done/dead/cancelled should be terminal")
	}
	if Status("NOPE").Valid() {
		t.Fatal("unknown status is valid")
	}
}

func TestTaskValidate(t *testing.T) {
	now := time.Now().UTC()
	valid := Task{
		ID:          id.New(),
		RunID:       "run-1",
		Type:        TaskDispatch,
		Status:      StatusPending,
		AvailableAt: now,
		MaxAttempts: 5,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	leased := valid
	leased.Status = StatusLeased
	leased.Attempt = 1
	leased.LeaseOwner = "worker-1"
	leased.LeaseToken = 1
	leased.LeaseExpiresAt = now.Add(30 * time.Second)
	if err := leased.Validate(); err != nil {
		t.Fatalf("leased Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Task)
		want   string
	}{
		{"bad id", func(task *Task) { task.ID = "bad" }, "task.id"},
		{"missing run", func(task *Task) { task.RunID = "" }, "run_id"},
		{"bad type", func(task *Task) { task.Type = "NOPE" }, "task.type"},
		{"bad status", func(task *Task) { task.Status = "NOPE" }, "task.status"},
		{"zero max", func(task *Task) { task.MaxAttempts = 0 }, "max_attempts"},
		{"attempt over max", func(task *Task) { task.Attempt = 6 }, "exceeds"},
		{"leased no owner", func(task *Task) {
			task.Status = StatusLeased
			task.Attempt = 1
			task.LeaseToken = 1
			task.LeaseExpiresAt = now.Add(time.Second)
		}, "lease_owner"},
		{"leased no token", func(task *Task) {
			task.Status = StatusLeased
			task.Attempt = 1
			task.LeaseOwner = "w"
			task.LeaseExpiresAt = now.Add(time.Second)
		}, "lease_token"},
		{"leased no expiry", func(task *Task) {
			task.Status = StatusLeased
			task.Attempt = 1
			task.LeaseOwner = "w"
			task.LeaseToken = 1
		}, "lease_expires_at"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := valid
			tt.mutate(&task)
			err := task.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
