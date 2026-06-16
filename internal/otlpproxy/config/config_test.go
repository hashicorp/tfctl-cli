// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package config

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mapLookup builds a lookup function backed by a map for deterministic,
// parallel-safe config tests (no reliance on process environment).
func mapLookup(m map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()

	cfg, err := Load(mapLookup(nil))
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.ListenAddr)
	assert.Equal(t, "", cfg.UpstreamEndpoint)
	assert.Equal(t, 5*time.Second, cfg.UpstreamTimeout)
	assert.Equal(t, int64(1048576), cfg.MaxBodyBytes)
	assert.InEpsilon(t, 120.0, cfg.RateLimitPerMinute, 0.0001)
	assert.Equal(t, 60, cfg.RateLimitBurst)
	assert.Equal(t, 1, cfg.TrustedHopCount)
	assert.True(t, cfg.RequireProtobufContentType)
	assert.False(t, cfg.ValidatePayload)
	assert.Equal(t, "tfctl", cfg.ExpectedServiceName)
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
	assert.True(t, cfg.NoOp(), "empty upstream endpoint should be no-op mode")
}

func TestLoad_Overrides(t *testing.T) {
	t.Parallel()

	cfg, err := Load(mapLookup(map[string]string{
		"LISTEN_ADDR":                   "127.0.0.1:9090",
		"UPSTREAM_ENDPOINT":             "http://collector.internal:4318",
		"UPSTREAM_TIMEOUT":              "10s",
		"MAX_BODY_BYTES":                "2048",
		"RATE_LIMIT_PER_MINUTE":         "600",
		"RATE_LIMIT_BURST":              "100",
		"TRUSTED_HOP_COUNT":             "2",
		"REQUIRE_PROTOBUF_CONTENT_TYPE": "false",
		"VALIDATE_PAYLOAD":              "true",
		"EXPECTED_SERVICE_NAME":         "custom",
		"LOG_LEVEL":                     "debug",
	}))
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:9090", cfg.ListenAddr)
	assert.Equal(t, "http://collector.internal:4318", cfg.UpstreamEndpoint)
	assert.Equal(t, 10*time.Second, cfg.UpstreamTimeout)
	assert.Equal(t, int64(2048), cfg.MaxBodyBytes)
	assert.InEpsilon(t, 600.0, cfg.RateLimitPerMinute, 0.0001)
	assert.Equal(t, 100, cfg.RateLimitBurst)
	assert.Equal(t, 2, cfg.TrustedHopCount)
	assert.False(t, cfg.RequireProtobufContentType)
	assert.True(t, cfg.ValidatePayload)
	assert.Equal(t, "custom", cfg.ExpectedServiceName)
	assert.Equal(t, slog.LevelDebug, cfg.LogLevel)
	assert.False(t, cfg.NoOp(), "non-empty upstream endpoint disables no-op mode")
}

func TestLoad_InvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		env  map[string]string
	}{
		{"bad duration", map[string]string{"UPSTREAM_TIMEOUT": "soon"}},
		{"non-positive duration", map[string]string{"UPSTREAM_TIMEOUT": "0s"}},
		{"bad int64", map[string]string{"MAX_BODY_BYTES": "huge"}},
		{"non-positive max body", map[string]string{"MAX_BODY_BYTES": "0"}},
		{"bad float", map[string]string{"RATE_LIMIT_PER_MINUTE": "fast"}},
		{"non-positive rate", map[string]string{"RATE_LIMIT_PER_MINUTE": "0"}},
		{"bad burst", map[string]string{"RATE_LIMIT_BURST": "lots"}},
		{"non-positive burst", map[string]string{"RATE_LIMIT_BURST": "0"}},
		{"negative hop count", map[string]string{"TRUSTED_HOP_COUNT": "-1"}},
		{"bad hop count", map[string]string{"TRUSTED_HOP_COUNT": "two"}},
		{"bad bool", map[string]string{"REQUIRE_PROTOBUF_CONTENT_TYPE": "maybe"}},
		{"bad log level", map[string]string{"LOG_LEVEL": "loud"}},
		{"bad upstream url", map[string]string{"UPSTREAM_ENDPOINT": "://nope"}},
		{"upstream url without scheme", map[string]string{"UPSTREAM_ENDPOINT": "collector.internal:4318"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(mapLookup(tt.env))
			require.Error(t, err)
		})
	}
}

func TestLoad_LogLevels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			cfg, err := Load(mapLookup(map[string]string{"LOG_LEVEL": tt.in}))
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg.LogLevel)
		})
	}
}
