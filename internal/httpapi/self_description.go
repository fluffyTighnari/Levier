// Levier — public service self-description handler.
//
// Copyright 2026 The Levier Authors.
package httpapi

import "net/http"

func (s *Server) handleMeta(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, s.cfg.Meta)
}
