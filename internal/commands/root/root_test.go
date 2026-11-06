// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package root

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

func TestNewCmdRootRegistersModulePublish(t *testing.T) {
	t.Parallel()

	io := iostreams.Test()
	inv := &cmd.Invocation{
		IO:          io,
		Output:      format.New(io),
		ShutdownCtx: context.Background(),
		Profile:     profile.TestProfile(t),
	}
	commands := cmd.ToCommandMap(NewCmdRoot(inv), inv)

	require.Contains(t, commands, "module")
	require.Contains(t, commands, "module publish")
}
