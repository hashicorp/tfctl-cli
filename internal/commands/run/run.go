// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package run implements the `tfcloud run` command group.
package run

import (
	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
)

// NewCmdRun creates the `tfcloud run` command.
func NewCmdRun(ctx *cmd.Context) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "run",
		ShortHelp: "Inspect and manage runs.",
		LongHelp: heredoc.New(ctx.IO).Must(`
		The {{ template "mdCodeOrBold" "tfcloud run" }} command group lets you inspect and manage
		Terraform runs in HCP Terraform and Terraform Enterprise.
		`),
	}

	cmd.AddChild(NewCmdRunStatus(ctx))

	return cmd
}
