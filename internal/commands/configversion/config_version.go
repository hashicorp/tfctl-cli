// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package configversion implements the config-version command group.
package configversion

import (
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdConfigVersion creates the config-version command group.
func NewCmdConfigVersion(inv *cmd.Invocation) *cmd.Command {
	c := &cmd.Command{
		Name:      "config-version",
		ShortHelp: "Upload Terraform config to a workspace.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		Use {{ template "mdCodeOrBold" "%s config-version upload" }} to pack and upload
		Terraform config to a workspace without queuing a run by default.
		`, version.Name),
	}
	c.AddChild(newCmdUpload(inv))
	return c
}
