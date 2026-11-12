// Middleware — internal/httpapi
//
// HTTP metrics, request logging, and trace correlation.
//
// Copyright 2026 The Middleware Authors.
package httpapi

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fluffyTighnari/middleware/internal/id"
	"github.com/fluffyTighnari/middleware/internal/observe"
)

func (s *Server) instrumentHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		started := time.Now()
		writer := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		duration := time.Since(started)
		status := writer.Status()
		if status == http.StatusOK && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			status = http.StatusSwitchingProtocols
		}
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		if s.cfg.Metrics != nil {
			s.cfg.Metrics.Inc(observe.MetricHTTPRequestsTotal, "HTTP requests by route and status.", map[string]string{
				"method": r.Method, "route": route, "status": httpStatus(status),
			})
			s.cfg.Metrics.Observe(observe.MetricHTTPRequestDuration, "HTTP request duration in seconds.", map[string]string{
				"method": r.Method, "route": route,
			}, duration.Seconds())
		}
		level := slog.LevelInfo
		switch {
		case status >= http.StatusInternalServerError:
			level = slog.LevelWarn
		case r.URL.Path == "/healthz" || r.URL.Path == "/readyz":
			level = slog.LevelDebug
		}
		s.log.Log(r.Context(), level, "http request",
			observe.FieldRequestID, requestIDFromContext(r.Context()),
			observe.FieldMethod, r.Method,
			observe.FieldRoute, route,
			observe.FieldStatus, status,
			observe.FieldDurationMS, duration.Milliseconds(),
			observe.FieldRemoteAddr, r.RemoteAddr,
		)
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if s.cfg.MetricsCollector != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		s.cfg.MetricsCollector(ctx)
		cancel()
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_ = s.cfg.Metrics.WritePrometheus(w)
}

func traceIDFromRequest(r *http.Request) string {
	if requestID := requestIDFromContext(r.Context()); requestID != "" {
		return requestID
	}
	return id.New()
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) Status() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(payload []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(payload)
}

func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return hijacker.Hijack()
}

func (w *statusWriter) ReadFrom(reader io.Reader) (int64, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return readerFrom.ReadFrom(reader)
	}
	return io.Copy(w.ResponseWriter, reader)
}

func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func httpStatus(code int) string {
	return strconv.Itoa(code)
}
