// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package tfcloud implements the root command for the tfcloud CLI.
package tfcloud

import (
	"github.com/hashicorp/tfcloud/internal/commands/api"
	"github.com/hashicorp/tfcloud/internal/commands/profile"
	"github.com/hashicorp/tfcloud/internal/commands/variable"
	"github.com/hashicorp/tfcloud/internal/config"
	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
)

// NewCmdRoot creates the root command.
func NewCmdRoot(ctx *cmd.Context) *cmd.Command {
	c := &cmd.Command{
		Name:      config.Name,
		ShortHelp: "Interact with HCP Terraform and Terraform Enterprise.",
		LongHelp:  "The tfcloud command-line interface (CLI) is a unified tool for managing HCP Terraform and Terraform Enterprise from the command line.",
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
	c.AddChild(variable.NewCmdVariable(ctx))
	c.AddChild(profile.NewCmdProfile(ctx))

	// Configure the command as the root command.
	cmd.ConfigureRootCommand(ctx, c)

	return c
}
