// Levier - remote App Gateway adapters.
//
// Copyright 2026 The Levier Authors.
package controlremote

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"

	rolev1 "github.com/fluffyTighnari/levier/gen/middleware/role/v1"
	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/cluster"
	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/internalapi"
)

// AppPlane routes local tools and run events to App Gateway replicas.
type AppPlane struct {
	store       *cluster.Store
	token       string
	dialTimeout time.Duration
	logger      *slog.Logger

	mu    sync.Mutex
	conns map[string]*appClient
}

type appClient struct {
	address string
	conn    *grpc.ClientConn
	client  rolev1.AppGatewayServiceClient
}

// NewAppPlane returns a remote App Gateway plane.
func NewAppPlane(store *cluster.Store, token string, dialTimeout time.Duration, logger *slog.Logger) *AppPlane {
	if dialTimeout <= 0 {
		dialTimeout = 3 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AppPlane{
		store:       store,
		token:       token,
		dialTimeout: dialTimeout,
		logger:      logger,
		conns:       make(map[string]*appClient),
	}
}

// CallLocalTool routes one invocation to a live App Gateway.
func (a *AppPlane) CallLocalTool(ctx context.Context, appID string, request connection.LocalToolRequest) (connection.LocalToolResult, error) {
	connectionID := ""
	gatewayID := ""
	if lease, err := a.store.GetAppExecutionLease(ctx, appID); err == nil {
		connectionID = lease.ConnectionID
		gatewayID = lease.GatewayID
	}
	gateway, client, err := a.firstGateway(ctx, gatewayID)
	if err != nil {
		return connection.LocalToolResult{}, err
	}
	response, err := client.CallLocalTool(ctx, &rolev1.CallLocalToolRequest{
		AppId:        appID,
		ConnectionId: connectionID,
		RunId:        request.RunID,
		Request: &runtimev1.ToolCallRequest{
			InvocationId:  request.InvocationID,
			ToolName:      request.ToolName,
			ArgumentsJson: request.ArgumentsJSON,
			TimeoutMs:     request.TimeoutMS,
		},
	})
	if err != nil {
		a.dropGateway(gateway.GatewayID)
		return connection.LocalToolResult{}, err
	}
	result := connection.LocalToolResult{InvocationID: request.InvocationID}
	if response.GetResult() != nil {
		result.OK = response.GetResult().GetOk()
		result.ResultJSON = response.GetResult().GetResultJson()
		if e := response.GetResult().GetError(); e != nil {
			result.Error = &connection.LocalToolError{
				Code: e.GetCode(), Message: e.GetMessage(), Retryable: e.GetRetryable(),
			}
		}
	}
	return result, nil
}

// Publish fans out one run event to every App Gateway; each gateway only
// delivers to its subscribed local sockets.
func (a *AppPlane) Publish(appID, runID string, payload []byte) {
	gateways, err := a.gateways(context.Background())
	if err != nil {
		a.logger.Warn("list app gateways failed", "err", err)
		return
	}
	for _, gateway := range gateways {
		client, err := a.client(context.Background(), gateway)
		if err != nil {
			continue
		}
		if _, err := client.PublishRunEvent(context.Background(), &rolev1.PublishRunEventRequest{
			AppId: appID, RunId: runID, Payload: payload,
		}); err != nil {
			a.dropGateway(gateway.GatewayID)
		}
	}
}

// CancelLocalTools fails pending calls on every App Gateway.
func (a *AppPlane) CancelLocalTools(appID, runID, code, message string) {
	gateways, err := a.gateways(context.Background())
	if err != nil {
		return
	}
	for _, gateway := range gateways {
		client, err := a.client(context.Background(), gateway)
		if err != nil {
			continue
		}
		if _, err := client.CancelLocalTools(context.Background(), &rolev1.CancelLocalToolsRequest{
			AppId: appID, RunId: runID, Code: code, Message: message,
		}); err != nil {
			a.dropGateway(gateway.GatewayID)
		}
	}
}

// Close releases cached internal gRPC connections.
func (a *AppPlane) Close() {
	a.mu.Lock()
	items := make([]*appClient, 0, len(a.conns))
	for _, item := range a.conns {
		items = append(items, item)
	}
	a.conns = make(map[string]*appClient)
	a.mu.Unlock()
	for _, item := range items {
		_ = item.conn.Close()
	}
}

func (a *AppPlane) firstGateway(
	ctx context.Context,
	preferredGatewayID string,
) (cluster.GatewayInstance, rolev1.AppGatewayServiceClient, error) {
	gateways, err := a.gateways(ctx)
	if err != nil {
		return cluster.GatewayInstance{}, nil, err
	}
	if len(gateways) == 0 {
		return cluster.GatewayInstance{}, nil, connection.ErrAppUnreachable
	}
	gateway := gateways[0]
	if preferredGatewayID != "" {
		for _, candidate := range gateways {
			if candidate.GatewayID == preferredGatewayID {
				gateway = candidate
				break
			}
		}
	}
	client, err := a.client(ctx, gateway)
	return gateway, client, err
}

func (a *AppPlane) gateways(ctx context.Context) ([]cluster.GatewayInstance, error) {
	gateways, err := a.store.ListGatewaysByRole(ctx, cluster.RoleAppGateway)
	if err != nil {
		return nil, fmt.Errorf("list app gateways: %w", err)
	}
	result := make([]cluster.GatewayInstance, 0, len(gateways))
	for _, gateway := range gateways {
		if gateway.Status == cluster.StatusOnline &&
			gateway.LeaseExpiresAt.After(time.Now().UTC()) &&
			gateway.InternalAddress != "" {
			result = append(result, gateway)
		}
	}
	return result, nil
}

func (a *AppPlane) client(ctx context.Context, gateway cluster.GatewayInstance) (rolev1.AppGatewayServiceClient, error) {
	a.mu.Lock()
	if cached := a.conns[gateway.GatewayID]; cached != nil && cached.address == gateway.InternalAddress {
		client := cached.client
		a.mu.Unlock()
		return client, nil
	}
	if cached := a.conns[gateway.GatewayID]; cached != nil {
		delete(a.conns, gateway.GatewayID)
		_ = cached.conn.Close()
	}
	a.mu.Unlock()
	conn, err := internalapi.DialWithTimeout(ctx, gateway.InternalAddress, a.token, a.dialTimeout)
	if err != nil {
		return nil, err
	}
	client := rolev1.NewAppGatewayServiceClient(conn)
	a.mu.Lock()
	a.conns[gateway.GatewayID] = &appClient{address: gateway.InternalAddress, conn: conn, client: client}
	a.mu.Unlock()
	return client, nil
}

func (a *AppPlane) dropGateway(gatewayID string) {
	a.mu.Lock()
	cached := a.conns[gatewayID]
	delete(a.conns, gatewayID)
	a.mu.Unlock()
	if cached != nil {
		_ = cached.conn.Close()
	}
}
