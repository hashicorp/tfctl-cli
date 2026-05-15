// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package root implements the root command for the CLI.
package root

import (
	"fmt"

	"github.com/hashicorp/tfctl-cli/internal/commands/api"
	"github.com/hashicorp/tfctl-cli/internal/commands/auth"
	"github.com/hashicorp/tfctl-cli/internal/commands/harness"
	"github.com/hashicorp/tfctl-cli/internal/commands/profile"
	"github.com/hashicorp/tfctl-cli/internal/commands/run"
	"github.com/hashicorp/tfctl-cli/internal/commands/variable"
	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
)

// NewCmdRoot creates the root command.
func NewCmdRoot(ctx *cmd.Context) *cmd.Command {
	c := &cmd.Command{
		Name:      config.Name,
		ShortHelp: "Interact with HCP Terraform and Terraform Enterprise.",
		LongHelp:  fmt.Sprintf("The %s command-line interface (CLI) is a unified tool for managing HCP Terraform and Terraform Enterprise from the command line.", config.Name),
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
	c.AddChild(api.NewCmdAPI(ctx))
	c.AddChild(run.NewCmdRun(ctx))
	c.AddChild(auth.NewCmdAuth(ctx))
	c.AddChild(variable.NewCmdVariable(ctx))
	c.AddChild(profile.NewCmdProfile(ctx))
	c.AddChild(harness.NewCmdHarness(ctx))

	// Configure the command as the root command.
	cmd.ConfigureRootCommand(ctx, c)

	return c
}
