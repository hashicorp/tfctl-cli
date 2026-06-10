// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package variable implements the `variable` command group.
package variable

import (
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdVariable creates the `variable` command.
func NewCmdVariable(inv *cmd.Invocation) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "variable",
		ShortHelp: "Manage variables in workspaces or variable sets.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s variable" }} command group lets you manage Terraform or
		environment variables belonging to Workspaces or Variable Sets.
		`, version.Name),
	}

	cmd.AddChild(NewCmdVariableImport(inv))

	return cmd
}
