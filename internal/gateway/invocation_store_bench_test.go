// Middleware — internal/gateway
//
// PostgreSQL invocation store latency benchmark.
//
// Copyright 2026 The Middleware Authors.
package gateway

import (
	"context"
	"fmt"
	"testing"

	runtimev1 "github.com/fluffyTighnari/levier/gen/middleware/runtime/v1"
)

// BenchmarkInvocationStoreRoundTrip measures the two writes the gateway does
// per local tool call: Begin (claim) and Complete (result).
func BenchmarkInvocationStoreRoundTrip(b *testing.B) {
	store := NewPostgresInvocationStore(testGatewayPool(b))
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		invocationID := fmt.Sprintf("inv-bench-%d", i)
		owner, _, err := store.Begin(ctx, invocationID, "run-1", "counter_add")
		if err != nil || !owner {
			b.Fatalf("Begin() = %v, %v", owner, err)
		}
		if err := store.Complete(ctx, invocationID, &runtimev1.ToolCallResult{
			InvocationId: invocationID,
			Ok:           true,
			ResultJson:   `{"value":1}`,
		}); err != nil {
			b.Fatalf("Complete() error = %v", err)
		}
	}
}
