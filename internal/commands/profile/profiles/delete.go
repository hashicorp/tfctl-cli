// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profiles

import (
	"context"
	"fmt"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdDelete returns the `profile profiles delete` command for deleting configuration profiles.
func NewCmdDelete(inv *cmd.Invocation) *cmd.Command {
	opts := &DeleteOpts{
		IO: inv.IO,
	}
	cmd := &cmd.Command{
		Name:      "delete",
		ShortHelp: "Delete an existing configuration profile.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile profiles delete" }} command
		deletes an existing configuration profile. If the profile is the active profile,
		it may not be deleted.

		To delete the current active profile, first run {{ template "mdCodeOrBold" "%s profile profiles activate" }}
		to active a different profile.
		`, version.Name, version.Name),
		Examples: []cmd.Example{
			{
				Preamble: "Delete a profile:",
				Command:  fmt.Sprintf("$ %s profile profiles delete my-profile", version.Name),
			},
			{
				Preamble: "Delete multiple profiles:",
				Command:  fmt.Sprintf("$ %s profile profiles delete my-profile-1 my-profile-2 my-profile-3", version.Name),
			},
			{
				Preamble: "Delete the active profile:",
				Command: heredoc.New(inv.IO).Mustf(`
				$ %s profile profiles active my-other-profile
				$ %s profile profiles delete my-profile
				`, version.Name, version.Name),
			},
		},
		NoAuthRequired: true,
		Args: cmd.PositionalArguments{
			Autocomplete: PredictProfiles(true, false),
			Args: []cmd.PositionalArgument{
				{
					Name:          "PROFILE_NAMES",
					Documentation: "The name of the profile to delete. May not be the active profile.",
					Repeatable:    true,
				},
			},
		},
		RunF: func(_ *cmd.Command, args []string) error {
			l, err := profile.NewLoader()
			if err != nil {
				return err
			}
			opts.Profiles = l
			opts.Names = args
			opts.DryRun = inv.IsDryRun()
			return deleteRun(inv.ShutdownCtx, opts)
		},
	}

	return cmd
}

// DeleteOpts defines the options for the `profile profiles delete` command.
type DeleteOpts struct {
	IO       iostreams.IOStreams
	Profiles *profile.Loader
	Names    []string
	DryRun   bool
}

func deleteRun(ctx context.Context, opts *DeleteOpts) error {
	logger := logging.FromContext(ctx)
	logger.Debug("deleting profiles", "names", opts.Names)

	// Get the active profile
	active, err := opts.Profiles.GetActiveProfile()
	if err != nil {
		return fmt.Errorf("failed to get active profile: %w", err)
	}

	profileNames, err := opts.Profiles.ListProfiles()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	// Validate that the given profiles to delete aren't active and that they
	// all exist.
	existing := make(map[string]struct{}, len(profileNames))
	for _, p := range profileNames {
		existing[p] = struct{}{}
	}

	cs := opts.IO.ColorScheme()
	for _, toDelete := range opts.Names {
		if toDelete == active.Name {
			return fmt.Errorf("profile %q is the active profile and may not be deleted. Use %s to change the active configuration",
				toDelete, cs.String(fmt.Sprintf("%s profile profiles activate", version.Name)).Bold())
		}
		if _, ok := existing[toDelete]; !ok {
			return fmt.Errorf("profile %q does not exist", toDelete)
		}
	}

	if opts.IO.CanPrompt() {
		fmt.Fprintln(opts.IO.Err(), "The following profiles will be deleted:")
		for _, toDelete := range opts.Names {
			fmt.Fprintf(opts.IO.Err(), "  - %s\n", toDelete)
		}

		fmt.Fprintln(opts.IO.Err())
		ok, err := opts.IO.PromptConfirm("Do you want to continue")
		if err != nil {
			return err
		}

		if !ok {
			return nil
		}
	}

	if opts.DryRun {
		for _, toDelete := range opts.Names {
			fmt.Fprintf(opts.IO.Err(), "%s would delete profile %q\n", cs.DryRunLabel(), toDelete)
		}
		return nil
	}

	for _, toDelete := range opts.Names {
		if err := opts.Profiles.DeleteProfile(toDelete); err != nil {
			return fmt.Errorf("failed to delete profile %q: %w", toDelete, err)
		}

		fmt.Fprintf(opts.IO.Err(), "%s Profile %q deleted.\n", cs.SuccessIcon(), toDelete)
	}

	return nil
}
