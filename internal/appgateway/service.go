// Package appgateway exposes the App connection role over internal gRPC.
//
// The App Gateway owns WebSocket connections and the local-tool pending map.
// Control routes tool calls and run events here instead of touching AppHub.
//
// Copyright 2026 The Levier Authors.
package appgateway

import (
	"context"
	"encoding/json"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	rolev1 "github.com/fluffyTighnari/levier/gen/middleware/role/v1"
	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
	"github.com/fluffyTighnari/levier/internal/connection"
)

// Service implements rolev1.AppGatewayService.
type Service struct {
	rolev1.UnimplementedAppGatewayServiceServer
	hub *connection.AppHub
}

// New returns an App Gateway service.
func New(hub *connection.AppHub) *Service {
	return &Service{hub: hub}
}

// Register registers the internal service.
func (s *Service) Register(server grpc.ServiceRegistrar) {
	rolev1.RegisterAppGatewayServiceServer(server, s)
}

// CallLocalTool delivers one invocation to the App Gateway's execution connection.
func (s *Service) CallLocalTool(ctx context.Context, req *rolev1.CallLocalToolRequest) (*rolev1.CallLocalToolResponse, error) {
	if s == nil || s.hub == nil {
		return nil, status.Error(codes.Unavailable, "app gateway is not configured")
	}
	if req.GetAppId() == "" || req.GetRequest() == nil {
		return nil, status.Error(codes.InvalidArgument, "app_id and request are required")
	}
	request := connection.LocalToolRequest{
		InvocationID:  req.GetRequest().GetInvocationId(),
		RunID:         req.GetRunId(),
		ToolName:      req.GetRequest().GetToolName(),
		ArgumentsJSON: req.GetRequest().GetArgumentsJson(),
		TimeoutMS:     req.GetRequest().GetTimeoutMs(),
	}
	if request.TimeoutMS <= 0 {
		request.TimeoutMS = req.GetTimeoutMs()
	}
	if request.TimeoutMS <= 0 {
		request.TimeoutMS = time.Minute.Milliseconds()
	}
	// The execution lease already selected the connection; AppHub currently
	// has one pending namespace per App, so the service passes through.
	result, err := s.hub.CallLocalToolTo(ctx, req.GetAppId(), req.GetConnectionId(), request)
	if err != nil {
		return nil, status.Error(codes.Unavailable, err.Error())
	}
	response := &runtimev1.ToolCallResult{
		InvocationId: result.InvocationID,
		Ok:           result.OK,
		ResultJson:   result.ResultJSON,
	}
	if result.Error != nil {
		response.Error = &runtimev1.ErrorInfo{
			Code:      result.Error.Code,
			Message:   result.Error.Message,
			Retryable: result.Error.Retryable,
		}
	}
	return &rolev1.CallLocalToolResponse{Result: response}, nil
}

// CancelLocalTools fails pending local tool calls for one App/Run.
func (s *Service) CancelLocalTools(_ context.Context, req *rolev1.CancelLocalToolsRequest) (*emptypb.Empty, error) {
	if s == nil || s.hub == nil {
		return nil, status.Error(codes.Unavailable, "app gateway is not configured")
	}
	s.hub.CancelLocalTools(req.GetAppId(), req.GetRunId(), req.GetCode(), req.GetMessage())
	return &emptypb.Empty{}, nil
}

// PublishRunEvent forwards one already-persisted Run event to subscribers.
func (s *Service) PublishRunEvent(_ context.Context, req *rolev1.PublishRunEventRequest) (*emptypb.Empty, error) {
	if s == nil || s.hub == nil {
		return nil, status.Error(codes.Unavailable, "app gateway is not configured")
	}
	if req.GetAppId() == "" || req.GetRunId() == "" || len(req.GetPayload()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "app_id, run_id, and payload are required")
	}
	s.hub.Publish(req.GetAppId(), req.GetRunId(), req.GetPayload())
	return &emptypb.Empty{}, nil
}

// PublishRunCompleted announces a terminal Run.
func (s *Service) PublishRunCompleted(_ context.Context, req *rolev1.PublishRunCompletedRequest) (*emptypb.Empty, error) {
	if s == nil || s.hub == nil {
		return nil, status.Error(codes.Unavailable, "app gateway is not configured")
	}
	payload, _ := json.Marshal(map[string]any{
		"type": "run_completed", "run_id": req.GetRunId(),
		"status": req.GetStatus(), "attempt": req.GetAttempt(),
	})
	s.hub.Publish(req.GetAppId(), req.GetRunId(), payload)
	return &emptypb.Empty{}, nil
}

var _ rolev1.AppGatewayServiceServer = (*Service)(nil)
