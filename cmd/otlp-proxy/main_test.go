// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func noOpConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(func(string) (string, bool) { return "", false })
	require.NoError(t, err)
	return cfg
}

func TestBuildHandler_NoOpModeAcceptsTraces(t *testing.T) {
	t.Parallel()

	cfg := noOpConfig(t)
	require.True(t, cfg.NoOp())

	h := buildHandler(cfg, testLogger())

	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte{0x0a, 0x00}))
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
}

func TestBuildHandler_HealthEndpoint(t *testing.T) {
	t.Parallel()

	h := buildHandler(noOpConfig(t), testLogger())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Body.String())
}

func TestNewHTTPServer_HasHardeningTimeouts(t *testing.T) {
	t.Parallel()

	cfg := noOpConfig(t)
	srv := newHTTPServer(cfg, http.NotFoundHandler(), testLogger())

	assert.Equal(t, cfg.ListenAddr, srv.Addr)
	assert.Positive(t, srv.ReadHeaderTimeout, "ReadHeaderTimeout mitigates Slowloris")
	assert.Positive(t, srv.ReadTimeout)
	assert.Positive(t, srv.WriteTimeout)
	assert.Positive(t, srv.IdleTimeout)
	assert.Positive(t, srv.MaxHeaderBytes)
}

func TestServe_GracefulShutdown(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &http.Server{Handler: buildHandler(noOpConfig(t), testLogger())}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, srv, ln, testLogger())
	}()

	// The server should be accepting requests.
	healthURL := "http://" + ln.Addr().String() + "/health"
	resp, err := http.Get(healthURL) //nolint:noctx // simple test request.
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_ = resp.Body.Close()

	// Signal shutdown and confirm serve drains and returns without error.
	cancel()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after context cancellation")
	}

	// After shutdown the listener is closed and connections are refused.
	_, err = http.Get(healthURL) //nolint:noctx // simple test request.
	assert.Error(t, err)
}
