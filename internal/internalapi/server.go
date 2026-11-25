// Levier - internal role gRPC server lifecycle.
//
// Copyright 2026 The Levier Authors.
package internalapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// ServerOptions configures the internal role listener.
type ServerOptions struct {
	Addr            string
	Token           string
	MaxRecvMsgBytes int
	MaxSendMsgBytes int
	Logger          *slog.Logger
}

// Server is one internal role listener.
type Server struct {
	grpc     *grpc.Server
	listener net.Listener
	logger   *slog.Logger
	health   *health.Server
}

// Start binds the internal listener and serves until stopped.
func Start(_ context.Context, opts ServerOptions) (*Server, error) {
	if opts.Token == "" {
		return nil, errors.New("internal token is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	listener, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return nil, err
	}
	options := []grpc.ServerOption{
		grpc.UnaryInterceptor(UnaryAuth(opts.Token)),
		grpc.StreamInterceptor(StreamAuth(opts.Token)),
	}
	if opts.MaxRecvMsgBytes > 0 {
		options = append(options, grpc.MaxRecvMsgSize(opts.MaxRecvMsgBytes))
	}
	if opts.MaxSendMsgBytes > 0 {
		options = append(options, grpc.MaxSendMsgSize(opts.MaxSendMsgBytes))
	}
	server := grpc.NewServer(options...)
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	internal := &Server{
		grpc:     server,
		listener: listener,
		logger:   opts.Logger,
		health:   healthServer,
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			internal.logger.Error("internal grpc server stopped", "addr", listener.Addr().String(), "err", err)
		}
	}()
	internal.logger.Info("internal grpc listening", "addr", listener.Addr().String())
	return internal, nil
}

// GRPC returns the underlying server so role services can be registered.
func (s *Server) GRPC() *grpc.Server {
	if s == nil {
		return nil
	}
	return s.grpc
}

// Addr returns the bound listener address.
func (s *Server) Addr() string {
	if s == nil || s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Stop gracefully stops the internal listener.
func (s *Server) Stop(ctx context.Context) {
	if s == nil {
		return
	}
	if s.health != nil {
		s.health.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	}
	done := make(chan struct{})
	go func() {
		s.grpc.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.grpc.Stop()
	}
	_ = s.listener.Close()
}

// Dial returns a client connection authenticated with the internal token.
func Dial(ctx context.Context, addr, token string) (*grpc.ClientConn, error) {
	return grpc.DialContext(
		ctx,
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithPerRPCCredentials(internalCredentials{token: token}),
		grpc.WithBlock(),
	)
}

type internalCredentials struct {
	token string
}

func (c internalCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{MetadataToken: c.token}, nil
}

func (c internalCredentials) RequireTransportSecurity() bool {
	return false
}

// DialWithTimeout is a convenience wrapper for startup wiring.
func DialWithTimeout(ctx context.Context, addr, token string, timeout time.Duration) (*grpc.ClientConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return Dial(dialCtx, addr, token)
}
