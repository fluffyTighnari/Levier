// Middleware — internal/connection
//
// DaemonHub owns daemon gRPC streams, Hello authentication, heartbeats, and
// outbound sends.
//
// Copyright 2026 The Middleware Authors.
package connection

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	runtimev1 "github.com/fluffyTighnari/middleware/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/middleware/internal/protocol"
)

const (
	defaultHelloTimeout     = 3 * time.Second
	defaultHeartbeatTimeout = 30 * time.Second
	defaultOutboundBuffer   = 256
)

var (
	// ErrDaemonOffline reports that the daemon has no active stream.
	ErrDaemonOffline = errors.New("daemon is offline")
	// ErrBackpressure reports a full outbound buffer.
	ErrBackpressure = errors.New("daemon outbound buffer is full")
	// ErrHubClosed reports sends to a closed hub.
	ErrHubClosed = errors.New("daemon hub is closed")
)

// Authenticator validates a Hello daemon_id/auth_token pair.
type Authenticator func(daemonID, token string) bool

// RunRef identifies one run still executing on a daemon.
type RunRef struct {
	RunID      string
	LeaseEpoch int64
}

// DaemonInfo is the in-memory registration snapshot for one daemon.
type DaemonInfo struct {
	DaemonID            string
	RuntimeType         runtimev1.RuntimeType
	MaxConcurrentRuns   int32
	SupportedStrategies []string
	Version             string
	Labels              map[string]string
	RunningRuns         []RunRef
	RunningCount        int32
	ConnectedAt         time.Time
	LastHeartbeat       time.Time
}

// Config controls DaemonHub behavior.
type Config struct {
	HelloTimeout     time.Duration
	HeartbeatTimeout time.Duration
	OutboundBuffer   int
	Authenticate     Authenticator
	OnEnvelope       func(daemonID string, env *runtimev1.Envelope)
	Now              func() time.Time
}

// Hub implements the RuntimeService daemon side of the connection plane.
type Hub struct {
	runtimev1.UnimplementedRuntimeServiceServer

	cfg Config

	mu      sync.RWMutex
	daemons map[string]*daemonConn
	closed  bool
}

type daemonConn struct {
	info   DaemonInfo
	stream runtimev1.RuntimeService_ChannelServer
	sendCh chan *runtimev1.Envelope
	done   chan struct{}
	once   sync.Once
}

// New returns a DaemonHub with defaults applied.
func New(cfg Config) *Hub {
	if cfg.HelloTimeout <= 0 {
		cfg.HelloTimeout = defaultHelloTimeout
	}
	if cfg.HeartbeatTimeout <= 0 {
		cfg.HeartbeatTimeout = defaultHeartbeatTimeout
	}
	if cfg.OutboundBuffer <= 0 {
		cfg.OutboundBuffer = defaultOutboundBuffer
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Hub{cfg: cfg, daemons: make(map[string]*daemonConn)}
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
		DaemonID:            hello.GetDaemonId(),
		RuntimeType:         hello.GetRuntimeType(),
		MaxConcurrentRuns:   hello.GetMaxConcurrentRuns(),
		SupportedStrategies: append([]string(nil), hello.GetSupportedStrategies()...),
		Version:             hello.GetVersion(),
		Labels:              cloneStringMap(hello.GetLabels()),
		ConnectedAt:         now,
		LastHeartbeat:       now,
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
	h.mu.Unlock()

	defer h.remove(conn)
	go conn.writeLoop()

	return h.readLoop(conn)
}

// Send queues one envelope to a daemon.
func (h *Hub) Send(daemonID string, env *runtimev1.Envelope) error {
	if env == nil {
		return fmt.Errorf("envelope is nil")
	}
	if err := protocol.Validate(env); err != nil {
		return err
	}

	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return ErrHubClosed
	}
	conn := h.daemons[daemonID]
	h.mu.RUnlock()
	if conn == nil {
		return ErrDaemonOffline
	}

	select {
	case <-conn.done:
		return ErrDaemonOffline
	default:
	}
	select {
	case conn.sendCh <- env:
		return nil
	case <-conn.done:
		return ErrDaemonOffline
	default:
		return ErrBackpressure
	}
}

// Get returns a daemon registration snapshot.
func (h *Hub) Get(daemonID string) (DaemonInfo, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn := h.daemons[daemonID]
	if conn == nil {
		return DaemonInfo{}, false
	}
	return cloneInfo(conn.info), true
}

// List returns registrations sorted by daemon_id.
func (h *Hub) List() []DaemonInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]DaemonInfo, 0, len(h.daemons))
	for _, conn := range h.daemons {
		result = append(result, cloneInfo(conn.info))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].DaemonID < result[j].DaemonID
	})
	return result
}

