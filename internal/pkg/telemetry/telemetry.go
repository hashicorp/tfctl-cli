// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package telemetry provides OpenTelemetry tracing for the tfctl CLI.
// It supports three modes: enabled (OTLP export), log (JSON to stderr),
// and disabled (no-op). Spans are buffered and flushed on Shutdown.
package telemetry

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

const (
	// serviceName is the service name reported in telemetry.
	serviceName = version.Name

	// shutdownTimeout is the maximum time allowed for flushing spans on shutdown.
	shutdownTimeout = 1 * time.Second

	// batchTimeout is set high to ensure spans are only flushed on shutdown,
	// not periodically during short-lived CLI execution.
	batchTimeout = 1 * time.Hour

	// DefaultHostname is the default OTLP endpoint hostname:port if not overridden by env var.
	DefaultHostname = "telemetry.terraform.io:4318"
)

// Config holds the configuration needed to initialize telemetry.
type Config struct {
	// InstallationID is the unique identifier for this CLI installation, used for telemetry purposes.
	InstallationID string

	// ProfileTelemetry is the value of the profile's telemetry setting.
	ProfileTelemetry string

	// Hostname is the profile hostname.
	Hostname string

	// Version is the CLI version string.
	Version string

	// ErrWriter is the writer for stderr output (used in log mode and for
	// emitting the traceparent header).
	ErrWriter io.Writer

	// IsTTY indicates whether the terminal is interactive.
	IsTTY bool
}

// CommandInfo contains metadata about the command being executed.
// This struct is used instead of raw OTel attributes so that callers
// do not need to import OTel packages directly.
type CommandInfo struct {
	// Command is the full command path (e.g., "run start").
	Command string

	// Profile is the active profile.
	Profile *profile.Profile

	// DryRun indicates whether --dry-run was specified.
	DryRun bool

	// Debug indicates whether --debug mode is enabled or verbose logging is enabled.
	Debug bool

	// JSON indicates whether --json output mode is enabled.
	JSON bool
}

// Telemetry manages the lifecycle of OpenTelemetry tracing for a CLI invocation.
type Telemetry struct {
	hostname       string
	mode           Mode
	provider       trace.TracerProvider
	sdkTP          *sdktrace.TracerProvider // nil when disabled
	tracer         trace.Tracer
	span           trace.Span
	parentCtx      context.Context
	errWriter      io.Writer
	isTTY          bool
	installationID string
}

// SetErrorHandler allows overriding the default OpenTelemetry error handler. By default,
// errors are ignored.
func SetErrorHandler(handler func(error)) {
	otel.SetErrorHandler(otel.ErrorHandlerFunc(handler))
}

func generateStableID(uuid string, id int) string {
	// Combine into a single string separator-style to prevent boundary collisions
	combined := fmt.Sprintf("%s:%d", uuid, id)

	// Hash it
	return fmt.Sprintf("%x", sha256.Sum256([]byte(combined)))
}

// Init creates and configures a Telemetry instance based on the resolved mode.
// If telemetry is disabled, a no-op instance is returned. Errors from exporter
// setup are non-fatal: telemetry should never break the CLI.
func Init(ctx context.Context, cfg Config) *Telemetry {
	mode := ResolveMode(cfg.ProfileTelemetry)

	t := &Telemetry{
		mode:           mode,
		errWriter:      cfg.ErrWriter,
		parentCtx:      ctx,
		hostname:       cfg.Hostname,
		isTTY:          cfg.IsTTY,
		installationID: cfg.InstallationID,
	}

	if mode == ModeDisabled {
		np := noop.NewTracerProvider()
		t.provider = np
		t.tracer = np.Tracer(serviceName)
		return t
	}

	// Build the resource with service metadata.
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(cfg.Version),
		semconv.TelemetrySDKLanguageGo,
		semconv.TelemetrySDKName("opentelemetry"),
		attribute.String("session_id", generateStableID(cfg.InstallationID, os.Getppid())),
	)

	// Create the exporter based on mode
	var (
		exporter sdktrace.SpanExporter
		err      error
	)
	switch mode {
	case ModeLog:
		exporter, err = stdouttrace.New(
			stdouttrace.WithWriter(cfg.ErrWriter),
			stdouttrace.WithPrettyPrint(),
		)
	case ModeEnabled:
		hostname := resolveEndpoint()
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(hostname),
		}
		if strings.HasPrefix(hostname, "localhost") {
			opts = append(opts, otlptracehttp.WithInsecure())
		}

		exporter, err = otlptracehttp.New(ctx, opts...)
	}

	if err != nil {
		// Telemetry setup failure is non-fatal
		np := noop.NewTracerProvider()
		t.provider = np
		t.tracer = np.Tracer(serviceName)
		t.mode = ModeDisabled
		return t
	}

	// Configure the batch span processor with a long timeout so that
	// spans are only flushed on explicit Shutdown(), not periodically.
	bsp := sdktrace.NewBatchSpanProcessor(exporter,
		sdktrace.WithBatchTimeout(batchTimeout),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	t.provider = tp
	t.sdkTP = tp
	t.tracer = tp.Tracer(serviceName)

	// Suppress OTel SDK internal errors (e.g. exporter connection failures)
	// from being printed to stderr. Telemetry errors are non-fatal and should
	// never produce visible output for the user.
	SetErrorHandler(otel.ErrorHandlerFunc(func(_ error) {}))

	// Parse TRACEPARENT from environment for context propagation
	t.parentCtx = extractTraceParent(ctx)

	return t
}

