// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package telemetry

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

// ============================================================
// Mode Resolution Tests
// ============================================================

func TestResolveMode_Defaults(t *testing.T) {
	clearEnv(t)
	assert.Equal(t, ModeEnabled, ResolveMode(""))
}

func TestResolveMode_DONotTrack(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  Mode
	}{
		{"true", "true", ModeDisabled},
		{"TRUE", "TRUE", ModeDisabled},
		{"1", "1", ModeDisabled},
		{"false does not disable", "false", ModeEnabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(EnvDoNotTrack, tt.value)
			assert.Equal(t, tt.want, ResolveMode(""))
		})
	}
}

func TestResolveMode_TFCTLTelemetry(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  Mode
	}{
		{"false", "false", ModeDisabled},
		{"disabled", "disabled", ModeDisabled},
		{"0", "0", ModeDisabled},
		{"off", "off", ModeDisabled},
		{"no", "no", ModeDisabled},
		{"log", "log", ModeLog},
		{"LOG uppercase", "LOG", ModeLog},
		{"true", "true", ModeEnabled},
		{"empty string falls through", "", ModeEnabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			if tt.value != "" {
				t.Setenv(EnvTelemetry, tt.value)
			}
			assert.Equal(t, tt.want, ResolveMode(""))
		})
	}
}

func TestResolveMode_ProfileSetting(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    Mode
	}{
		{"false", "false", ModeDisabled},
		{"disabled", "disabled", ModeDisabled},
		{"log", "log", ModeLog},
		{"true", "true", ModeEnabled},
		{"anything else enables", "something", ModeEnabled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			assert.Equal(t, tt.want, ResolveMode(tt.profile))
		})
	}
}

func TestResolveMode_EnvTakesPrecedenceOverProfile(t *testing.T) {
	clearEnv(t)
	// Profile says log, but env says disabled.
	t.Setenv(EnvTelemetry, "false")
	assert.Equal(t, ModeDisabled, ResolveMode("log"))
}

func TestResolveMode_DONotTrackTakesPrecedenceOverAll(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvDoNotTrack, "true")
	t.Setenv(EnvTelemetry, "true")
	assert.Equal(t, ModeDisabled, ResolveMode("true"))
}

// ============================================================
// Init Tests
// ============================================================

func TestInit_DisabledMode(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvTelemetry, "false")

	var buf bytes.Buffer
	tel := Init(context.Background(), Config{
		ErrWriter: &buf,
		Version:   "0.1.0",
	})

	assert.Equal(t, ModeDisabled, tel.Mode())
	assert.NotNil(t, tel.tracer)
}

func TestInit_LogMode(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvTelemetry, "log")

	var buf bytes.Buffer
	tel := Init(context.Background(), Config{
		ErrWriter: &buf,
		Version:   "0.1.0",
	})

	assert.Equal(t, ModeLog, tel.Mode())
	assert.NotNil(t, tel.sdkTP)
}

func TestInit_EnabledMode(t *testing.T) {
	clearEnv(t)

	var buf bytes.Buffer
	tel := Init(context.Background(), Config{
		ErrWriter: &buf,
		Version:   "0.1.0",
	})

	assert.Equal(t, ModeEnabled, tel.Mode())
	assert.NotNil(t, tel.sdkTP)
}

func TestInit_ProfileDisabled(t *testing.T) {
	clearEnv(t)

	var buf bytes.Buffer
	tel := Init(context.Background(), Config{
		ProfileTelemetry: "disabled",
		ErrWriter:        &buf,
		Version:          "0.1.0",
	})

	assert.Equal(t, ModeDisabled, tel.Mode())
}

// ============================================================
// StartCommand Tests
// ============================================================

func TestStartCommand_DisabledMode_NoSpan(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvTelemetry, "false")

	var buf bytes.Buffer
	tel := Init(context.Background(), Config{
		ErrWriter: &buf,
		Version:   "0.1.0",
	})

	ctx := tel.StartCommand(context.Background(), CommandInfo{
		Command: "run start",
	})
	assert.NotNil(t, ctx)
	// No span should be stored.
	assert.Nil(t, tel.span)
}

func TestStartCommand_CreatesSpanWithAttributes(t *testing.T) {
	clearEnv(t)

	tel, exporter := newTestTelemetry(t)

	ctx := tel.StartCommand(context.Background(), CommandInfo{
		Command: "run start",
		Profile: profile.TestProfile(t),
		DryRun:  true,
	})
	require.NotNil(t, ctx)
	require.NotNil(t, tel.span)

	// End the span so it gets exported.
	tel.span.End()

	// Force flush.
	err := tel.sdkTP.ForceFlush(context.Background())
	require.NoError(t, err)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	s := spans[0]
	assert.Equal(t, "tfctl run start", s.Name)

	// Check attributes.
	attrs := spanAttrMap(s)
	assert.Equal(t, "run start", attrs["command"])
	assert.Equal(t, true, attrs["dry_run_flag"])
	assert.Equal(t, false, attrs["is_tty"])
	assert.NotEmpty(t, attrs["os"])
	assert.NotEmpty(t, attrs["arch"])
}

