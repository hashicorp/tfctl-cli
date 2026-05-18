// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profiles

import (
	"fmt"
	"slices"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

// NewCmdActivate returns the `profile profiles activate` command for activating a configuration profile.
func NewCmdActivate(ctx *cmd.Context) *cmd.Command {
	opts := &ActivateOpts{
		IO: ctx.IO,
	}
	cmd := &cmd.Command{
		Name:      "activate",
		ShortHelp: "Activates an existing profile.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile profiles activate" }} command activates an existing profile.
		`, config.Name),
		Examples: []cmd.Example{
			{
				Preamble: heredoc.New(ctx.IO).Must(`
				To active profile {{ template "mdCodeOrBold" "my-profile" }}, run:
				`),
				Command: fmt.Sprintf("$ %s profile profiles activate my-profile", config.Name),
			},
		},
		Args: cmd.PositionalArguments{
			Autocomplete: PredictProfiles(false, false),
			Args: []cmd.PositionalArgument{
				{
					Name:          "NAME",
					Documentation: "The name of the profile to activate.",
				},
			},
		},
		NoAuthRequired: true,
		RunF: func(c *cmd.Command, args []string) error {
			opts.Name = args[0]
			opts.Logger = c.Logger(ctx)
			opts.DryRun = ctx.IsDryRun()
			l, err := profile.NewLoader()
			if err != nil {
				return err
			}
			opts.Profiles = l
			return activateRun(opts)
		},
	}

	return cmd
}

// ActivateOpts defines the options for the `profile profiles activate` command.
type ActivateOpts struct {
	IO       iostreams.IOStreams
	Logger   hclog.Logger
	Profiles *profile.Loader
	Name     string
	DryRun   bool
}

func activateRun(opts *ActivateOpts) error {
	opts.Logger.Debug("activating profile", "name", opts.Name)

	// Get the active profile
	active, err := opts.Profiles.GetActiveProfile()
	if err != nil {
		return fmt.Errorf("failed to get active profile: %w", err)
	}

	// Ensure the given profile isn't already the active profile
	if active.Name == opts.Name {
		return fmt.Errorf("profile %q is already the active profile", opts.Name)
	}

	// Ensure the given profile exists.
	profileNames, err := opts.Profiles.ListProfiles()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	if !slices.Contains(profileNames, opts.Name) {
		return fmt.Errorf("profile %q does not exist", opts.Name)
	}

	if opts.DryRun {
		fmt.Fprintf(opts.IO.Err(), "%s would activate profile %q\n", opts.IO.ColorScheme().DryRunLabel(), opts.Name)
		return nil
	}

	// Save the new active profile
	active.Name = opts.Name
	if err := active.Write(); err != nil {
		return fmt.Errorf("failed to save active profile: %w", err)
	}

	fmt.Fprintf(opts.IO.Err(), "%s Profile %q activated.\n",
		opts.IO.ColorScheme().SuccessIcon(), opts.Name)
	return nil
}
