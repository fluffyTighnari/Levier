// Package controlremote binds Control's Runtime ports to internal RPC clients.
//
// The control/API role still uses PostgreSQL as the placement source. It
// resolves daemon ownership, acquires capacity leases, and sends only to the
// Runtime Gateway that owns the daemon stream.
//
// Copyright 2026 The Levier Authors.
package controlremote

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	rolev1 "github.com/fluffyTighnari/levier/gen/middleware/role/v1"
	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/cluster"
	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/control"
	"github.com/fluffyTighnari/levier/internal/internalapi"
	"github.com/fluffyTighnari/levier/internal/ports"
)

// Options configures the remote Runtime plane.
type Options struct {
	Store       *cluster.Store
	Token       string
	DialTimeout time.Duration
	Logger      *slog.Logger
}

// Plane implements the Runtime transport, dispatcher, placement and scheduler
// ports used by a split control/api process.
type Plane struct {
	store       *cluster.Store
	token       string
	dialTimeout time.Duration
	logger      *slog.Logger

	mu    sync.Mutex
	conns map[string]*gatewayClient
}

type gatewayClient struct {
	address string
	conn    *grpc.ClientConn
	client  rolev1.RuntimeGatewayServiceClient
}

// New returns a remote Runtime plane.
func New(opts Options) *Plane {
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = 3 * time.Second
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Plane{
		store:       opts.Store,
		token:       opts.Token,
		dialTimeout: opts.DialTimeout,
		logger:      opts.Logger,
		conns:       make(map[string]*gatewayClient),
	}
}

