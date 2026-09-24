// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package module implements the `module` command group.
package module

import (
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdModule creates the `module` command group.
func NewCmdModule(inv *cmd.Invocation) *cmd.Command {
	c := &cmd.Command{
		Name:      "module",
		ShortHelp: "Manage private registry modules.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s module" }} command group lets you manage
		private registry modules in HCP Terraform and Terraform Enterprise.
		`, version.Name),
	}

	c.AddChild(NewCmdPublish(inv))

	return c
}
