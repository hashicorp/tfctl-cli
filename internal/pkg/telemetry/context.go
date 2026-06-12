package telemetry

import (
	"context"
)

type ctxKey string

var telemetryKey ctxKey = "telemetry"

// WithTelemetry returns a new context with the provided telemetry.
func WithTelemetry(ctx context.Context, tel *Telemetry) context.Context {
	return context.WithValue(ctx, telemetryKey, tel)
}

// FromContext extracts the telemetry from the context, or returns nil if not found.
func FromContext(ctx context.Context) *Telemetry {
	if tel, ok := ctx.Value(telemetryKey).(*Telemetry); ok {
		return tel
	}
	return nil
}
