// Middleware — internal/connection
//
// DaemonHub gRPC integration tests.
//
// Copyright 2026 The Middleware Authors.
package connection

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/protocol"
	"github.com/fluffyTighnari/middleware/internal/testutil/fakedaemon"
)

func startHub(t *testing.T, cfg Config) (*Hub, string) {
	t.Helper()
	hub := New(cfg)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	hub.Register(server)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		hub.Close()
		_ = listener.Close()
	})
	return hub, listener.Addr().String()
}

func waitFor(t *testing.T, timeout time.Duration, predicate func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if predicate() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func waitDaemonError(t *testing.T, daemon *fakedaemon.Daemon) error {
	t.Helper()
	var err error
	if !waitFor(t, 2*time.Second, func() bool {
		err = daemon.Err()
		return err != nil
	}) {
		t.Fatal("daemon did not receive a stream error")
	}
	return err
}

func TestHelloAuthHeartbeatAndCleanup(t *testing.T) {
	hub, addr := startHub(t, Config{
		HelloTimeout:     time.Second,
		HeartbeatTimeout: time.Second,
		Authenticate: func(daemonID, token string) bool {
			return daemonID == "daemon-1" && token == "token-1"
		},
	})

	cfg := fakedaemon.DefaultConfig()
	cfg.DaemonID = "daemon-1"
	cfg.AuthToken = "token-1"
	cfg.HeartbeatInterval = 20 * time.Millisecond
	daemon, err := fakedaemon.Dial(context.Background(), addr, cfg)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer daemon.Close()

	if !waitFor(t, time.Second, func() bool { return hub.Count() == 1 }) {
		t.Fatal("daemon was not registered")
	}
	info, ok := hub.Get("daemon-1")
	if !ok {
		t.Fatal("Get() = false")
	}
	if info.RuntimeType != runtimev1.RuntimeType_NORMAL || info.MaxConcurrentRuns != 4 {
		t.Fatalf("info = %+v", info)
	}
	if len(info.SupportedStrategies) != 2 {
		t.Fatalf("supported strategies = %v", info.SupportedStrategies)
	}
	firstHeartbeat := info.LastHeartbeat
	if !waitFor(t, time.Second, func() bool {
		current, ok := hub.Get("daemon-1")
		return ok && current.LastHeartbeat.After(firstHeartbeat)
	}) {
		t.Fatal("heartbeat was not applied")
	}

	if err := daemon.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !waitFor(t, time.Second, func() bool { return hub.Count() == 0 }) {
		t.Fatal("daemon was not removed after disconnect")
	}
}

func TestHelloTimeout(t *testing.T) {
	_, addr := startHub(t, Config{
		HelloTimeout: 50 * time.Millisecond,
		Authenticate: func(daemonID, token string) bool { return true },
	})
	cfg := fakedaemon.DefaultConfig()
	cfg.SkipHello = true
	daemon, err := fakedaemon.Dial(context.Background(), addr, cfg)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer daemon.Close()

	err = waitDaemonError(t, daemon)
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("error code = %v, want DeadlineExceeded; err=%v", status.Code(err), err)
	}
}

func TestInvalidToken(t *testing.T) {
	hub, addr := startHub(t, Config{
		HelloTimeout: time.Second,
		Authenticate: func(daemonID, token string) bool {
			return daemonID == "daemon-1" && token == "token-1"
		},
	})
	cfg := fakedaemon.DefaultConfig()
	cfg.DaemonID = "daemon-1"
	cfg.AuthToken = "wrong"
	daemon, err := fakedaemon.Dial(context.Background(), addr, cfg)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer daemon.Close()

	err = waitDaemonError(t, daemon)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("error code = %v, want Unauthenticated; err=%v", status.Code(err), err)
	}
	if hub.Count() != 0 {
		t.Fatalf("hub count = %d, want 0", hub.Count())
	}
}

