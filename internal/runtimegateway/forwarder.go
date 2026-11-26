// Levier - Runtime Gateway to Control event forwarding.
//
// Copyright 2026 The Levier Authors.
package runtimegateway

import (
	"context"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"

	rolev1 "github.com/fluffyTighnari/levier/gen/middleware/role/v1"
	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/cluster"
	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/controlservice"
	"github.com/fluffyTighnari/levier/internal/internalapi"
)

// Forwarder sends daemon envelopes to a Control replica.
type Forwarder struct {
	store       *cluster.Store
	gatewayID   string
	token       string
	dialTimeout time.Duration
	logger      *slog.Logger

	mu    sync.Mutex
	conns map[string]*controlClient
}

type controlClient struct {
	address string
	conn    *grpc.ClientConn
	client  rolev1.ControlServiceClient
}

// NewForwarder returns an envelope forwarder.
func NewForwarder(store *cluster.Store, gatewayID, token string, dialTimeout time.Duration, logger *slog.Logger) *Forwarder {
	if dialTimeout <= 0 {
		dialTimeout = 3 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Forwarder{
		store:       store,
		gatewayID:   gatewayID,
		token:       token,
		dialTimeout: dialTimeout,
		logger:      logger,
		conns:       make(map[string]*controlClient),
	}
}

// ForwardEnvelope durably enqueues one non-TaskAck envelope for Control.
// Control claims the outbox row and applies it; if no Control is online the
// envelope remains pending instead of being lost.
func (f *Forwarder) ForwardEnvelope(daemonID string, env *runtimev1.Envelope) error {
	if env == nil || env.GetTaskAck() != nil {
		return nil
	}
	payload, err := controlservice.EncodeEnvelope(f.gatewayID, daemonID, env)
	if err != nil {
		return fmt.Errorf("encode control envelope: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err = f.store.EnqueueNotificationKey(
		ctx,
		controlservice.EnvelopeOutboxChannel,
		"control-envelope:"+env.GetMsgId(),
		payload,
	)
	if err != nil {
		return fmt.Errorf("enqueue control envelope: %w", err)
	}
	return nil
}

// DaemonOnline mirrors a live daemon snapshot into Control.
func (f *Forwarder) DaemonOnline(info connection.DaemonInfo) error {
	return f.sendDaemonState(info.DaemonID, cluster.StatusOnline, info)
}

// DaemonOffline mirrors a closed daemon stream into Control.
func (f *Forwarder) DaemonOffline(daemonID string) error {
	return f.sendDaemonState(daemonID, cluster.StatusOffline, connection.DaemonInfo{DaemonID: daemonID})
}

func (f *Forwarder) sendDaemonState(daemonID, status string, info connection.DaemonInfo) error {
	ctx, cancel := context.WithTimeout(context.Background(), f.dialTimeout+3*time.Second)
	defer cancel()
	gateway, client, err := f.controlClient(ctx, daemonID)
	if err != nil {
		return err
	}
	running := make([]*rolev1.DaemonRun, 0, len(info.RunningRuns))
	for _, run := range info.RunningRuns {
		running = append(running, &rolev1.DaemonRun{RunId: run.RunID, LeaseEpoch: run.LeaseEpoch})
	}
	_, err = client.UpdateDaemonState(ctx, &rolev1.DaemonStateRequest{
		GatewayId:         f.gatewayID,
		DaemonId:          daemonID,
		Status:            status,
		RuntimeName:       info.RuntimeName,
		RuntimeType:       info.RuntimeType.String(),
		MaxConcurrentRuns: info.MaxConcurrentRuns,
		RunningRuns:       running,
		Labels:            info.Labels,
		Capabilities:      info.Capabilities,
		ConnectedAtMs:     info.ConnectedAt.UnixMilli(),
	})
	if err != nil {
		f.dropControl(gateway.GatewayID)
		return fmt.Errorf("update daemon state on control %s: %w", gateway.GatewayID, err)
	}
	return nil
}

// Close releases cached gRPC connections.
func (f *Forwarder) Close() {
	f.mu.Lock()
	items := make([]*controlClient, 0, len(f.conns))
	for _, item := range f.conns {
		items = append(items, item)
	}
	f.conns = make(map[string]*controlClient)
	f.mu.Unlock()
	for _, item := range items {
		_ = item.conn.Close()
	}
}

func (f *Forwarder) controlClient(ctx context.Context, daemonID string) (cluster.GatewayInstance, rolev1.ControlServiceClient, error) {
	gateways, err := f.store.ListGatewaysByRole(ctx, cluster.RoleControl)
	if err != nil {
		return cluster.GatewayInstance{}, nil, fmt.Errorf("list control gateways: %w", err)
	}
	eligible := make([]cluster.GatewayInstance, 0, len(gateways))
	for _, gateway := range gateways {
		if gateway.Status == cluster.StatusOnline &&
			gateway.LeaseExpiresAt.After(time.Now().UTC()) &&
			gateway.InternalAddress != "" {
			eligible = append(eligible, gateway)
		}
	}
	if len(eligible) == 0 {
		return cluster.GatewayInstance{}, nil, fmt.Errorf("no control gateway is available")
	}
	index := int(hashString(daemonID) % uint32(len(eligible)))
	gateway := eligible[index]

	f.mu.Lock()
	if cached := f.conns[gateway.GatewayID]; cached != nil && cached.address == gateway.InternalAddress {
		client := cached.client
		f.mu.Unlock()
		return gateway, client, nil
	}
	if cached := f.conns[gateway.GatewayID]; cached != nil {
		delete(f.conns, gateway.GatewayID)
		_ = cached.conn.Close()
	}
	f.mu.Unlock()

	conn, err := internalapi.DialWithTimeout(ctx, gateway.InternalAddress, f.token, f.dialTimeout)
	if err != nil {
		return cluster.GatewayInstance{}, nil, fmt.Errorf("dial control gateway %s: %w", gateway.GatewayID, err)
	}
	client := rolev1.NewControlServiceClient(conn)
	f.mu.Lock()
	f.conns[gateway.GatewayID] = &controlClient{address: gateway.InternalAddress, conn: conn, client: client}
	f.mu.Unlock()
	return gateway, client, nil
}

func (f *Forwarder) dropControl(gatewayID string) {
	f.mu.Lock()
	cached := f.conns[gatewayID]
	delete(f.conns, gatewayID)
	f.mu.Unlock()
	if cached != nil {
		_ = cached.conn.Close()
	}
}

func hashString(value string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(value))
	return hash.Sum32()
}
