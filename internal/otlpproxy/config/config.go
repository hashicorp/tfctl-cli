// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package config loads and validates the OTLP proxy configuration from
// environment variables. Configuration is parsed once at startup and the
// process should fail fast on any invalid value.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Environment variable names recognized by the proxy.
const (
	EnvListenAddr                 = "LISTEN_ADDR"
	EnvUpstreamEndpoint           = "UPSTREAM_ENDPOINT"
	EnvUpstreamTimeout            = "UPSTREAM_TIMEOUT"
	EnvMaxBodyBytes               = "MAX_BODY_BYTES"
	EnvRateLimitPerMinute         = "RATE_LIMIT_PER_MINUTE"
	EnvRateLimitBurst             = "RATE_LIMIT_BURST"
	EnvTrustedHopCount            = "TRUSTED_HOP_COUNT"
	EnvRequireProtobufContentType = "REQUIRE_PROTOBUF_CONTENT_TYPE"
	EnvValidatePayload            = "VALIDATE_PAYLOAD"
	EnvExpectedServiceName        = "EXPECTED_SERVICE_NAME"
	EnvLogLevel                   = "LOG_LEVEL"
)

// Config holds the fully parsed and validated proxy configuration.
type Config struct {
	// ListenAddr is the plain-HTTP bind address (TLS is terminated upstream).
	ListenAddr string

	// UpstreamEndpoint is the OTLP/HTTP collector base URL. Empty enables
	// no-op mode (requests are accepted and dropped, never forwarded).
	UpstreamEndpoint string

	// UpstreamTimeout bounds each forwarded request to the collector.
	UpstreamTimeout time.Duration

	// MaxBodyBytes is the hard cap on request body size.
	MaxBodyBytes int64

	// RateLimitPerMinute is the per-IP sustained request rate.
	RateLimitPerMinute float64

	// RateLimitBurst is the per-IP token-bucket burst size.
	RateLimitBurst int

	// TrustedHopCount is the number of trusted reverse proxies in front of
	// the service, used to resolve the client IP from X-Forwarded-For.
	TrustedHopCount int

	// RequireProtobufContentType enforces an application/x-protobuf
	// Content-Type when true.
	RequireProtobufContentType bool

	// ValidatePayload enables an optional protobuf decode of the body.
	ValidatePayload bool

	// ExpectedServiceName, when validating, is the only service.name accepted.
	ExpectedServiceName string

	// LogLevel is the minimum slog level emitted.
	LogLevel slog.Level
}

// NoOp reports whether the proxy runs without an upstream collector.
func (c *Config) NoOp() bool {
	return c.UpstreamEndpoint == ""
}

// Load parses configuration using the provided environment lookup function
// (typically os.LookupEnv). It returns a clear error on the first invalid
// value so the process can fail fast at startup.
func Load(lookup func(string) (string, bool)) (*Config, error) {
	cfg := &Config{
		ListenAddr:                 ":8080",
		UpstreamEndpoint:           "",
		UpstreamTimeout:            5 * time.Second,
		MaxBodyBytes:               1 << 20, // 1 MiB.
		RateLimitPerMinute:         120,
		RateLimitBurst:             60,
		TrustedHopCount:            1,
		RequireProtobufContentType: true,
		ValidatePayload:            false,
		ExpectedServiceName:        "tfctl",
		LogLevel:                   slog.LevelInfo,
	}

	if v, ok := lookup(EnvListenAddr); ok && v != "" {
		cfg.ListenAddr = v
	}

	if v, ok := lookup(EnvUpstreamEndpoint); ok && v != "" {
		if err := validateUpstreamURL(v); err != nil {
			return nil, err
		}
		cfg.UpstreamEndpoint = v
	}

	if v, ok := lookup(EnvUpstreamTimeout); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid duration %q: %w", EnvUpstreamTimeout, v, err)
		}
		if d <= 0 {
			return nil, fmt.Errorf("%s: must be positive, got %s", EnvUpstreamTimeout, d)
		}
		cfg.UpstreamTimeout = d
	}

	if v, ok := lookup(EnvMaxBodyBytes); ok && v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid integer %q: %w", EnvMaxBodyBytes, v, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("%s: must be positive, got %d", EnvMaxBodyBytes, n)
		}
		cfg.MaxBodyBytes = n
	}

	if v, ok := lookup(EnvRateLimitPerMinute); ok && v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid number %q: %w", EnvRateLimitPerMinute, v, err)
		}
		if f <= 0 {
			return nil, fmt.Errorf("%s: must be positive, got %v", EnvRateLimitPerMinute, f)
		}
		cfg.RateLimitPerMinute = f
	}

	if v, ok := lookup(EnvRateLimitBurst); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid integer %q: %w", EnvRateLimitBurst, v, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("%s: must be positive, got %d", EnvRateLimitBurst, n)
		}
		cfg.RateLimitBurst = n
	}

	if v, ok := lookup(EnvTrustedHopCount); ok && v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid integer %q: %w", EnvTrustedHopCount, v, err)
		}
		if n < 0 {
			return nil, fmt.Errorf("%s: must not be negative, got %d", EnvTrustedHopCount, n)
		}
		cfg.TrustedHopCount = n
	}

	if v, ok := lookup(EnvRequireProtobufContentType); ok && v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid boolean %q: %w", EnvRequireProtobufContentType, v, err)
		}
		cfg.RequireProtobufContentType = b
	}

	if v, ok := lookup(EnvValidatePayload); ok && v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid boolean %q: %w", EnvValidatePayload, v, err)
		}
		cfg.ValidatePayload = b
	}

	if v, ok := lookup(EnvExpectedServiceName); ok {
		cfg.ExpectedServiceName = v
	}

	if v, ok := lookup(EnvLogLevel); ok && v != "" {
		level, err := parseLogLevel(v)
		if err != nil {
			return nil, err
		}
		cfg.LogLevel = level
	}

	return cfg, nil
}

// validateUpstreamURL ensures the endpoint is an absolute http(s) URL with a
// host. The proxy appends the OTLP path, so the value must be a base URL.
func validateUpstreamURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s: invalid URL %q: %w", EnvUpstreamEndpoint, raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s: URL %q must use http or https scheme", EnvUpstreamEndpoint, raw)
	}
	if u.Host == "" {
		return fmt.Errorf("%s: URL %q must include a host", EnvUpstreamEndpoint, raw)
	}
	return nil
}

// parseLogLevel maps a case-insensitive level name to a slog.Level.
func parseLogLevel(v string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("%s: unknown level %q (want debug, info, warn, or error)", EnvLogLevel, v)
	}
}
