// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package run implements the `run` command group.
package run

import (
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdRun creates the `run` command.
func NewCmdRun(inv *cmd.Invocation) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "run",
		ShortHelp: "Inspect and manage runs.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s run" }} command group lets you diagnose and start
		Terraform runs in HCP Terraform and Terraform Enterprise.
		`, version.Name),
	}

	cmd.AddChild(NewCmdRunStatus(inv))
	cmd.AddChild(NewCmdRunStart(inv))

	// Hidden commands:
	cmd.AddChild(NewCmdRunStatusSample(inv))

	return cmd
}
