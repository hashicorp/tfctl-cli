// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"context"
	"testing"

	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
)

func TestInvocationDryRunHelper(t *testing.T) {
	t.Parallel()

	inv := &Invocation{IO: iostreams.Test(), ShutdownCtx: context.Background()}
	inv.flags.parsed = true
	inv.flags.dryRun = true

	if !inv.IsDryRun() {
		t.Fatal("expected dry-run to be enabled")
	}
}
