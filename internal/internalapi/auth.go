// Package internalapi hosts role-to-role gRPC services.
//
// The listener is separate from the daemon-facing RuntimeService listener and
// requires a dedicated internal token. Only control/api, runtime-gateway, and
// app-gateway processes may call these services.
//
// Copyright 2026 The Levier Authors.
package internalapi

import (
	"context"
	"crypto/subtle"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MetadataToken is the metadata key used for internal role authentication.
const MetadataToken = "x-levier-role-token"

var errMissingInternalToken = errors.New("missing internal service token")

// UnaryAuth rejects unary calls without the expected internal token.
func UnaryAuth(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if err := authorize(ctx, token); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// StreamAuth rejects streaming calls without the expected internal token.
func StreamAuth(token string) grpc.StreamServerInterceptor {
	return func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if err := authorize(stream.Context(), token); err != nil {
			return err
		}
		return handler(srv, stream)
	}
}

func authorize(ctx context.Context, token string) error {
	if token == "" {
		return status.Error(codes.Internal, "internal token is not configured")
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, errMissingInternalToken.Error())
	}
	values := md.Get(MetadataToken)
	if len(values) != 1 {
		return status.Error(codes.Unauthenticated, errMissingInternalToken.Error())
	}
	if subtle.ConstantTimeCompare([]byte(values[0]), []byte(token)) != 1 {
		return status.Error(codes.Unauthenticated, "invalid internal service token")
	}
	return nil
}
