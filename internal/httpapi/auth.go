// Middleware — httpapi
//
// Split from server.go (P8R.3).
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"crypto/subtle"
	"errors"
	"github.com/fluffyTighnari/middleware/internal/errs"
	"net/http"
	"strings"

	"github.com/fluffyTighnari/middleware/internal/runstore"
)

func (s *Server) authDisabled() bool {
	return s.cfg.BearerToken == "" && s.cfg.AdminToken == "" && s.apps == nil
}

func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if strings.HasPrefix(value, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
	}
	return ""
}

func matchesToken(provided, expected string) bool {
	if expected == "" || provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func (s *Server) isAdmin(r *http.Request) bool {
	return matchesToken(bearerToken(r), s.cfg.AdminToken)
}

func (s *Server) legacyAllowed(r *http.Request) bool {
	return s.cfg.AllowLegacyBearer && matchesToken(bearerToken(r), s.cfg.BearerToken)
}

func (s *Server) appFromRequest(r *http.Request) (runstore.App, error) {
	if s.apps == nil {
		return runstore.App{}, runstore.ErrAppNotFound
	}
	token := bearerToken(r)
	if token == "" {
		return runstore.App{}, runstore.ErrAppNotFound
	}
	return s.apps.AuthenticateAPIKey(r.Context(), token)
}

// writeAppAuthError separates bad credentials from a broken auth backend.
func writeAppAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, runstore.ErrAppNotFound) {
		writeError(w, http.StatusUnauthorized, string(errs.CodeUnauthorized), "missing or invalid bearer token", false)
		return
	}
	writeError(w, http.StatusServiceUnavailable, string(errs.CodeDependencyUnavailable), "app authentication backend unavailable", true)
}

func (s *Server) resolveRequestApp(w http.ResponseWriter, r *http.Request, appID string) (string, bool) {
	if s.authDisabled() {
		if appID == "" {
			writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), "app_id is required", false)
			return "", false
		}
		return appID, true
	}
	if s.isAdmin(r) {
		if appID == "" {
			writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), "app_id is required", false)
			return "", false
		}
		return appID, true
	}
	if app, err := s.appFromRequest(r); err == nil {
		if appID != "" && appID != app.AppID {
			writeError(w, http.StatusForbidden, string(errs.CodeForbidden), "app_id does not match the authenticated app", false)
			return "", false
		}
		return app.AppID, true
	} else if !errors.Is(err, runstore.ErrAppNotFound) {
		writeAppAuthError(w, err)
		return "", false
	}
	if s.legacyAllowed(r) {
		if appID == "" {
			writeError(w, http.StatusBadRequest, string(errs.CodeBadRequest), "app_id is required", false)
			return "", false
		}
		return appID, true
	}
	writeAppAuthError(w, runstore.ErrAppNotFound)
	return "", false
}

func (s *Server) canAccessApp(r *http.Request, appID string) bool {
	if s.authDisabled() || s.isAdmin(r) || s.legacyAllowed(r) {
		return true
	}
	app, err := s.appFromRequest(r)
	return err == nil && app.AppID == appID
}
