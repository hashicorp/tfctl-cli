// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Command otlp-proxy is a public OTLP/HTTP trace proxy. It accepts protobuf
// trace exports from the tfctl CLI over the internet and forwards them,
// unchanged, to a network-segmented internal OpenTelemetry collector.
//
// TLS is terminated at an edge layer in front of this service; the proxy
// itself serves plain HTTP and must not terminate TLS.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/clientip"
	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/config"
	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/forward"
	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/ratelimit"
	"github.com/hashicorp/tfctl-cli/internal/otlpproxy/server"
)

// HTTP server hardening timeouts. ReadHeaderTimeout in particular mitigates
// Slowloris-style attacks on a public endpoint.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 15 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 1 << 16 // 64 KiB.

	// shutdownTimeout bounds in-flight request draining on SIGTERM.
	shutdownTimeout = 10 * time.Second
)

func main() {
	os.Exit(realMain())
}

func realMain() int {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		return 1
	}

	logger := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("server terminated with error", "error", err)
		return 1
	}
	return 0
}

// run wires dependencies, binds the listen address, and serves until the
// context is canceled, then drains gracefully.
func run(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	handler := buildHandler(cfg, logger)
	srv := newHTTPServer(cfg, handler, logger)

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}

	logger.Info("otlp proxy starting",
		"listen_addr", cfg.ListenAddr,
		"no_op", cfg.NoOp(),
		"validate_payload", cfg.ValidatePayload,
		"require_protobuf_content_type", cfg.RequireProtobufContentType,
		"max_body_bytes", cfg.MaxBodyBytes,
		"rate_limit_per_minute", cfg.RateLimitPerMinute,
		"rate_limit_burst", cfg.RateLimitBurst,
		"trusted_hop_count", cfg.TrustedHopCount,
	)

	return serve(ctx, srv, ln, logger)
}

// serve runs srv on ln and shuts it down gracefully when ctx is canceled.
func serve(ctx context.Context, srv *http.Server, ln net.Listener, logger *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received; draining in-flight requests")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
		logger.Info("shutdown complete")
		return nil
	}
}

// buildHandler assembles the request pipeline from configuration. When no
// upstream is configured the proxy runs in no-op mode.
func buildHandler(cfg *config.Config, logger *slog.Logger) http.Handler {
	limiter := ratelimit.NewMemory(ratelimit.MemoryConfig{
		PerMinute:  cfg.RateLimitPerMinute,
		Burst:      cfg.RateLimitBurst,
		MaxEntries: ratelimit.DefaultMaxEntries,
		TTL:        ratelimit.DefaultTTL,
	})

	var fwd forward.Forwarder
	if cfg.NoOp() {
		logger.Info("no upstream endpoint configured; running in no-op mode (exports dropped)")
		fwd = forward.NewNoOp(logger)
	} else {
		logger.Info("forwarding exports upstream", "upstream_endpoint", cfg.UpstreamEndpoint, "upstream_timeout", cfg.UpstreamTimeout)
		fwd = forward.NewHTTP(cfg.UpstreamEndpoint, cfg.UpstreamTimeout, logger)
	}

	return server.New(server.Options{
		Limiter:                    limiter,
		Forwarder:                  fwd,
		IPResolver:                 clientip.Resolver{TrustedHopCount: cfg.TrustedHopCount},
		Logger:                     logger,
		MaxBodyBytes:               cfg.MaxBodyBytes,
		RequireProtobufContentType: cfg.RequireProtobufContentType,
		ValidatePayload:            cfg.ValidatePayload,
		ExpectedServiceName:        cfg.ExpectedServiceName,
	})
}

// newHTTPServer builds an http.Server with hardening timeouts suitable for a
// public, internet-facing endpoint.
func newHTTPServer(cfg *config.Config, handler http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

// newLogger builds a JSON structured logger writing to stdout at the given
// level. LOG_LEVEL=debug enables verbose output.
func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
