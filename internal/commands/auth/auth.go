// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package auth implements the `auth` command group for managing authentication.
package auth

import (
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdAuth returns the `auth` command for managing authentication.
func NewCmdAuth(ctx *cmd.Context) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "auth",
		ShortHelp: "Authenticate with HCP Terraform.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s auth" }} command group lets you
		authenticate the tfcloud CLI with HCP Terraform or Terraform Enterprise.
		`, version.Name),
	}

	cmd.AddChild(NewCmdLogin(ctx))
	cmd.AddChild(NewCmdStatus(ctx))
	return cmd
}
