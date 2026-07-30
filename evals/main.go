// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package main runs deterministic evaluations of the tfctl skill.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/hashicorp/tfctl-cli/evals/internal/run"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run.Main(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