func detectAgent() string {
	if os.Getenv("OPENCODE") == "1" {
		return "opencode"
	}
	if os.Getenv("CLAUDECODE") == "1" {
		return "claudecode"
	}
	if os.Getenv("COPILOT_GH") == "true" || os.Getenv("COPILOT_CLI") == "1" {
		return "github_copilot"
	}
	return ""
}

// StartNetwork begins a new span representing an outgoing network request.
func (t *Telemetry) StartNetwork(ctx context.Context, name string, req *http.Request) (context.Context, trace.Span) {
	// Build attributes from Request
	attrs := []attribute.KeyValue{
		attribute.String("http.path", req.URL.Path),
		attribute.String("http.method", req.Method),
	}

	return t.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// StartCommand begins a new span representing the CLI command invocation.
// The span is stored internally and will be ended by Shutdown.
func (t *Telemetry) StartCommand(ctx context.Context, info CommandInfo) context.Context {
	if t.mode == ModeDisabled {
		return ctx
	}

	// Use the parent context (which may contain a remote parent from TRACEPARENT)
	if t.parentCtx != nil {
		if remoteSpanCtx := trace.SpanContextFromContext(t.parentCtx); remoteSpanCtx.IsValid() {
			ctx = trace.ContextWithRemoteSpanContext(ctx, remoteSpanCtx)
		}
	}

	// Build attributes from CommandInfo
	attrs := []attribute.KeyValue{
		attribute.String("installation_id", t.installationID),
		attribute.String("command", info.Command),
		attribute.Bool("dry_run_flag", info.DryRun),
		attribute.Bool("debug_flag", info.Debug),
		attribute.Bool("json_flag", info.JSON),
		attribute.String("os", runtime.GOOS),
		attribute.String("arch", runtime.GOARCH),
		attribute.Bool("is_ci", os.Getenv("CI") != ""),
		attribute.Bool("is_tty", t.isTTY),
	}

	if info.Profile != nil {
		attrs = append(attrs,
			attribute.String("hostname", info.Profile.GetHostname()),
			attribute.Bool("is_named_profile", info.Profile.Name != profile.ProfileNameDefault),
		)
	}

	if agent := detectAgent(); agent != "" {
		attrs = append(attrs, attribute.String("tfctl.agent", agent))
	}

	spanName := fmt.Sprintf("tfctl %s", info.Command)
	ctx, span := t.tracer.Start(ctx, spanName,
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindClient),
	)

	t.span = span
	return ctx
}

// Shutdown ends the active span, emits the traceparent to stderr, and flushes
// all buffered spans. It should be called once at the end of CLI execution.
func (t *Telemetry) Shutdown(ctx context.Context, exitStatus int) error {
	if t.mode == ModeDisabled {
		return nil
	}

	// End the command span
	if t.span != nil {
		t.span.SetAttributes(attribute.Int64("exit_status", int64(exitStatus)))
		t.span.End()
	}

	// Flush all pending spans
	if t.sdkTP != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()
		return t.sdkTP.Shutdown(shutdownCtx)
	}

	return nil
}

// Mode returns the resolved telemetry mode.
func (t *Telemetry) Mode() Mode {
	return t.mode
}

// resolveEndpoint returns the OTLP endpoint (host:port only, no scheme),
// checking the standard env var first and falling back to the default.
func resolveEndpoint() string {
	if ep := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); ep != "" {
		return stripScheme(ep)
	}
	return DefaultHostname
}

// stripScheme removes the http:// or https:// prefix from an endpoint URL.
func stripScheme(endpoint string) string {
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	return endpoint
}

// extractTraceParent reads the TRACEPARENT environment variable and extracts
// a remote span context from it using the W3C Trace Context propagator.
func extractTraceParent(ctx context.Context) context.Context {
	traceparent := os.Getenv("TRACEPARENT")
	if traceparent == "" {
		return ctx
	}

	prop := propagation.TraceContext{}
	carrier := propagation.MapCarrier{
		"traceparent": traceparent,
	}

	// Also check for tracestate.
	if ts := os.Getenv("TRACESTATE"); ts != "" {
		carrier["tracestate"] = ts
	}

	return prop.Extract(ctx, carrier)
}
