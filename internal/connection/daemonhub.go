// Levier — connection
//
// Split from daemonhub.go (P8R.3).
// Copyright 2026 The Levier Authors.
package connection

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/protocol"
)

const (
	defaultHelloTimeout         = 3 * time.Second
	defaultHeartbeatTimeout     = 30 * time.Second
	defaultOutboundBuffer       = 256
	defaultCloseTimeout         = 5 * time.Second
	defaultSendTimeout          = 100 * time.Millisecond
	defaultSlowConsumerTime     = 5 * time.Second
	defaultHeartbeatCheckPeriod = 5 * time.Second
)

var (
	// ErrDaemonOffline reports that the daemon has no active stream.
	ErrDaemonOffline = errors.New("daemon is offline")
	// ErrBackpressure reports a full outbound buffer.
	ErrBackpressure = errors.New("daemon outbound buffer is full")
	// ErrSlowConsumer reports a daemon that cannot drain control traffic.
	ErrSlowConsumer = errors.New("daemon is a slow consumer")
	// ErrHubClosed reports sends to a closed hub.
	ErrHubClosed = errors.New("daemon hub is closed")
)

// Authenticator validates a Hello daemon_id/auth_token pair.
type Authenticator func(daemonID, token string) bool

// SendKind classifies outbound traffic for backpressure handling.
type SendKind int

const (
	// SendControl covers control messages that must not be dropped.
	SendControl SendKind = iota
	// SendPersistent covers non-ephemeral events.
	SendPersistent
	// SendEphemeral covers ephemeral events that may be dropped.
	SendEphemeral
)

// RunRef identifies one run still executing on a daemon.
type RunRef struct {
	RunID      string
	LeaseEpoch int64
}

// DaemonInfo is the in-memory registration snapshot for one daemon.
type DaemonInfo struct {
	DaemonID          string
	Status            string
	RuntimeName       string
	RuntimeType       runtimev1.RuntimeType
	MaxConcurrentRuns int32
	Version           string
	Labels            map[string]string
	RunningRuns       []RunRef
	RunningCount      int32
	Reserved          int32
	Healthy           bool
	ConnectedAt       time.Time
	LastHeartbeat     time.Time
}

// Config controls DaemonHub behavior.
type Config struct {
	HelloTimeout         time.Duration
	HeartbeatTimeout     time.Duration
	CloseTimeout         time.Duration
	SendTimeout          time.Duration
	SlowConsumerTimeout  time.Duration
	HeartbeatCheckPeriod time.Duration
	OutboundBuffer       int
	Authenticate         Authenticator
	OnEnvelope           func(daemonID string, env *runtimev1.Envelope)
	OnHello              func(info DaemonInfo)
	OnHeartbeat          func(daemonID string, runningCount int32)
	OnOffline            func(daemonID string)
	OnError              func(error)
	Now                  func() time.Time
}

// Hub implements the RuntimeService daemon side of the connection plane.
type Hub struct {
	runtimev1.UnimplementedRuntimeServiceServer

	cfg Config

	mu         sync.RWMutex
	daemons    map[string]*daemonConn
	onEnvelope func(daemonID string, env *runtimev1.Envelope)
	onHello    func(info DaemonInfo)
	onOffline  func(daemonID string)
	closed     bool
	wg         sync.WaitGroup
	stopCh     chan struct{}
	stopOnce   sync.Once
}

type daemonConn struct {
	info   DaemonInfo
	stream runtimev1.RuntimeService_ChannelServer
	sendCh chan *runtimev1.Envelope
	done   chan struct{}
	once   sync.Once

	slowMu           sync.Mutex
	slow             bool
	slowTimer        *time.Timer
	droppedEphemeral atomic.Int64
}

// New returns a DaemonHub with defaults applied.
func New(cfg Config) *Hub {
	if cfg.HelloTimeout <= 0 {
		cfg.HelloTimeout = defaultHelloTimeout
	}
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = defaultHeartbeatTimeout
	}
	if cfg.CloseTimeout <= 0 {
		cfg.CloseTimeout = defaultCloseTimeout
	}
	if cfg.SendTimeout <= 0 {
		cfg.SendTimeout = defaultSendTimeout
	}
	if cfg.SlowConsumerTimeout <= 0 {
		cfg.SlowConsumerTimeout = defaultSlowConsumerTime
	}
	if cfg.HeartbeatCheckPeriod <= 0 {
		cfg.HeartbeatCheckPeriod = defaultHeartbeatCheckPeriod
	}
	if cfg.OutboundBuffer <= 0 {
		cfg.OutboundBuffer = defaultOutboundBuffer
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	hub := &Hub{
		cfg:        cfg,
		daemons:    make(map[string]*daemonConn),
		onEnvelope: cfg.OnEnvelope,
		onHello:    cfg.OnHello,
		onOffline:  cfg.OnOffline,
		stopCh:     make(chan struct{}),
	}
	go hub.reaper()
	return hub
}

