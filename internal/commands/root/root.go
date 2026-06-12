// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package root implements the root command for the CLI.
package root

import (
	"fmt"

	"github.com/hashicorp/tfctl-cli/internal/commands/api"
	"github.com/hashicorp/tfctl-cli/internal/commands/auth"
	"github.com/hashicorp/tfctl-cli/internal/commands/create"
	"github.com/hashicorp/tfctl-cli/internal/commands/get"
	"github.com/hashicorp/tfctl-cli/internal/commands/harness"
	"github.com/hashicorp/tfctl-cli/internal/commands/profile"
	"github.com/hashicorp/tfctl-cli/internal/commands/run"
	"github.com/hashicorp/tfctl-cli/internal/commands/variable"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdRoot creates the root command.
func NewCmdRoot(inv *cmd.Invocation) *cmd.Command {
	c := &cmd.Command{
		Name:      version.Name,
		ShortHelp: "Interact with HCP Terraform and Terraform Enterprise.",
		LongHelp:  fmt.Sprintf("The %s command-line interface (CLI) is a unified tool for managing HCP Terraform and Terraform Enterprise from the command line.", version.Name),
	}

	//  _   _  ___ _____ _____
	// | \ | |/ _ \_   _| ____|
	// |  \| | | | || | |  _|
	// | |\  | |_| || | | |___
	// |_| \_|\___/ |_| |_____|
	//
	// When adding a top level command group, be sure to regenerate the
	// screenshot in the README by running `make gen/screenshot`.

	// Add the subcommands
	c.AddChild(api.NewCmdAPI(inv))
	c.AddChild(get.NewCmdGet(inv))
	c.AddChild(create.NewCmdCreate(inv))
	c.AddChild(run.NewCmdRun(inv))
	c.AddChild(auth.NewCmdAuth(inv))
	c.AddChild(variable.NewCmdVariable(inv))
	c.AddChild(profile.NewCmdProfile(inv))
	c.AddChild(harness.NewCmdHarness(inv))

	// Configure the command as the root command.
	cmd.ConfigureRootCommand(inv, c)

	return c
}
