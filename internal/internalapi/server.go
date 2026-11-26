// Levier - internal role gRPC server lifecycle.
//
// Copyright 2026 The Levier Authors.
package internalapi

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"
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
	serve    sync.Once
}

// New binds the internal listener without serving it, so role services can be
// registered before the server accepts RPCs. Call Serve after registration.
func New(opts ServerOptions) (*Server, error) {
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
	return &Server{
		grpc:     server,
		listener: listener,
		logger:   opts.Logger,
		health:   healthServer,
	}, nil
}

// Serve starts the internal listener. It is safe to call once.
func (s *Server) Serve() {
	if s == nil {
		return
	}
	s.serve.Do(func() {
		if s.health != nil {
			s.health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		}
		go func() {
			if err := s.grpc.Serve(s.listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				s.logger.Error("internal grpc server stopped", "addr", s.listener.Addr().String(), "err", err)
			}
		}()
		s.logger.Info("internal grpc listening", "addr", s.listener.Addr().String())
	})
}

// Start binds and serves the internal listener.
func Start(_ context.Context, opts ServerOptions) (*Server, error) {
	server, err := New(opts)
	if err != nil {
		return nil, err
	}
	server.Serve()
	return server, nil
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
