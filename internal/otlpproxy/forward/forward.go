// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package forward sends accepted OTLP trace exports to the internal collector.
//
// Two implementations satisfy Forwarder: HTTP forwards synchronously to a
// configured upstream, and NoOp accepts and drops the request (used for local
// development and infrastructure bring-up when no upstream is configured).
// Forwarding never rewrites the payload: the original bytes are sent verbatim.
package forward

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// tracesPath is the OTLP/HTTP traces path appended to the upstream base URL.
const tracesPath = "/v1/traces"

// drainLimit bounds how much of an upstream response body is read (and
// discarded) to allow connection reuse without trusting upstream size.
const drainLimit = 1 << 20

// Forwarder sends a trace export body to the upstream collector.
type Forwarder interface {
	// Forward delivers body to the upstream, preserving the provided
	// contentType and contentEncoding. It returns nil when the export is
	// accepted (forwarded successfully, or dropped in no-op mode) and a
	// non-nil error when the upstream is unavailable or rejects the request.
	Forward(ctx context.Context, body []byte, contentType, contentEncoding string) error
}

// HTTP forwards exports to an upstream OTLP/HTTP collector.
type HTTP struct {
	tracesURL string
	client    *http.Client
	logger    *slog.Logger
}

// NewHTTP builds an HTTP forwarder targeting endpoint (the collector base URL).
// The OTLP traces path is appended automatically.
func NewHTTP(endpoint string, timeout time.Duration, logger *slog.Logger) *HTTP {
	return &HTTP{
		tracesURL: strings.TrimRight(endpoint, "/") + tracesPath,
		client:    &http.Client{Timeout: timeout},
		logger:    logger,
	}
}

// Forward POSTs the body to the upstream collector. A 2xx response yields nil;
// any other status, a transport error, or a timeout yields an error. The
// upstream error is never exposed to the original client.
func (h *HTTP) Forward(ctx context.Context, body []byte, contentType, contentEncoding string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.tracesURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build upstream request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if contentEncoding != "" {
		req.Header.Set("Content-Encoding", contentEncoding)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("forward to upstream: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, drainLimit))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	h.logger.Debug("forwarded trace export", "bytes", len(body), "upstream_status", resp.StatusCode)
	return nil
}

// NoOp accepts exports and drops them without contacting any upstream. It is
// used when no upstream endpoint is configured.
type NoOp struct {
	logger *slog.Logger
}

// NewNoOp builds a no-op forwarder.
func NewNoOp(logger *slog.Logger) *NoOp {
	return &NoOp{logger: logger}
}

// Forward logs at debug and returns nil; the export is intentionally dropped.
func (n *NoOp) Forward(_ context.Context, body []byte, _, _ string) error {
	n.logger.Debug("dropping trace export in no-op mode", "bytes", len(body))
	return nil
}
