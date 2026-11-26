// Package controlservice exposes Control's internal role RPC.
//
// Runtime Gateways forward daemon envelopes here. The service does not own
// scheduling state; it delegates to the Control instance that hosts it.
//
// Copyright 2026 The Levier Authors.
package controlservice

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	rolev1 "github.com/fluffyTighnari/levier/gen/middleware/role/v1"
	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/cluster"
	"github.com/fluffyTighnari/levier/internal/connection"
	"github.com/fluffyTighnari/levier/internal/runstore"
)

// Controller is the Control subset used by the internal service.
type Controller interface {
	HandleEnvelope(daemonID string, env *runtimev1.Envelope)
	DispatchToolCall(daemonID string, env *runtimev1.Envelope)
	ReconcileDaemon(info connection.DaemonInfo)
	HandleDaemonOffline(daemonID string)
	CancelRun(ctx context.Context, runID, reason string) (runstore.Run, error)
	ResumeRun(ctx context.Context, runID string, resume json.RawMessage) (runstore.Run, error)
}

// Service implements rolev1.ControlService.
type Service struct {
	rolev1.UnimplementedControlServiceServer
	control Controller
	store   *cluster.Store
}

// New returns a Control service.
func New(control Controller) *Service {
	return &Service{control: control}
}

// WithRuntimeStore enables daemon-state fencing against PostgreSQL.
func (s *Service) WithRuntimeStore(store *cluster.Store) *Service {
	if s != nil {
		s.store = store
	}
	return s
}

// Register registers the service on an internal gRPC server.
func (s *Service) Register(server grpc.ServiceRegistrar) {
	rolev1.RegisterControlServiceServer(server, s)
}

// DeliverEnvelope forwards one daemon envelope to Control.
func (s *Service) DeliverEnvelope(ctx context.Context, req *rolev1.DeliverEnvelopeRequest) (*emptypb.Empty, error) {
	if s == nil || s.control == nil {
		return nil, status.Error(codes.Unavailable, "control service is not configured")
	}
	if req.GetEnvelope() == nil {
		return nil, status.Error(codes.InvalidArgument, "envelope is required")
	}
	if req.GetEnvelope().GetToolCallRequest() != nil {
		s.control.DispatchToolCall(req.GetDaemonId(), req.GetEnvelope())
		return &emptypb.Empty{}, nil
	}
	s.control.HandleEnvelope(req.GetDaemonId(), req.GetEnvelope())
	return &emptypb.Empty{}, nil
}

// UpdateDaemonState mirrors a Runtime Gateway's daemon lifecycle snapshot.
func (s *Service) UpdateDaemonState(ctx context.Context, req *rolev1.DaemonStateRequest) (*emptypb.Empty, error) {
	if s == nil || s.control == nil {
		return nil, status.Error(codes.Unavailable, "control service is not configured")
	}
	if req.GetDaemonId() == "" {
		return nil, status.Error(codes.InvalidArgument, "daemon_id is required")
	}
	if s.store != nil {
		runtime, err := s.store.GetRuntime(ctx, req.GetDaemonId())
		offline := req.GetStatus() == cluster.StatusOffline
		if err != nil ||
			runtime.GatewayID != req.GetGatewayId() ||
			runtime.ConnectionEpoch != req.GetConnectionEpoch() ||
			(!offline && runtime.Status != cluster.StatusOnline) {
			return nil, status.Error(codes.FailedPrecondition, "daemon state is stale")
		}
	}
	if req.GetStatus() == cluster.StatusOffline {
		s.control.HandleDaemonOffline(req.GetDaemonId())
		return &emptypb.Empty{}, nil
	}
	running := make([]connection.RunRef, 0, len(req.GetRunningRuns()))
	for _, run := range req.GetRunningRuns() {
		running = append(running, connection.RunRef{RunID: run.GetRunId(), LeaseEpoch: run.GetLeaseEpoch()})
	}
	s.control.ReconcileDaemon(connection.DaemonInfo{
		DaemonID:          req.GetDaemonId(),
		Status:            req.GetStatus(),
		RuntimeName:       req.GetRuntimeName(),
		RuntimeType:       runtimeType(req.GetRuntimeType()),
		MaxConcurrentRuns: req.GetMaxConcurrentRuns(),
		Labels:            req.GetLabels(),
		Capabilities:      req.GetCapabilities(),
		RunningRuns:       running,
		RunningCount:      int32(len(running)),
		ConnectedAt:       time.UnixMilli(req.GetConnectedAtMs()).UTC(),
	})
	return &emptypb.Empty{}, nil
}

// CancelRun cancels one Run on the owning Control replica.
func (s *Service) CancelRun(ctx context.Context, req *rolev1.CancelRunRequest) (*rolev1.CancelRunResponse, error) {
	if s == nil || s.control == nil {
		return nil, status.Error(codes.Unavailable, "control service is not configured")
	}
	run, err := s.control.CancelRun(ctx, req.GetRunId(), req.GetReason())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &rolev1.CancelRunResponse{RunId: run.RunID, Status: run.Status}, nil
}

// ResumeRun resumes one waiting Run on the owning Control replica.
func (s *Service) ResumeRun(ctx context.Context, req *rolev1.ResumeRunRequest) (*rolev1.ResumeRunResponse, error) {
	if s == nil || s.control == nil {
		return nil, status.Error(codes.Unavailable, "control service is not configured")
	}
	run, err := s.control.ResumeRun(ctx, req.GetRunId(), json.RawMessage(req.GetResumeValueJson()))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &rolev1.ResumeRunResponse{
		RunId:     run.RunID,
		SessionId: run.SessionID,
		Status:    run.Status,
	}, nil
}

func runtimeType(value string) runtimev1.RuntimeType {
	switch strings.ToLower(value) {
	case "sandbox":
		return runtimev1.RuntimeType_SANDBOX
	case "longterm":
		return runtimev1.RuntimeType_LONGTERM
	default:
		return runtimev1.RuntimeType_NORMAL
	}
}

var _ rolev1.ControlServiceServer = (*Service)(nil)
