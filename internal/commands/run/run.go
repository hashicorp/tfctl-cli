// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package run implements the `run` command group.
package run

import (
	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
)

// NewCmdRun creates the `run` command.
func NewCmdRun(ctx *cmd.Context) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "run",
		ShortHelp: "Inspect and manage runs.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s run" }} command group lets you inspect and manage
		Terraform runs in HCP Terraform and Terraform Enterprise.
		`, config.Name),
	}

	cmd.AddChild(NewCmdRunStatus(ctx))
	cmd.AddChild(NewCmdRunStart(ctx))

	return cmd
}
