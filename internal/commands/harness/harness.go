// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package harness implements the `harness` command group.
package harness

import (
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdHarness creates the `harness` command.
func NewCmdHarness(inv *cmd.Invocation) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "harness",
		ShortHelp: "Install coding agent skills and see agent context.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s harness" }} command group lets you install coding agent skills and see printed coding agent context.
		`, version.Name),
	}

	cmd.AddChild(NewCmdHarnessContext(inv))
	cmd.AddChild(NewCmdHarnessInstall(inv))

	return cmd
}
