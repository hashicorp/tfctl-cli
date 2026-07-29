// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package logging provides utilities for creating and accessing a logger within the application.
package logging

import (
	"context"
	"time"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/version"
)

type ctxKey struct{}

const (
	// LevelDefault is the default logging level for the application, which is error.
	LevelDefault = hclog.Error

	// LevelDebug is the logging level that includes debug messages, which is more verbose than the default.
	LevelDebug = hclog.Debug
)

var (
	loggingKey = ctxKey{}
)

// WithLogger returns a new context with the provided logger.
func WithLogger(ctx context.Context, logger hclog.Logger) context.Context {
	return context.WithValue(ctx, loggingKey, logger)
}

// FromContext extracts the logger from the context, or returns a null logger if not found.
func FromContext(ctx context.Context) hclog.Logger {
	if logger, ok := ctx.Value(loggingKey).(hclog.Logger); ok {
		return logger
	}
	return hclog.NewNullLogger()
}

// NewLogger constructs a new logger configured based on the provided IOStreams.
func NewLogger(io iostreams.IOStreams, initialLevel hclog.Level) hclog.Logger {
	// Create the Logger
	logOpt := &hclog.LoggerOptions{
		Name:       version.Name,
		Level:      initialLevel,
		Output:     io.ErrUnessential(),
		TimeFn:     time.Now,
		TimeFormat: "15:04:05.000",
		Color:      hclog.ColorOff, // Enabled later, maybe
	}

	if io.ColorEnabled() && io.IsErrorTTY() {
		logOpt.Color = hclog.ForceColor
		logOpt.ColorHeaderAndFields = true
	}

	return hclog.New(logOpt)
}
