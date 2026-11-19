// Levier — internal/testutil/fakedaemon
//
// Fake daemon behavior tests over an in-memory gRPC connection.
//
// Copyright 2026 The Levier Authors.
package fakedaemon

import (
	"context"
	"net"
	"testing"
	"time"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/protocol"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type testMW struct {
	runtimev1.UnimplementedRuntimeServiceServer
	received chan *runtimev1.Envelope
	done     chan struct{}
}

func (s *testMW) Channel(stream runtimev1.RuntimeService_ChannelServer) error {
	defer close(s.done)
	for {
		env, err := stream.Recv()
		if err != nil {
			return err
		}
		select {
		case s.received <- env:
		default:
		}

		switch {
		case env.GetHello() != nil:
			assign, err := protocol.NewRun("run-1", 1, 1, &runtimev1.TaskAssign{
				RunId: "run-1",
				AppId: "app-1",
				Model: &runtimev1.ModelProfile{
					Provider: "openai",
					Model:    "test",
				},
				Timeouts: &runtimev1.TimeoutProfile{
					RunTimeoutMs:  60000,
					ToolTimeoutMs: 5000,
				},
			})
			if err != nil {
				return err
			}
			if err := stream.Send(assign); err != nil {
				return err
			}
		case env.GetToolCallRequest() != nil:
			req := env.GetToolCallRequest()
			result, err := protocol.NewResponse(env, &runtimev1.ToolCallResult{
				InvocationId: req.GetInvocationId(),
				Ok:           true,
				ResultJson:   `{"ok":true}`,
			})
			if err != nil {
				return err
			}
			if err := stream.Send(result); err != nil {
				return err
			}
		}
	}
}

func startTestMW(t *testing.T) (*testMW, *grpc.ClientConn) {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	mw := &testMW{
		received: make(chan *runtimev1.Envelope, 128),
		done:     make(chan struct{}),
	}
	runtimev1.RegisterRuntimeServiceServer(srv, mw)
	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return mw, conn
}

func waitEnvelope(t *testing.T, ch <-chan *runtimev1.Envelope, match func(*runtimev1.Envelope) bool) *runtimev1.Envelope {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case env := <-ch:
			if match(env) {
				return env
			}
		case <-deadline:
			t.Fatal("timed out waiting for envelope")
		}
	}
}

func TestAutoEchoSequence(t *testing.T) {
	mw, conn := startTestMW(t)
	cfg := DefaultConfig()
	cfg.AutoEcho = true
	d, err := New(context.Background(), conn, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()

	hello := waitEnvelope(t, mw.received, func(env *runtimev1.Envelope) bool {
		return env.GetHello() != nil
	})
	if hello.GetHello().GetDaemonId() != cfg.DaemonID {
		t.Fatalf("hello daemon_id = %q", hello.GetHello().GetDaemonId())
	}

	var ackSeen bool
	var eventTypes []runtimev1.EventType
	for len(eventTypes) < 3 {
		env := waitEnvelope(t, mw.received, func(*runtimev1.Envelope) bool { return true })
		if env.GetTaskAck() != nil {
			ackSeen = env.GetTaskAck().GetAccepted()
		}
		if event := env.GetEvent(); event != nil {
			eventTypes = append(eventTypes, event.GetType())
			if event.GetSeq() != int64(len(eventTypes)) {
				t.Fatalf("event seq = %d, want %d", event.GetSeq(), len(eventTypes))
			}
		}
	}
	if !ackSeen {
		t.Fatal("task ack was not accepted")
	}
	want := []runtimev1.EventType{
		runtimev1.EventType_RUN_STATUS,
		runtimev1.EventType_MESSAGE_COMPLETED,
		runtimev1.EventType_RUN_STATUS,
	}
	for i := range want {
		if eventTypes[i] != want[i] {
			t.Fatalf("event[%d] = %v, want %v", i, eventTypes[i], want[i])
		}
	}
}

func TestRejectAssignment(t *testing.T) {
	mw, conn := startTestMW(t)
	cfg := DefaultConfig()
	cfg.Accept = false
	cfg.RejectReason = "full"
	d, err := New(context.Background(), conn, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()

	ackEnv := waitEnvelope(t, mw.received, func(env *runtimev1.Envelope) bool {
		return env.GetTaskAck() != nil
	})
	ack := ackEnv.GetTaskAck()
	if ack.GetAccepted() {
		t.Fatal("task ack accepted = true, want false")
	}
	if ack.GetReason() != "full" {
		t.Fatalf("task ack reason = %q", ack.GetReason())
	}
}

func TestRequestTool(t *testing.T) {
	mw, conn := startTestMW(t)
	cfg := DefaultConfig()
	d, err := New(context.Background(), conn, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()

	// Wait until the server has processed Hello and sent TaskAssign.
	waitEnvelope(t, mw.received, func(env *runtimev1.Envelope) bool {
		return env.GetHello() != nil
	})

	result, err := d.RequestTool(context.Background(), "run-1", 1, 1, "echo", "{}", 1000)
	if err != nil {
		t.Fatalf("RequestTool() error = %v", err)
	}
	if !result.GetOk() || result.GetResultJson() != `{"ok":true}` {
		t.Fatalf("result = %+v", result)
	}
}

func TestDisconnectAfterAssignment(t *testing.T) {
	mw, conn := startTestMW(t)
	cfg := DefaultConfig()
	cfg.DisconnectAfterAssignments = 1
	d, err := New(context.Background(), conn, cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer d.Close()

	select {
	case <-mw.done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for daemon disconnect")
	}
}