func TestStartCommand_IncludesCIAttribute(t *testing.T) {
	clearEnv(t)
	t.Setenv("CI", "true")

	tel, exporter := newTestTelemetry(t)

	tel.StartCommand(context.Background(), CommandInfo{Command: "api get", Profile: profile.TestProfile(t)})
	tel.span.End()
	require.NoError(t, tel.sdkTP.ForceFlush(context.Background()))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	attrs := spanAttrMap(spans[0])
	assert.Equal(t, true, attrs["is_ci"])
}

func TestStartCommand_NoCIAttribute(t *testing.T) {
	clearEnv(t)
	os.Unsetenv("CI")

	tel, exporter := newTestTelemetry(t)

	tel.StartCommand(context.Background(), CommandInfo{Command: "api get"})
	tel.span.End()
	require.NoError(t, tel.sdkTP.ForceFlush(context.Background()))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	attrs := spanAttrMap(spans[0])
	assert.Equal(t, false, attrs["is_ci"])
}

// ============================================================
// StartNetwork Tests
// ============================================================

func TestStartNetwork_ParentIsCommandSpan(t *testing.T) {
	clearEnv(t)

	tel, exporter := newTestTelemetry(t)

	// Start the command span — returns a context carrying the command span.
	cmdCtx := tel.StartCommand(context.Background(), CommandInfo{
		Command: "api get",
		Profile: profile.TestProfile(t),
	})

	// Simulate an HTTP request using the command context.
	req, err := http.NewRequestWithContext(cmdCtx, http.MethodGet, "https://app.terraform.io/api/v2/runs/run-abc123", nil)
	require.NoError(t, err)

	// StartNetwork should create a child span under the command span.
	_, netSpan := tel.StartNetwork(cmdCtx, "HTTP GET", req)
	netSpan.End()
	tel.span.End()

	require.NoError(t, tel.sdkTP.ForceFlush(context.Background()))

	spans := exporter.GetSpans()
	require.Len(t, spans, 2)

	// Identify spans by name.
	var cmdSpan, networkSpan tracetest.SpanStub
	for _, s := range spans {
		switch s.Name {
		case "tfctl api get":
			cmdSpan = s
		case "HTTP GET":
			networkSpan = s
		}
	}

	require.NotEmpty(t, cmdSpan.Name, "command span not found")
	require.NotEmpty(t, networkSpan.Name, "network span not found")

	// The network span's parent span ID must equal the command span's span ID.
	assert.Equal(t, cmdSpan.SpanContext.SpanID(), networkSpan.Parent.SpanID(),
		"network span should be a child of the command span")

	// Both spans must share the same trace ID.
	assert.Equal(t, cmdSpan.SpanContext.TraceID(), networkSpan.SpanContext.TraceID(),
		"network span must belong to the same trace as the command span")
}

func TestStartNetwork_WithoutCommandContext_NoParent(t *testing.T) {
	clearEnv(t)

	tel, exporter := newTestTelemetry(t)

	// Use a bare context (no command span) — simulates the bug where the
	// stale context is passed to StartNetwork.
	bareCtx := context.Background()

	req, err := http.NewRequestWithContext(bareCtx, http.MethodGet, "https://app.terraform.io/api/v2/workspaces", nil)
	require.NoError(t, err)

	_, netSpan := tel.StartNetwork(bareCtx, "HTTP GET", req)
	netSpan.End()

	require.NoError(t, tel.sdkTP.ForceFlush(context.Background()))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	// With no parent context, the network span should have no valid parent.
	assert.False(t, spans[0].Parent.IsValid(),
		"network span should have no parent when created without the command context")
}

// ============================================================
// TRACEPARENT Propagation Tests
// ============================================================

func TestInit_ParsesTraceparentEnv(t *testing.T) {
	clearEnv(t)
	// Set a valid TRACEPARENT.
	t.Setenv("TRACEPARENT", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")

	tel, exporter := newTestTelemetry(t)

	tel.StartCommand(context.Background(), CommandInfo{Command: "run start"})
	tel.span.End()
	require.NoError(t, tel.sdkTP.ForceFlush(context.Background()))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	s := spans[0]
	// The span should have the trace ID from TRACEPARENT.
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", s.SpanContext.TraceID().String())
	// The parent span ID should match.
	assert.Equal(t, "00f067aa0ba902b7", s.Parent.SpanID().String())
}

func TestInit_InvalidTraceparent_NoError(t *testing.T) {
	clearEnv(t)
	t.Setenv("TRACEPARENT", "not-valid")

	var buf bytes.Buffer
	tel := Init(context.Background(), Config{
		ErrWriter: &buf,
		Version:   "0.1.0",
	})

	// Should not panic or error, just create a new trace.
	tel.StartCommand(context.Background(), CommandInfo{Command: "run start"})
	assert.NotNil(t, tel.span)
	assert.True(t, tel.span.SpanContext().IsValid())
	tel.span.End()
}

func TestInit_NoTraceparent_NewTrace(t *testing.T) {
	clearEnv(t)

	tel, exporter := newTestTelemetry(t)

	tel.StartCommand(context.Background(), CommandInfo{Command: "run start"})
	tel.span.End()
	require.NoError(t, tel.sdkTP.ForceFlush(context.Background()))

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)

	// The span should have no parent.
	assert.False(t, spans[0].Parent.IsValid())
}