// SetOnEnvelope replaces the inbound envelope callback.
func (h *Hub) SetOnEnvelope(handler func(daemonID string, env *runtimev1.Envelope)) {
	h.mu.Lock()
	h.onEnvelope = handler
	h.mu.Unlock()
}

// SetOnHello replaces the Hello callback.
func (h *Hub) SetOnHello(handler func(info DaemonInfo)) {
	h.mu.Lock()
	h.onHello = handler
	h.mu.Unlock()
}

// SetOnOffline replaces the offline callback.
func (h *Hub) SetOnOffline(handler func(daemonID string)) {
	h.mu.Lock()
	h.onOffline = handler
	h.mu.Unlock()
}

// Register registers the hub as the RuntimeService implementation.
func (h *Hub) Register(server grpc.ServiceRegistrar) {
	runtimev1.RegisterRuntimeServiceServer(server, h)
}

// Channel handles one daemon RuntimeChannel stream.
func (h *Hub) Channel(stream runtimev1.RuntimeService_ChannelServer) error {
	first, err := h.receiveHello(stream)
	if err != nil {
		return err
	}
	if err := protocol.Validate(first); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	hello := first.GetHello()
	if hello == nil {
		return status.Error(codes.InvalidArgument, "first message must be hello")
	}
	if h.cfg.Authenticate == nil || !h.cfg.Authenticate(hello.GetDaemonId(), hello.GetAuthToken()) {
		return status.Error(codes.Unauthenticated, "invalid daemon credentials")
	}

	now := h.cfg.Now()
	info := DaemonInfo{
		DaemonID:          hello.GetDaemonId(),
		Status:            "online",
		RuntimeName:       hello.GetRuntimeName(),
		RuntimeType:       hello.GetRuntimeType(),
		MaxConcurrentRuns: hello.GetMaxConcurrentRuns(),
		Version:           hello.GetVersion(),
		Labels:            cloneStringMap(hello.GetLabels()),
		ConnectedAt:       now,
		LastHeartbeat:     now,
	}
	for _, running := range hello.GetRunningRuns() {
		info.RunningRuns = append(info.RunningRuns, RunRef{
			RunID:      running.GetRunId(),
			LeaseEpoch: running.GetLeaseEpoch(),
		})
	}
	info.RunningCount = int32(len(info.RunningRuns))

	conn := &daemonConn{
		info:   info,
		stream: stream,
		sendCh: make(chan *runtimev1.Envelope, h.cfg.OutboundBuffer),
		done:   make(chan struct{}),
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return status.Error(codes.Unavailable, "daemon hub is closed")
	}
	if _, exists := h.daemons[info.DaemonID]; exists {
		h.mu.Unlock()
		return status.Errorf(
			codes.AlreadyExists,
			"daemon %q already has an active stream",
			info.DaemonID,
		)
	}
	h.daemons[info.DaemonID] = conn
	h.wg.Add(1)
	h.mu.Unlock()

	h.notifyHello(info)
	defer h.wg.Done()
	defer func() {
		if h.remove(conn) {
			h.notifyOffline(conn.info.DaemonID)
		}
	}()
	go conn.writeLoop()

	return h.readLoop(conn)
}

// Close stops accepting new streams and marks existing connections offline.
func (h *Hub) Close() {
	h.stopOnce.Do(func() { close(h.stopCh) })
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	conns := make([]*daemonConn, 0, len(h.daemons))
	for _, conn := range h.daemons {
		conns = append(conns, conn)
	}
	h.daemons = make(map[string]*daemonConn)
	h.mu.Unlock()

	for _, conn := range conns {
		conn.close()
	}

	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(h.cfg.CloseTimeout):
	}
}

func (h *Hub) remove(conn *daemonConn) bool {
	h.mu.Lock()
	current := h.daemons[conn.info.DaemonID]
	if current == conn {
		delete(h.daemons, conn.info.DaemonID)
	}
	h.mu.Unlock()
	conn.close()
	return current == conn
}
