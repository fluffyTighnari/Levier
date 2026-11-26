// Package runtimegateway exposes the runtime-gateway role over internal gRPC.
//
// The service is intentionally thin: connection ownership, capacity, and
// TaskAck waiting stay in connection.Hub. A control replica can use this
// service instead of a local daemon hub.
//
// Copyright 2026 The Levier Authors.
package runtimegateway

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	rolev1 "github.com/fluffyTighnari/levier/gen/middleware/role/v1"
	"github.com/fluffyTighnari/levier/internal/cluster"
	"github.com/fluffyTighnari/levier/internal/connection"
)

// Service implements rolev1.RuntimeGatewayService.
type Service struct {
	rolev1.UnimplementedRuntimeGatewayServiceServer
	hub        *connection.Hub
	store      *cluster.Store
	gatewayID  string
	ackTimeout time.Duration
}

// New returns a runtime-gateway service for one process-local hub.
func New(hub *connection.Hub, ackTimeout time.Duration) *Service {
	if ackTimeout <= 0 {
		ackTimeout = 5 * time.Second
	}
	return &Service{hub: hub, ackTimeout: ackTimeout}
}

// WithOwnership enables connection-epoch fencing against the runtime registry.
func (s *Service) WithOwnership(store *cluster.Store, gatewayID string) *Service {
	if s != nil {
		s.store = store
		s.gatewayID = gatewayID
	}
	return s
}

func (s *Service) owns(ctx context.Context, daemonID string, epoch int64) error {
	if s == nil || s.store == nil {
		return nil
	}
	runtime, err := s.store.GetRuntime(ctx, daemonID)
	if err != nil {
		return status.Error(codes.FailedPrecondition, "runtime connection is not registered")
	}
	if runtime.GatewayID != s.gatewayID || runtime.Status != cluster.StatusOnline {
		return status.Error(codes.FailedPrecondition, "runtime connection is owned by another gateway")
	}
	if epoch > 0 && runtime.ConnectionEpoch != epoch {
		return status.Error(codes.FailedPrecondition, "runtime connection epoch is stale")
	}
	return nil
}

// Register registers the internal service.
func (s *Service) Register(server grpc.ServiceRegistrar) {
	rolev1.RegisterRuntimeGatewayServiceServer(server, s)
}

// Dispatch sends one TaskAssign and blocks for its TaskAck.
func (s *Service) Dispatch(ctx context.Context, req *rolev1.DispatchRequest) (*rolev1.DispatchResponse, error) {
	if s == nil || s.hub == nil {
		return nil, status.Error(codes.Unavailable, "runtime gateway is not configured")
	}
	if req.GetDaemonId() == "" || req.GetEnvelope() == nil {
		return nil, status.Error(codes.InvalidArgument, "daemon_id and envelope are required")
	}
	if err := s.owns(ctx, req.GetDaemonId(), 0); err != nil {
		return nil, err
	}
	timeout := time.Duration(req.GetAckTimeoutMs()) * time.Millisecond
	if timeout <= 0 {
		timeout = s.ackTimeout
	}
	ack, err := s.hub.Dispatch(ctx, req.GetDaemonId(), req.GetEnvelope(), timeout)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	return &rolev1.DispatchResponse{
		Accepted: ack.GetAccepted(),
		Reason:   ack.GetReason(),
		Ack:      ack,
	}, nil
}

// Send forwards a non-dispatch envelope.
func (s *Service) Send(ctx context.Context, req *rolev1.SendRequest) (*emptypb.Empty, error) {
	if s == nil || s.hub == nil {
		return nil, status.Error(codes.Unavailable, "runtime gateway is not configured")
	}
	if req.GetDaemonId() == "" || req.GetEnvelope() == nil {
		return nil, status.Error(codes.InvalidArgument, "daemon_id and envelope are required")
	}
	if err := s.owns(ctx, req.GetDaemonId(), req.GetConnectionEpoch()); err != nil {
		return nil, err
	}
	if err := s.hub.Send(req.GetDaemonId(), req.GetEnvelope()); err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	return &emptypb.Empty{}, nil
}

// Reserve reserves one capacity slot owned by this gateway.
func (s *Service) Reserve(ctx context.Context, req *rolev1.ReserveRequest) (*rolev1.ReserveResponse, error) {
	if s == nil || s.hub == nil {
		return nil, status.Error(codes.Unavailable, "runtime gateway is not configured")
	}
	if req.GetDaemonId() == "" {
		return nil, status.Error(codes.InvalidArgument, "daemon_id is required")
	}
	if err := s.owns(ctx, req.GetDaemonId(), 0); err != nil {
		return nil, err
	}
	if !s.hub.Reserve(req.GetDaemonId()) {
		return &rolev1.ReserveResponse{Accepted: false, Reason: "capacity unavailable"}, nil
	}
	return &rolev1.ReserveResponse{Accepted: true}, nil
}

// Release releases one capacity slot.
func (s *Service) Release(ctx context.Context, req *rolev1.ReleaseRequest) (*emptypb.Empty, error) {
	if s == nil || s.hub == nil {
		return nil, status.Error(codes.Unavailable, "runtime gateway is not configured")
	}
	if err := s.owns(ctx, req.GetDaemonId(), 0); err != nil {
		return nil, err
	}
	s.hub.Release(req.GetDaemonId())
	return &emptypb.Empty{}, nil
}

// Disconnect closes the daemon stream owned by this gateway.
func (s *Service) Disconnect(ctx context.Context, req *rolev1.DisconnectRequest) (*emptypb.Empty, error) {
	if s == nil || s.hub == nil {
		return nil, status.Error(codes.Unavailable, "runtime gateway is not configured")
	}
	if err := s.owns(ctx, req.GetDaemonId(), req.GetConnectionEpoch()); err != nil {
		return nil, err
	}
	s.hub.Disconnect(req.GetDaemonId())
	return &emptypb.Empty{}, nil
}

// ListRuntimes lists live connections owned by this gateway.
func (s *Service) ListRuntimes(context.Context, *rolev1.ListRuntimesRequest) (*rolev1.ListRuntimesResponse, error) {
	if s == nil || s.hub == nil {
		return nil, status.Error(codes.Unavailable, "runtime gateway is not configured")
	}
	items := s.hub.List()
	result := make([]*rolev1.RuntimeSnapshot, 0, len(items))
	for _, item := range items {
		result = append(result, &rolev1.RuntimeSnapshot{
			DaemonId:          item.DaemonID,
			RuntimeName:       item.RuntimeName,
			RuntimeType:       item.RuntimeType.String(),
			MaxConcurrentRuns: int32(item.MaxConcurrentRuns),
			RunningCount:      int32(item.RunningCount),
			Reserved:          int32(item.Reserved),
			Labels:            item.Labels,
			Capabilities:      item.Capabilities,
		})
	}
	return &rolev1.ListRuntimesResponse{Runtimes: result}, nil
}

var _ rolev1.RuntimeGatewayServiceServer = (*Service)(nil)