func TestDuplicateDaemonIDIsRejected(t *testing.T) {
	hub, addr := startHub(t, Config{
		HelloTimeout: time.Second,
		Authenticate: func(daemonID, token string) bool { return token == "token-1" },
	})
	cfg := fakedaemon.DefaultConfig()
	cfg.DaemonID = "daemon-1"
	cfg.AuthToken = "token-1"
	first, err := fakedaemon.Dial(context.Background(), addr, cfg)
	if err != nil {
		t.Fatalf("first Dial() error = %v", err)
	}
	defer first.Close()
	received := make(chan *runtimev1.Envelope, 1)
	first.SetOnMessage(func(_ context.Context, env *runtimev1.Envelope, _ *fakedaemon.Daemon) error {
		select {
		case received <- env:
		default:
		}
		return nil
	})
	if !waitFor(t, time.Second, func() bool { return hub.Count() == 1 }) {
		t.Fatal("first daemon was not registered")
	}

	second, err := fakedaemon.Dial(context.Background(), addr, cfg)
	if err != nil {
		t.Fatalf("second Dial() error = %v", err)
	}
	defer second.Close()
	err = waitDaemonError(t, second)
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("error code = %v, want AlreadyExists; err=%v", status.Code(err), err)
	}
	if hub.Count() != 1 {
		t.Fatalf("hub count = %d, want 1", hub.Count())
	}

	env, err := protocol.NewRun("run-1", 1, 1, &runtimev1.Event{
		Seq: 1, Type: runtimev1.EventType_RUN_STATUS, Source: protocol.EventSourceDaemon,
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := hub.Send("daemon-1", env); err != nil {
		t.Fatalf("Send() after duplicate error = %v", err)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("first connection did not receive the event")
	}
}

func TestSendAndOffline(t *testing.T) {
	hub, addr := startHub(t, Config{
		HelloTimeout: time.Second,
		Authenticate: func(daemonID, token string) bool { return true },
	})
	cfg := fakedaemon.DefaultConfig()
	cfg.DaemonID = "daemon-1"
	daemon, err := fakedaemon.Dial(context.Background(), addr, cfg)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer daemon.Close()

	received := make(chan *runtimev1.Envelope, 1)
	daemon.SetOnMessage(func(_ context.Context, env *runtimev1.Envelope, _ *fakedaemon.Daemon) error {
		select {
		case received <- env:
		default:
		}
		return nil
	})
	if !waitFor(t, time.Second, func() bool { return hub.Count() == 1 }) {
		t.Fatal("daemon was not registered")
	}

	data, err := structpb.NewStruct(map[string]any{"status": "running"})
	if err != nil {
		t.Fatalf("NewStruct() error = %v", err)
	}
	event := &runtimev1.Event{
		Seq:    1,
		Type:   runtimev1.EventType_RUN_STATUS,
		Source: protocol.EventSourceDaemon,
		Data:   data,
	}
	env, err := protocol.NewRun("run-1", 1, 1, event)
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := hub.Send("daemon-1", env); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	select {
	case got := <-received:
		if got.GetEvent().GetType() != runtimev1.EventType_RUN_STATUS {
			t.Fatalf("received payload = %T", got.GetPayload())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not receive event")
	}

	if err := hub.Send("missing", env); err != ErrDaemonOffline {
		t.Fatalf("Send(missing) error = %v, want ErrDaemonOffline", err)
	}
}

func TestClassifyAndEphemeralDrop(t *testing.T) {
	hub := New(Config{})
	conn := &daemonConn{
		sendCh: make(chan *runtimev1.Envelope, 1),
		done:   make(chan struct{}),
	}
	hub.daemons["daemon-drop"] = conn

	ephemeral, err := protocol.NewRun("run-1", 1, 1, &runtimev1.Event{
		Seq:       0,
		Type:      runtimev1.EventType_MESSAGE_DELTA,
		Ephemeral: true,
		Source:    protocol.EventSourceDaemon,
	})
	if err != nil {
		t.Fatalf("NewRun(ephemeral) error = %v", err)
	}
	if ClassifyEnvelope(ephemeral) != SendEphemeral {
		t.Fatal("ephemeral envelope classified incorrectly")
	}
	if err := hub.Send("daemon-drop", ephemeral); err != nil {
		t.Fatalf("first ephemeral Send() error = %v", err)
	}
	if err := hub.Send("daemon-drop", ephemeral); err != nil {
		t.Fatalf("dropped ephemeral Send() error = %v", err)
	}
	if dropped := hub.DroppedEphemeral("daemon-drop"); dropped != 1 {
		t.Fatalf("dropped = %d, want 1", dropped)
	}

	control, err := protocol.NewRun("run-1", 1, 1, &runtimev1.Cancel{Reason: "test"})
	if err != nil {
		t.Fatalf("NewRun(control) error = %v", err)
	}
	if ClassifyEnvelope(control) != SendControl {
		t.Fatal("control envelope classified incorrectly")
	}
}

func TestControlBackpressureMarksSlowAndCloses(t *testing.T) {
	hub := New(Config{
		SendTimeout:         10 * time.Millisecond,
		SlowConsumerTimeout: 30 * time.Millisecond,
	})
	conn := &daemonConn{
		sendCh: make(chan *runtimev1.Envelope, 1),
		done:   make(chan struct{}),
	}
	hub.daemons["daemon-slow"] = conn
	control, err := protocol.NewRun("run-1", 1, 1, &runtimev1.Cancel{Reason: "test"})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	conn.sendCh <- control
	if err := hub.Send("daemon-slow", control); err != ErrBackpressure {
		t.Fatalf("Send() error = %v, want ErrBackpressure", err)
	}
	select {
	case <-conn.done:
	case <-time.After(time.Second):
		t.Fatal("slow consumer connection was not closed")
	}
}

func TestHubCloseStopsConnections(t *testing.T) {
	hub, addr := startHub(t, Config{
		HelloTimeout: time.Second,
		CloseTimeout: time.Second,
		Authenticate: func(daemonID, token string) bool { return true },
	})
	cfg := fakedaemon.DefaultConfig()
	cfg.DaemonID = "daemon-close"
	daemon, err := fakedaemon.Dial(context.Background(), addr, cfg)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer daemon.Close()
	if !waitFor(t, time.Second, func() bool { return hub.Count() == 1 }) {
		t.Fatal("daemon was not registered")
	}

	hub.Close()
	if hub.Count() != 0 {
		t.Fatalf("hub count after Close = %d", hub.Count())
	}
	env, err := protocol.NewRun("run-1", 1, 1, &runtimev1.Event{
		Seq: 1, Type: runtimev1.EventType_RUN_STATUS, Source: protocol.EventSourceDaemon,
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if err := hub.Send("daemon-close", env); err != ErrHubClosed {
		t.Fatalf("Send() after Close = %v, want ErrHubClosed", err)
	}
}

func TestPickHealthyLabelsAndStrategies(t *testing.T) {
	hub, addr := startHub(t, Config{
		HelloTimeout: time.Second,
		Authenticate: func(daemonID, token string) bool { return true },
	})
	firstCfg := fakedaemon.DefaultConfig()
	firstCfg.DaemonID = "daemon-a"
	firstCfg.Labels = map[string]string{"pool": "a", "region": "cn"}
	firstCfg.SupportedStrategies = []string{"echo"}
	first, err := fakedaemon.Dial(context.Background(), addr, firstCfg)
	if err != nil {
		t.Fatalf("first Dial() error = %v", err)
	}
	defer first.Close()
	secondCfg := fakedaemon.DefaultConfig()
	secondCfg.DaemonID = "daemon-b"
	secondCfg.Labels = map[string]string{"pool": "b", "region": "cn"}
	secondCfg.SupportedStrategies = []string{"echo"}
	second, err := fakedaemon.Dial(context.Background(), addr, secondCfg)
	if err != nil {
		t.Fatalf("second Dial() error = %v", err)
	}
	defer second.Close()
	if !waitFor(t, time.Second, func() bool { return hub.Count() == 2 }) {
		t.Fatal("daemons were not registered")
	}

	if id, ok := hub.PickHealthy(runtimev1.RuntimeType_NORMAL, "echo", map[string]string{"pool": "a"}); !ok || id != "daemon-a" {
		t.Fatalf("pool a = %q, %v", id, ok)
	}
	if id, ok := hub.PickHealthy(runtimev1.RuntimeType_NORMAL, "echo", map[string]string{"pool": "b"}); !ok || id != "daemon-b" {
		t.Fatalf("pool b = %q, %v", id, ok)
	}
	if _, ok := hub.PickHealthy(runtimev1.RuntimeType_NORMAL, "react", nil); ok {
		t.Fatal("unsupported react strategy matched")
	}
	if _, ok := hub.PickHealthy(runtimev1.RuntimeType_NORMAL, "echo", map[string]string{"pool": "missing"}); ok {
		t.Fatal("missing label matched")
	}
}

func TestPickHealthy(t *testing.T) {
	hub, addr := startHub(t, Config{
		HelloTimeout: time.Second,
		Authenticate: func(daemonID, token string) bool { return true },
	})
	cfg := fakedaemon.DefaultConfig()
	cfg.DaemonID = "daemon-1"
	cfg.AuthToken = "token-1"
	cfg.Labels = map[string]string{"pool": "a"}
	daemon, err := fakedaemon.Dial(context.Background(), addr, cfg)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer daemon.Close()
	if !waitFor(t, time.Second, func() bool { return hub.Count() == 1 }) {
		t.Fatal("daemon was not registered")
	}

	if id, ok := hub.PickHealthy(runtimev1.RuntimeType_NORMAL, "echo", map[string]string{"pool": "a"}); !ok || id != "daemon-1" {
		t.Fatalf("PickHealthy() = %q, %v", id, ok)
	}
	if _, ok := hub.PickHealthy(runtimev1.RuntimeType_LONGTERM, "echo", nil); ok {
		t.Fatal("PickHealthy() matched wrong runtime type")
	}
	if _, ok := hub.PickHealthy(runtimev1.RuntimeType_NORMAL, "unknown", nil); ok {
		t.Fatal("PickHealthy() matched unsupported strategy")
	}
}
