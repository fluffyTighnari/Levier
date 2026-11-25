// Levier - App connection boundary.
//
// Copyright 2026 The Levier Authors.
package ports

import "github.com/fluffyTighnari/levier/internal/connection"

// AppConnections is the App Gateway surface used by the HTTP/WebSocket layer.
type AppConnections interface {
	Register(appID string, sink connection.EventSink) *connection.AppConn
	CloseConn(conn *connection.AppConn)
	ResolveLocalToolChecked(appID string, result connection.LocalToolResult) connection.LocalToolResolution
	Count(appID string) int
	Stats() connection.AppStats
}
