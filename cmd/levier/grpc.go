// Levier - daemon-facing gRPC listener.
//
// Copyright 2026 The Levier Authors.
package main

import (
	"net"

	"google.golang.org/grpc"

	"github.com/fluffyTighnari/levier/internal/config"
	"github.com/fluffyTighnari/levier/internal/connection"
)

func startDaemonGRPC(cfg config.Config, hub *connection.Hub) (net.Listener, *grpc.Server, error) {
	listener, err := net.Listen("tcp", cfg.GRPC.Addr)
	if err != nil {
		return nil, nil, err
	}
	server := grpc.NewServer(
		grpc.MaxRecvMsgSize(cfg.GRPC.MaxRecvMsgBytes),
		grpc.MaxSendMsgSize(cfg.GRPC.MaxSendMsgBytes),
	)
	hub.Register(server)
	go func() {
		_ = server.Serve(listener)
	}()
	return listener, server, nil
}
