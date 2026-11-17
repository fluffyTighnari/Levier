// Levier — httpapi
//
// Split from server.go (P8R.3).
// Copyright 2026 The Levier Authors.
package httpapi

import (
	"errors"
	"github.com/fluffyTighnari/levier/internal/errs"
	"net/http"

	"github.com/fluffyTighnari/levier/internal/runstore"
)

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeErrorCode(w, errs.CodeNotImplemented, "session store is not configured", false)
		return
	}
	session, err := s.sessions.GetSession(r.Context(), r.PathValue("session_id"))
	if errors.Is(err, runstore.ErrSessionNotFound) {
		writeErrorCode(w, errs.CodeSessionNotFound, "session not found", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "get session failed", true)
		return
	}
	if !s.canAccessApp(r, session.AppID) {
		writeErrorCode(w, errs.CodeSessionNotFound, "session not found", false)
		return
	}
	writeJSON(w, http.StatusOK, sessionView(session))
}

func (s *Server) handleListSessionEvents(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeErrorCode(w, errs.CodeNotImplemented, "session store is not configured", false)
		return
	}
	afterAttempt, err := queryInt32(r.URL.Query().Get("after_attempt"))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, "after_attempt must be a non-negative integer", false)
		return
	}
	afterSeq, err := queryInt64(r.URL.Query().Get("after_seq"))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, "after_seq must be a non-negative integer", false)
		return
	}
	limit, err := queryLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeErrorCode(w, errs.CodeBadRequest, "limit must be between 1 and 1000", false)
		return
	}
	sessionID := r.PathValue("session_id")
	session, err := s.sessions.GetSession(r.Context(), sessionID)
	if errors.Is(err, runstore.ErrSessionNotFound) {
		writeErrorCode(w, errs.CodeSessionNotFound, "session not found", false)
		return
	}
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "get session failed", true)
		return
	}
	if !s.canAccessApp(r, session.AppID) {
		writeErrorCode(w, errs.CodeSessionNotFound, "session not found", false)
		return
	}
	events, err := s.sessions.ListSessionEvents(r.Context(), sessionID, afterAttempt, afterSeq, limit)
	if err != nil {
		writeErrorCode(w, errs.CodeInternal, "list session events failed", true)
		return
	}
	writeJSON(w, http.StatusOK, eventListResponse(events, limit))
}

func eventListResponse(events []runstore.Event, limit int) EventListResponse {
	views := make([]RunEventView, 0, len(events))
	for _, event := range events {
		views = append(views, RunEventView{
			RunID:     event.RunID,
			Attempt:   int32(event.Attempt),
			Seq:       event.Seq,
			Source:    event.Source,
			EventType: event.Type,
			Data:      event.Payload,
			TSMillis:  event.TSMs,
		})
	}
	response := EventListResponse{
		Events:   views,
		CaughtUp: len(views) < limit,
	}
	if len(events) > 0 {
		last := events[len(events)-1]
		response.NextAttempt = int32(last.Attempt)
		response.NextSeq = last.Seq
	}
	return response
}

func sessionView(session runstore.Session) SessionView {
	return SessionView{
		SessionID:      session.SessionID,
		AppID:          session.AppID,
		Status:         session.Status,
		CurrentRunID:   session.CurrentRunID,
		EventAttempt:   int32(session.EventAttempt),
		EventWatermark: session.EventWatermark,
		CreatedAt:      session.CreatedAt,
		UpdatedAt:      session.UpdatedAt,
		ExpiresAt:      session.ExpiresAt,
	}
}