// Count returns the number of active daemon streams.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.daemons)
}

// PickHealthy returns the least-loaded matching daemon.
func (h *Hub) PickHealthy(
	runtimeType runtimev1.RuntimeType,
	strategy string,
	labels map[string]string,
) (string, bool) {
	now := h.cfg.Now()
	h.mu.RLock()
	defer h.mu.RUnlock()

	bestID := ""
	bestScore := 0.0
	for _, conn := range h.daemons {
		info := conn.info
		if !h.healthy(info, now) {
			continue
		}
		if runtimeType != runtimev1.RuntimeType_RUNTIME_TYPE_UNSPECIFIED &&
			info.RuntimeType != runtimeType {
			continue
		}
		if strategy != "" && !contains(info.SupportedStrategies, strategy) {
			continue
		}
		if !labelsMatch(info.Labels, labels) {
			continue
		}
		maxRuns := info.MaxConcurrentRuns
		if maxRuns <= 0 {
			maxRuns = 1
		}
		if info.RunningCount >= maxRuns {
			continue
		}
		score := float64(info.RunningCount) / float64(maxRuns)
		if bestID == "" || score < bestScore {
			bestID = info.DaemonID
			bestScore = score
		}
	}
	return bestID, bestID != ""
}

// Close stops accepting new streams and marks existing connections offline.
func (h *Hub) Close() {
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
}

func (h *Hub) receiveHello(stream runtimev1.RuntimeService_ChannelServer) (*runtimev1.Envelope, error) {
	type result struct {
		env *runtimev1.Envelope
		err error
	}
	ch := make(chan result, 1)
	go func() {
		env, err := stream.Recv()
		ch <- result{env: env, err: err}
	}()

	timer := time.NewTimer(h.cfg.HelloTimeout)
	defer timer.Stop()
	select {
	case <-stream.Context().Done():
		return nil, stream.Context().Err()
	case <-timer.C:
		return nil, status.Errorf(codes.DeadlineExceeded, "hello not received within %s", h.cfg.HelloTimeout)
	case got := <-ch:
		if got.err != nil {
			if errors.Is(got.err, io.EOF) {
				return nil, status.Error(codes.InvalidArgument, "stream closed before hello")
			}
			return nil, got.err
		}
		return got.env, nil
	}
}

func (h *Hub) readLoop(conn *daemonConn) error {
	for {
		env, err := conn.stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := protocol.Validate(env); err != nil {
			return status.Error(codes.InvalidArgument, err.Error())
		}
		switch payload := env.GetPayload().(type) {
		case *runtimev1.Envelope_Hello:
			return status.Error(codes.InvalidArgument, "duplicate hello")
		case *runtimev1.Envelope_Heartbeat:
			h.updateHeartbeat(conn, payload.Heartbeat.GetRunningCount())
		default:
			if h.cfg.OnEnvelope != nil {
				h.cfg.OnEnvelope(conn.info.DaemonID, env)
			}
		}
	}
}

func (h *Hub) updateHeartbeat(conn *daemonConn, runningCount int32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	current := h.daemons[conn.info.DaemonID]
	if current != conn {
		return
	}
	conn.info.LastHeartbeat = h.cfg.Now()
	if runningCount >= 0 {
		conn.info.RunningCount = runningCount
	}
}

func (h *Hub) remove(conn *daemonConn) {
	h.mu.Lock()
	if current := h.daemons[conn.info.DaemonID]; current == conn {
		delete(h.daemons, conn.info.DaemonID)
	}
	h.mu.Unlock()
	conn.close()
}

func (h *Hub) healthy(info DaemonInfo, now time.Time) bool {
	last := info.LastHeartbeat
	if last.IsZero() {
		last = info.ConnectedAt
	}
	return now.Sub(last) <= h.cfg.HeartbeatTimeout
}

func (c *daemonConn) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case env := <-c.sendCh:
			if err := c.stream.Send(env); err != nil {
				c.close()
				return
			}
		}
	}
}

func (c *daemonConn) close() {
	c.once.Do(func() {
		close(c.done)
	})
}

func cloneInfo(info DaemonInfo) DaemonInfo {
	info.SupportedStrategies = append([]string(nil), info.SupportedStrategies...)
	info.Labels = cloneStringMap(info.Labels)
	info.RunningRuns = append([]RunRef(nil), info.RunningRuns...)
	return info
}

func cloneStringMap(value map[string]string) map[string]string {
	if value == nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func labelsMatch(have, want map[string]string) bool {
	for key, value := range want {
		if have[key] != value {
			return false
		}
	}
	return true
}
