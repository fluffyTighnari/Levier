// Levier — httpapi
//
// Split from server.go (P8R.3).
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"github.com/fluffyTighnari/levier/internal/errs"
	"net/http"
)

func (s *Server) handleIssueWSToken(w http.ResponseWriter, r *http.Request) {
	appID, ok := s.resolveRequestApp(w, r, r.URL.Query().Get("app_id"))
	if !ok {
		return
	}
	token := s.wsTokens.Issue(appID)
	if token == "" {
		writeErrorCode(w, errs.CodeInternal, "issue ws token failed", true)
		return
	}
	writeJSON(w, http.StatusOK, WSTokenResponse{
		WSURL:     WSURL,
		WSToken:   token,
		ExpiresIn: int64(s.wsTokens.TTL().Seconds()),
	})
}
