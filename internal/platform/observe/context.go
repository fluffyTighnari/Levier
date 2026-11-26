// Levier - request-scoped observability context.
// Copyright 2026 The Levier Authors.

package observe

import "context"

type tenantContextKey struct{}

// WithTenant attaches the current tenant to a request or invocation context.
func WithTenant(ctx context.Context, tenantID string) context.Context {
	if tenantID == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantContextKey{}, tenantID)
}

// TenantFromContext returns the tenant attached to a context, if any.
func TenantFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(tenantContextKey{}).(string)
	return value
}
