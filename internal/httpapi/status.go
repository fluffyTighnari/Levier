// Levier — runtime status overview handler.
//
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"context"
	"net/http"
	"time"
)

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.cfg.Status == nil {
		writeError(w, http.StatusServiceUnavailable, "STATUS_UNAVAILABLE", "status provider is not configured", true)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.cfg.Status.Snapshot(ctx))
}
