// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profiles

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdActivate returns the `profile profiles activate` command for activating a configuration profile.
func NewCmdActivate(inv *cmd.Invocation) *cmd.Command {
	opts := &ActivateOpts{
		IO: inv.IO,
	}
	cmd := &cmd.Command{
		Name:      "activate",
		ShortHelp: "Activates an existing profile.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile profiles activate" }} command activates an existing profile.
		`, version.Name),
		Examples: []cmd.Example{
			{
				Preamble: heredoc.New(inv.IO).Must(`
				To active profile {{ template "mdCodeOrBold" "my-profile" }}, run:
				`),
				Command: fmt.Sprintf("$ %s profile profiles activate my-profile", version.Name),
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
		RunF: func(_ *cmd.Command, args []string) error {
			opts.Name = args[0]
			opts.DryRun = inv.IsDryRun()
			l, err := profile.NewLoader()
			if err != nil {
				return err
			}
			opts.Profiles = l
			return activateRun(inv.ShutdownCtx, opts)
		},
	}

	return cmd
}

// ActivateOpts defines the options for the `profile profiles activate` command.
type ActivateOpts struct {
	IO       iostreams.IOStreams
	Profiles *profile.Loader
	Name     string
	DryRun   bool
}

func activateRun(ctx context.Context, opts *ActivateOpts) error {
	logger := logging.FromContext(ctx)
	logger.Debug("activating profile", "name", opts.Name)

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
