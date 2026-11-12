// Middleware — httpapi
//
// Split from server.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"github.com/fluffyTighnari/middleware/internal/errs"
	"net/http"
)

func (s *Server) handleIssueWSToken(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.resolveRequestApp(w, r, r.URL.Query().Get("app_id"))
	if !ok {
		return
	}
	token := s.wsTokens.Issue(appID)
	if token == "" {
		writeError(w, http.StatusInternalServerError, string(errs.CodeInternal), "issue ws token failed", true)
		return
	}
	writeJSON(w, http.StatusOK, WSTokenResponse{
		WSURL:     WSURL,
		WSToken:   token,
		ExpiresIn: int64(s.wsTokens.TTL().Seconds()),
	})
}
