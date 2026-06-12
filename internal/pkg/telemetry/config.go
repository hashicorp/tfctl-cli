// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package telemetry

import (
	"os"
	"strings"
)

// Mode represents the telemetry operating mode.
type Mode int

const (
	// ModeEnabled exports spans via OTLP to the configured endpoint.
	ModeEnabled Mode = iota

	// ModeDisabled produces no telemetry output (no-op).
	ModeDisabled

	// ModeLog writes span data as JSON to stderr.
	ModeLog
)

const (
	// EnvTelemetry is the environment variable controlling telemetry mode.
	EnvTelemetry = "TFCTL_TELEMETRY"

	// EnvDoNotTrack is the standard DO_NOT_TRACK environment variable.
	EnvDoNotTrack = "DO_NOT_TRACK"
)

// ResolveMode determines the telemetry mode from environment variables and
// the profile setting. Environment variables take precedence over the profile.
//
// Resolution order:
//  1. DO_NOT_TRACK=true → ModeDisabled
//  2. TFCTL_TELEMETRY=false/disabled → ModeDisabled
//  3. TFCTL_TELEMETRY=log → ModeLog
//  4. Profile telemetry=false/disabled → ModeDisabled
//  5. Profile telemetry=log → ModeLog
//  6. Otherwise → ModeEnabled
func ResolveMode(profileTelemetry string) Mode {
	// Check DO_NOT_TRACK first (standard)
	if strings.EqualFold(os.Getenv(EnvDoNotTrack), "true") ||
		os.Getenv(EnvDoNotTrack) == "1" {
		return ModeDisabled
	}

	// Check TFCTL_TELEMETRY env var
	if envVal := os.Getenv(EnvTelemetry); envVal != "" {
		return parseModeValue(envVal)
	}

	// Check profile setting
	if profileTelemetry != "" {
		return parseModeValue(profileTelemetry)
	}

	// Default: enabled
	return ModeEnabled
}

// parseModeValue interprets a telemetry configuration value.
// "false" or "disabled" → ModeDisabled.
// "log" → ModeLog.
// anything else → ModeEnabled.
func parseModeValue(val string) Mode {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "false", "disabled", "0", "off", "no":
		return ModeDisabled
	case "log":
		return ModeLog
	default:
		return ModeEnabled
	}
}

func (m Mode) String() string {
	switch m {
	case ModeEnabled:
		return "enabled"
	case ModeDisabled:
		return "disabled"
	case ModeLog:
		return "log"
	default:
		return "unknown"
	}
}