// Send forwards a non-dispatch envelope to the owning Runtime Gateway.
func (p *Plane) Send(daemonID string, env *runtimev1.Envelope) error {
	if env == nil {
		return fmt.Errorf("envelope is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.dialTimeout+2*time.Second)
	defer cancel()
	runtime, gateway, client, err := p.clientFor(ctx, daemonID)
	if err != nil {
		return err
	}
	_, err = client.Send(ctx, &rolev1.SendRequest{
		DaemonId:        daemonID,
		Envelope:        env,
		ConnectionEpoch: runtime.ConnectionEpoch,
	})
	if err != nil {
		p.dropGateway(gateway.GatewayID)
		return fmt.Errorf("send to runtime gateway %s: %w", gateway.GatewayID, err)
	}
	return nil
}

// Dispatch sends a TaskAssign and waits for the Runtime Gateway's synchronous
// TaskAck. Control does not keep a cross-process pending-ack map on this path.
func (p *Plane) Dispatch(
	ctx context.Context,
	daemonID string,
	env *runtimev1.Envelope,
	timeout time.Duration,
) (*runtimev1.TaskAck, error) {
	runtime, gateway, client, err := p.clientFor(ctx, daemonID)
	if err != nil {
		return nil, err
	}
	resp, err := client.Dispatch(ctx, &rolev1.DispatchRequest{
		DaemonId:        daemonID,
		Envelope:        env,
		AckTimeoutMs:    timeout.Milliseconds(),
		ConnectionEpoch: runtime.ConnectionEpoch,
	})
	if err != nil {
		p.dropGateway(gateway.GatewayID)
		if status.Code(err) == codes.DeadlineExceeded {
			return nil, connection.ErrAckTimeout
		}
		return nil, fmt.Errorf("dispatch to runtime gateway %s: %w", gateway.GatewayID, err)
	}
	if resp.GetAck() == nil {
		return nil, fmt.Errorf("runtime gateway returned no task ack")
	}
	return resp.GetAck(), nil
}

// Release releases the PostgreSQL capacity reservation.
func (p *Plane) Release(daemonID, reservationID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := p.store.ReleaseCapacityLease(ctx, daemonID, reservationID); err != nil {
		p.logger.Warn("release runtime capacity failed",
			"daemon_id", daemonID,
			"reservation_id", reservationID,
			"err", err,
		)
	}
}

// Reserve acquires one PostgreSQL capacity lease.
func (p *Plane) Reserve(daemonID, reservationID string, ttl time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	runtime, err := p.store.GetRuntime(ctx, daemonID)
	if err != nil {
		return false
	}
	if _, err := p.store.AcquireCapacityLease(ctx, daemonID, reservationID, runtime.GatewayID, ttl); err != nil {
		if !errors.Is(err, cluster.ErrCapacityExceeded) && !errors.Is(err, cluster.ErrLeaseLost) {
			p.logger.Warn("acquire runtime capacity failed",
				"daemon_id", daemonID,
				"reservation_id", reservationID,
				"err", err,
			)
		}
		return false
	}
	return true
}

// PickHealthy implements the fallback placement port.
func (p *Plane) PickHealthy(
	runtimeType runtimev1.RuntimeType,
	runtimeName string,
	labels map[string]string,
) (string, bool) {
	return p.Pick(control.PlacementRequest{
		Runtime: runtimeType, RuntimeName: runtimeName, Labels: labels,
	})
}

// Pick implements control.Scheduler using the PostgreSQL Runtime registry.
func (p *Plane) Pick(request control.PlacementRequest) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connections, err := p.store.ListOnlineRuntimes(ctx)
	if err != nil {
		p.logger.Warn("list online runtimes failed", "err", err)
		return "", false
	}
	now := time.Now().UTC()
	candidates := make([]control.Candidate, 0, len(connections))
	for _, connection := range connections {
		if connection.LeaseExpiresAt.Before(now) {
			continue
		}
		running := connection.RunningCount
		if connection.Reserved > running {
			running = connection.Reserved
		}
		candidates = append(candidates, control.Candidate{
			DaemonID:          connection.DaemonID,
			RuntimeName:       connection.RuntimeName,
			RuntimeType:       parseRuntimeType(connection.RuntimeType),
			Labels:            connection.Labels,
			MaxConcurrentRuns: int32(connection.MaxConcurrentRuns),
			RunningCount:      int32(running),
			Healthy:           connection.Status == cluster.StatusOnline,
		})
	}
	return control.Select(candidates, request)
}

// Disconnect closes the daemon stream on its owning Runtime Gateway.
func (p *Plane) Disconnect(daemonID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, gateway, client, err := p.clientFor(ctx, daemonID)
	if err != nil {
		return
	}
	if _, err := client.Disconnect(ctx, &rolev1.DisconnectRequest{DaemonId: daemonID}); err != nil {
		p.dropGateway(gateway.GatewayID)
		p.logger.Warn("disconnect runtime failed", "daemon_id", daemonID, "err", err)
	}
}

// DaemonAvailable reports whether the daemon has a live lease. It is used by
// Control's startup recovery pass.
func (p *Plane) DaemonAvailable(daemonID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	runtime, err := p.store.GetRuntime(ctx, daemonID)
	if err != nil {
		return false
	}
	return runtime.Status == cluster.StatusOnline && runtime.LeaseExpiresAt.After(time.Now().UTC())
}

// Close releases cached internal gRPC connections.
func (p *Plane) Close() {
	p.mu.Lock()
	conns := make([]*gatewayClient, 0, len(p.conns))
	for _, item := range p.conns {
		conns = append(conns, item)
	}
	p.conns = make(map[string]*gatewayClient)
	p.mu.Unlock()
	for _, item := range conns {
		_ = item.conn.Close()
	}
}

func (p *Plane) clientFor(
	ctx context.Context,
	daemonID string,
) (cluster.RuntimeConnection, cluster.GatewayInstance, rolev1.RuntimeGatewayServiceClient, error) {
	runtime, err := p.store.GetRuntime(ctx, daemonID)
	if err != nil {
		return cluster.RuntimeConnection{}, cluster.GatewayInstance{}, nil, fmt.Errorf("resolve runtime %s: %w", daemonID, err)
	}
	if runtime.Status != cluster.StatusOnline || !runtime.LeaseExpiresAt.After(time.Now().UTC()) {
		return cluster.RuntimeConnection{}, cluster.GatewayInstance{}, nil, connection.ErrDaemonOffline
	}
	gateway, err := p.store.GetGateway(ctx, runtime.GatewayID)
	if err != nil {
		return cluster.RuntimeConnection{}, cluster.GatewayInstance{}, nil, fmt.Errorf("resolve gateway %s: %w", runtime.GatewayID, err)
	}
	if gateway.Status != cluster.StatusOnline || !gateway.LeaseExpiresAt.After(time.Now().UTC()) {
		return cluster.RuntimeConnection{}, cluster.GatewayInstance{}, nil, connection.ErrDaemonOffline
	}
	if strings.TrimSpace(gateway.InternalAddress) == "" {
		return cluster.RuntimeConnection{}, cluster.GatewayInstance{}, nil, fmt.Errorf("gateway %s has no internal address", gateway.GatewayID)
	}
	client, err := p.gatewayClient(ctx, gateway)
	if err != nil {
		return cluster.RuntimeConnection{}, cluster.GatewayInstance{}, nil, err
	}
	return runtime, gateway, client, nil
}

func (p *Plane) gatewayClient(ctx context.Context, gateway cluster.GatewayInstance) (rolev1.RuntimeGatewayServiceClient, error) {
	p.mu.Lock()
	if cached := p.conns[gateway.GatewayID]; cached != nil && cached.address == gateway.InternalAddress {
		client := cached.client
		p.mu.Unlock()
		return client, nil
	}
	if cached := p.conns[gateway.GatewayID]; cached != nil {
		delete(p.conns, gateway.GatewayID)
		_ = cached.conn.Close()
	}
	p.mu.Unlock()

	conn, err := internalapi.DialWithTimeout(ctx, gateway.InternalAddress, p.token, p.dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial runtime gateway %s: %w", gateway.GatewayID, err)
	}
	client := rolev1.NewRuntimeGatewayServiceClient(conn)
	p.mu.Lock()
	p.conns[gateway.GatewayID] = &gatewayClient{
		address: gateway.InternalAddress,
		conn:    conn,
		client:  client,
	}
	p.mu.Unlock()
	return client, nil
}

func (p *Plane) dropGateway(gatewayID string) {
	p.mu.Lock()
	cached := p.conns[gatewayID]
	delete(p.conns, gatewayID)
	p.mu.Unlock()
	if cached != nil {
		_ = cached.conn.Close()
	}
}

func parseRuntimeType(value string) runtimev1.RuntimeType {
	switch strings.ToLower(value) {
	case "normal":
		return runtimev1.RuntimeType_NORMAL
	case "sandbox":
		return runtimev1.RuntimeType_SANDBOX
	case "longterm":
		return runtimev1.RuntimeType_LONGTERM
	default:
		return runtimev1.RuntimeType_RUNTIME_TYPE_UNSPECIFIED
	}
}

var _ ports.RuntimeTransport = (*Plane)(nil)
var _ ports.RuntimeDispatcher = (*Plane)(nil)
var _ ports.RuntimePlacement = (*Plane)(nil)
var _ control.Scheduler = (*Plane)(nil)
