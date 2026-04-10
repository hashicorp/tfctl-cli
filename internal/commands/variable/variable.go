// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package variable implements the `tfcloud variable` command group.
package variable

import (
	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
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
