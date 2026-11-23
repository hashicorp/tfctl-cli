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
		The {{ template "mdCodeOrBold" "%s config-version" }} command group can package and
		upload Terraform config to a workspace. By default, it does not queue a run.
		`, version.Name),
	}
	c.AddChild(newCmdUpload(inv))
	return c
}
