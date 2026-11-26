// Levier - internal Control service tests.
//
// Copyright 2026 The Levier Authors.
package controlservice

import (
	"context"
	"encoding/json"
	"testing"

	rolev1 "github.com/fluffyTighnari/levier/gen/middleware/role/v1"
	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/runstore"
)

type fakeController struct {
	envelopes []string
	tools     int
	reconcile []connection.DaemonInfo
	offline   []string
}

func (f *fakeController) HandleEnvelope(daemonID string, env *runtimev1.Envelope) {
	kind := "unknown"
	switch {
	case env.GetEvent() != nil:
		kind = "event"
	case env.GetCheckpoint() != nil:
		kind = "checkpoint"
	}
	f.envelopes = append(f.envelopes, daemonID+":"+kind)
}

func (f *fakeController) DispatchToolCall(string, *runtimev1.Envelope) {
	f.tools++
}

func (f *fakeController) ReconcileDaemon(info connection.DaemonInfo) {
	f.reconcile = append(f.reconcile, info)
}

func (f *fakeController) HandleDaemonOffline(daemonID string) {
	f.offline = append(f.offline, daemonID)
}

func (f *fakeController) CancelRun(context.Context, string, string) (runstore.Run, error) {
	return runstore.Run{RunID: "run-1", Status: runstore.StatusCancelled}, nil
}

func (f *fakeController) ResumeRun(context.Context, string, json.RawMessage) (runstore.Run, error) {
	return runstore.Run{RunID: "run-1", SessionID: "session-1", Status: runstore.StatusQueued}, nil
}

func TestDeliverEnvelopeRoutesToolCalls(t *testing.T) {
	controller := &fakeController{}
	service := New(controller)
	_, err := service.DeliverEnvelope(context.Background(), &rolev1.DeliverEnvelopeRequest{
		DaemonId: "daemon-1",
		Envelope: &runtimev1.Envelope{Payload: &runtimev1.Envelope_ToolCallRequest{ToolCallRequest: &runtimev1.ToolCallRequest{}}},
	})
	if err != nil {
		t.Fatalf("DeliverEnvelope() error = %v", err)
	}
	if controller.tools != 1 || len(controller.envelopes) != 0 {
		t.Fatalf("tool routing = tools:%d envelopes:%v", controller.tools, controller.envelopes)
	}

	if _, err := service.DeliverEnvelope(context.Background(), &rolev1.DeliverEnvelopeRequest{
		DaemonId: "daemon-1",
		Envelope: &runtimev1.Envelope{Payload: &runtimev1.Envelope_Event{Event: &runtimev1.Event{}}},
	}); err != nil {
		t.Fatalf("DeliverEnvelope(event) error = %v", err)
	}
	if len(controller.envelopes) != 1 || controller.envelopes[0] != "daemon-1:event" {
		t.Fatalf("event routing = %v", controller.envelopes)
	}
}

func TestUpdateDaemonStateRoutesOnlineAndOffline(t *testing.T) {
	controller := &fakeController{}
	service := New(controller)
	if _, err := service.UpdateDaemonState(context.Background(), &rolev1.DaemonStateRequest{
		DaemonId:      "daemon-1",
		Status:        "online",
		RuntimeName:   "echo1",
		RuntimeType:   "normal",
		ConnectedAtMs: 1234,
	}); err != nil {
		t.Fatalf("UpdateDaemonState(online) error = %v", err)
	}
	if len(controller.reconcile) != 1 || controller.reconcile[0].DaemonID != "daemon-1" {
		t.Fatalf("reconcile = %+v", controller.reconcile)
	}
	if _, err := service.UpdateDaemonState(context.Background(), &rolev1.DaemonStateRequest{
		DaemonId: "daemon-1",
		Status:   "offline",
	}); err != nil {
		t.Fatalf("UpdateDaemonState(offline) error = %v", err)
	}
	if len(controller.offline) != 1 || controller.offline[0] != "daemon-1" {
		t.Fatalf("offline = %v", controller.offline)
	}
}
