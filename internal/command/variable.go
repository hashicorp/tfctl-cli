// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package command

import (
	"github.com/hashicorp/tfcloud/internal/cmd"
	"github.com/hashicorp/tfcloud/internal/heredoc"
)

// NewCmdVariable creates the `tfcloud variable` command.
func NewCmdVariable(ctx *cmd.Context) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "variable",
		ShortHelp: "Manage variables in workspaces or variable sets.",
		LongHelp: heredoc.New(ctx.IO).Must(`
		The {{ template "mdCodeOrBold" "tfcloud variable" }} command group lets you manage Terraform or
		environment variables belonging to Workspaces or Variable Sets.
		`),
	}

	cmd.AddChild(NewCmdVariableImport(ctx))

	return cmd
}