// ============================================================
// Shutdown Tests
// ============================================================

func TestShutdown_DisabledMode_NoOutput(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvTelemetry, "false")

	var buf bytes.Buffer
	tel := Init(context.Background(), Config{
		ErrWriter: &buf,
		Version:   "0.1.0",
	})

	tel.StartCommand(context.Background(), CommandInfo{Command: "run start"})
	err := tel.Shutdown(context.Background(), 0)
	require.NoError(t, err)

	assert.Empty(t, buf.String())
}

func TestShutdown_EndsSpanAndFlushes(t *testing.T) {
	clearEnv(t)

	tel, exporter := newTestTelemetry(t)

	tel.StartCommand(context.Background(), CommandInfo{Command: "run start"})

	// Span is still active (not ended).
	require.True(t, tel.span.IsRecording())

	// Verify span is exported immediately on End() with SimpleSpanProcessor.
	// Call End explicitly to test that Shutdown still works when span already ended.
	tel.span.End()

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "tfctl run start", spans[0].Name)

	// Shutdown should still emit traceparent and not error.
	var buf bytes.Buffer
	tel.errWriter = &buf
	err := tel.Shutdown(context.Background(), 0)
	require.NoError(t, err)
}

func TestShutdown_AddsExitStatus(t *testing.T) {
	clearEnv(t)

	t.Setenv(EnvTelemetry, "log")
	var buf bytes.Buffer
	tel := Init(context.Background(), Config{
		ErrWriter: &buf,
		Version:   "0.1.0",
	})

	tel.StartCommand(context.Background(), CommandInfo{Command: "run start"})

	// Shutdown should add the exit status attribute to the span and flush.
	err := tel.Shutdown(context.Background(), 0)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "exit_status")
}

// ============================================================
// Log Mode Tests
// ============================================================

func TestLogMode_WritesSpanToWriter(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvTelemetry, "log")

	var buf bytes.Buffer
	tel := Init(context.Background(), Config{
		ErrWriter: &buf,
		Version:   "0.1.0",
	})

	require.Equal(t, ModeLog, tel.Mode())

	tel.StartCommand(context.Background(), CommandInfo{Command: "variable import"})

	err := tel.Shutdown(context.Background(), 0)
	require.NoError(t, err)

	output := buf.String()
	// In log mode, the stdout exporter writes JSON-like span data.
	assert.Contains(t, output, "tfctl variable import")
}

// ============================================================
// Endpoint Resolution Tests
// ============================================================

func TestResolveEndpoint_Default(t *testing.T) {
	clearEnv(t)
	assert.Equal(t, "telemetry.terraform.io:4318", resolveEndpoint())
}

func TestResolveEndpoint_EnvOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector.example.com:4318")
	assert.Equal(t, "collector.example.com:4318", resolveEndpoint())
}

// ============================================================
// Mode String Tests
// ============================================================

func TestModeString(t *testing.T) {
	assert.Equal(t, "enabled", ModeEnabled.String())
	assert.Equal(t, "disabled", ModeDisabled.String())
	assert.Equal(t, "log", ModeLog.String())
}

// ============================================================
// Helpers
// ============================================================

// clearEnv unsets all telemetry-related env vars for test isolation.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvTelemetry, "")
	t.Setenv(EnvDoNotTrack, "")
	t.Setenv("TRACEPARENT", "")
	t.Setenv("TRACESTATE", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("CI", "")
}

// newTestTelemetry creates a Telemetry instance with an in-memory exporter for testing.
func newTestTelemetry(t *testing.T) (*Telemetry, *tracetest.InMemoryExporter) {
	t.Helper()

	exporter := tracetest.NewInMemoryExporter()

	// Use a simple span processor for immediate export in tests.
	sp := sdktrace.NewSimpleSpanProcessor(exporter)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sp),
	)

	tel := &Telemetry{
		mode:      ModeEnabled,
		provider:  tp,
		sdkTP:     tp,
		tracer:    tp.Tracer(serviceName),
		parentCtx: extractTraceParent(context.Background()),
		errWriter: &bytes.Buffer{},
	}

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	return tel, exporter
}

// spanAttrMap converts a span's attributes to a simple map for assertion.
func spanAttrMap(s tracetest.SpanStub) map[string]interface{} {
	m := make(map[string]interface{})
	for _, attr := range s.Attributes {
		switch attr.Value.Type() {
		case attribute.STRING:
			m[string(attr.Key)] = attr.Value.AsString()
		case attribute.BOOL:
			m[string(attr.Key)] = attr.Value.AsBool()
		case attribute.INT64:
			m[string(attr.Key)] = attr.Value.AsInt64()
		case attribute.FLOAT64:
			m[string(attr.Key)] = attr.Value.AsFloat64()
		default:
			m[string(attr.Key)] = attr.Value.AsString()
		}
	}
	return m
}
