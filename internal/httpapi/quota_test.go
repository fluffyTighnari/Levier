// Levier - HTTP quota hook tests.
// Copyright 2026 The Levier Authors.

package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeQuota struct {
	err    error
	tenant string
	appID  string
	calls  int
}

func (f *fakeQuota) AllowRun(_ context.Context, tenantID, appID string) error {
	f.calls++
	f.tenant = tenantID
	f.appID = appID
	return f.err
}

func TestCreateRunQuotaDenied(t *testing.T) {
	store := newFakeStore()
	queue := &fakeQueue{}
	quota := &fakeQuota{err: errors.New("tenant run quota exceeded")}
	server := NewServer(store, queue, Config{
		BearerToken:       "token-1",
		AllowLegacyBearer: true,
		QuotaPolicy:       quota,
	})
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(httpServer.Close)

	resp := request(t, http.MethodPost, httpServer.URL+"/api/v1/runs", "token-1", []byte(validBody))
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("POST status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
	resp.Body.Close()

	if quota.calls != 1 || quota.tenant != "default" || quota.appID != "app-1" {
		t.Fatalf("quota call = calls:%d tenant:%q app:%q", quota.calls, quota.tenant, quota.appID)
	}
	if len(store.runs) != 0 || len(queue.tasks) != 0 {
		t.Fatalf("denied run was persisted: runs=%d tasks=%d", len(store.runs), len(queue.tasks))
	}
}
