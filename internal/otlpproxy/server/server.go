// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package server implements the OTLP proxy HTTP routing and request pipeline.
//
// It exposes POST /v1/traces (the export endpoint) and GET /health (liveness).
// The traces pipeline runs, in order: per-IP rate limiting, an optional
// content-type check, a hard body-size cap, optional protobuf validation, and
// synchronous forwarding to the upstream collector. Dependencies are injected
// so the handlers are unit-testable in isolation.
package server

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/clientip"
	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/forward"
	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/ratelimit"
	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/validate"
)

// expectedContentType is the only Content-Type accepted when enforcement is on.
const expectedContentType = "application/x-protobuf"

// Decision labels recorded in the per-request log line.
const (
	decisionAccepted    = "accepted"
	decisionLimited     = "limited"
	decisionRejected    = "rejected"
	decisionUnavailable = "upstream_unavailable"
)

// Options carries the dependencies and tunables for the proxy handlers.
type Options struct {
	// Limiter enforces per-client rate limits. Required.
	Limiter ratelimit.RateLimiter

	// Forwarder delivers accepted exports upstream (or drops them). Required.
	Forwarder forward.Forwarder

	// IPResolver extracts the client IP for rate limiting and logging.
	IPResolver clientip.Resolver

	// Logger is the structured logger. Required.
	Logger *slog.Logger

	// MaxBodyBytes is the hard request-body cap enforced via MaxBytesReader.
	MaxBodyBytes int64

	// RequireProtobufContentType rejects non-protobuf content types with 415.
	RequireProtobufContentType bool

	// ValidatePayload enables an optional protobuf decode before forwarding.
	ValidatePayload bool

	// ExpectedServiceName, when validating, is the only accepted service.name.
	ExpectedServiceName string
}

// server holds the injected dependencies shared by the handlers.
type server struct {
	limiter                    ratelimit.RateLimiter
	forwarder                  forward.Forwarder
	ipResolver                 clientip.Resolver
	logger                     *slog.Logger
	maxBodyBytes               int64
	requireProtobufContentType bool
	validatePayload            bool
	expectedServiceName        string
}

// New builds the proxy HTTP handler. Unknown paths yield 404 and unsupported
// methods on a known path yield 405, both via the standard library mux.
func New(opts Options) http.Handler {
	s := &server{
		limiter:                    opts.Limiter,
		forwarder:                  opts.Forwarder,
		ipResolver:                 opts.IPResolver,
		logger:                     opts.Logger,
		maxBodyBytes:               opts.MaxBodyBytes,
		requireProtobufContentType: opts.RequireProtobufContentType,
		validatePayload:            opts.ValidatePayload,
		expectedServiceName:        opts.ExpectedServiceName,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /v1/traces", s.handleTraces)
	return mux
}

// handleHealth answers liveness checks without touching the upstream.
func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok")
	s.logger.Debug("health check", "path", r.URL.Path)
}

// handleTraces runs the export pipeline described in the package doc comment.
func (s *server) handleTraces(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	clientIP := s.ipResolver.ClientIP(r)

	status := http.StatusOK
	decision := decisionAccepted
	defer func() {
		s.logRequest(r, clientIP, decision, status, time.Since(start))
	}()

	// 1. Rate limit by client IP. Fail open on limiter error.
	allowed, err := s.limiter.Allow(clientIP)
	if err != nil {
		s.logger.Debug("rate limiter error; failing open", "client_ip", clientIP, "error", err)
		allowed = true
	}
	if !allowed {
		status, decision = http.StatusTooManyRequests, decisionLimited
		w.WriteHeader(status)
		return
	}

	// 2. Optional content-type enforcement.
	if s.requireProtobufContentType && !isProtobufContentType(r.Header.Get("Content-Type")) {
		status, decision = http.StatusUnsupportedMediaType, decisionRejected
		w.WriteHeader(status)
		return
	}

	// 3. Hard size cap. The Content-Length header is only a cheap pre-check;
	// MaxBytesReader is the real guard against a lying or absent length.
	if s.maxBodyBytes > 0 && r.ContentLength > s.maxBodyBytes {
		status, decision = http.StatusRequestEntityTooLarge, decisionRejected
		w.WriteHeader(status)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			status, decision = http.StatusRequestEntityTooLarge, decisionRejected
		} else {
			status, decision = http.StatusBadRequest, decisionRejected
		}
		w.WriteHeader(status)
		return
	}

	contentType := r.Header.Get("Content-Type")
	contentEncoding := r.Header.Get("Content-Encoding")

	// 4. Optional light validation of a decoded copy. Original bytes are still
	// forwarded unchanged on success.
	if s.validatePayload {
		if verr := validate.Payload(body, contentEncoding, s.expectedServiceName); verr != nil {
			s.logger.Debug("payload validation failed", "client_ip", clientIP, "error", verr)
			status, decision = http.StatusBadRequest, decisionRejected
			w.WriteHeader(status)
			return
		}
	}

	// 5. Forward the original body bytes unmodified (or drop in no-op mode).
	if ferr := s.forwarder.Forward(r.Context(), body, contentType, contentEncoding); ferr != nil {
		s.logger.Warn("upstream forwarding failed", "client_ip", clientIP, "error", ferr)
		status, decision = http.StatusServiceUnavailable, decisionUnavailable
		w.WriteHeader(status)
		return
	}

	// 6. Success: empty 200.
	status, decision = http.StatusOK, decisionAccepted
	w.WriteHeader(status)
}

// logRequest emits a single structured line per request. It never logs request
// bodies or full header sets.
func (s *server) logRequest(r *http.Request, clientIP, decision string, status int, dur time.Duration) {
	level := slog.LevelInfo
	if status >= http.StatusInternalServerError {
		level = slog.LevelWarn
	}
	s.logger.LogAttrs(r.Context(), level, "request",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("client_ip", clientIP),
		slog.String("decision", decision),
		slog.Int("status", status),
		slog.Int64("duration_ms", dur.Milliseconds()),
	)
}

// isProtobufContentType reports whether v is the OTLP protobuf media type,
// tolerating parameters such as a charset.
func isProtobufContentType(v string) bool {
	if v == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(v)
	if err != nil {
		return false
	}
	return mediaType == expectedContentType
}
