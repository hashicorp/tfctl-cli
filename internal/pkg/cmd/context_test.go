// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"context"
	"testing"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestContextDryRunHelper(t *testing.T) {
	t.Parallel()

	ctx := &Context{IO: iostreams.Test(), ShutdownCtx: context.Background()}
	ctx.flags.parsed = true
	ctx.flags.dryRun = true

	if !ctx.IsDryRun() {
		t.Fatal("expected dry-run to be enabled")
	}
}
