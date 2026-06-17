// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profiles

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdRename returns the `profile profiles rename` command for renaming a configuration profile.
func NewCmdRename(inv *cmd.Invocation) *cmd.Command {
	opts := &RenameOpts{
		IO: inv.IO,
	}
	renameCmd := &cmd.Command{
		Name:      "rename",
		ShortHelp: "Rename an existing profile.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile profiles rename" }} command renames an existing profile.
		`, version.Name),
		Examples: []cmd.Example{
			{
				Preamble: heredoc.New(inv.IO).Must(`
				To rename profile {{ template "mdCodeOrBold" "my-profile" }} to
				{{ template "mdCodeOrBold" "new-profile" }}, run:
				`),
				Command: fmt.Sprintf("$ %s profile profiles rename my-profile --new-name=new_profile", version.Name),
			},
		},
		Args: cmd.PositionalArguments{
			Autocomplete: PredictProfiles(false, true),
			Args: []cmd.PositionalArgument{
				{
					Name:          "NAME",
					Documentation: "The name of the profile to rename.",
				},
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:         "new_name",
					DisplayValue: "NEW_NAME",
					Description:  "Specifies the new name of the profile.",
					Value:        flagvalue.Simple("", &opts.NewName),
					Required:     true,
				},
			},
		},
		NoAuthRequired: true,
		RunF: func(_ *cmd.Command, args []string) error {
			opts.ExistingName = args[0]
			l, err := profile.NewLoader()
			if err != nil {
				return err
			}
			opts.Profiles = l
			opts.DryRun = inv.IsDryRun()
			return renameRun(inv.ShutdownCtx, opts)
		},
	}

	return renameCmd
}

// RenameOpts defines the options for the `profile profiles rename` command.
type RenameOpts struct {
	IO           iostreams.IOStreams
	Profiles     *profile.Loader
	ExistingName string
	NewName      string
	DryRun       bool
}

func renameRun(ctx context.Context, opts *RenameOpts) error {
	logger := logging.FromContext(ctx)
	logger.Debug("renaming profile", "from", opts.ExistingName, "to", opts.NewName)

	if opts.ExistingName == opts.NewName {
		return fmt.Errorf("new name must be different from the existing name")
	}

	// Validate new name is a valid name.
	if _, err := opts.Profiles.NewProfile(opts.NewName); err != nil {
		return fmt.Errorf("invalid new name %q: %w", opts.NewName, err)
	}

	// Load the existing profile
	existing, err := opts.Profiles.LoadProfile(ctx, opts.ExistingName)
	if err != nil {
		if errors.Is(err, profile.ErrNoProfileFilePresent) {
			return fmt.Errorf("profile %q does not exist", opts.ExistingName)
		}

		return fmt.Errorf("failed to load profile %q: %w", opts.ExistingName, err)
	}

	// Ensure we don't clash with an existing profile name.
	profileNames, err := opts.Profiles.ListProfiles()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	if slices.Contains(profileNames, opts.NewName) {
		return fmt.Errorf("a profile with name %q already exists", opts.NewName)
	}

	active, err := opts.Profiles.GetActiveProfile()
	if err != nil {
		return fmt.Errorf("failed to get active profile: %w", err)
	}

	// Update the name and save.
	existing.Name = opts.NewName
	if opts.DryRun {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Err(), "%s would rename profile %q to %q\n", cs.DryRunLabel(), opts.ExistingName, opts.NewName)
		if active.Name == opts.ExistingName {
			fmt.Fprintf(opts.IO.Err(), "%s would activate profile %q\n", cs.DryRunLabel(), opts.NewName)
		}
		return nil
	}
	if err := existing.Write(); err != nil {
		return fmt.Errorf("error saving renamed profile: %w", err)
	}

	fmt.Fprintf(opts.IO.Err(), "%s Profile %q renamed to %q.\n",
		opts.IO.ColorScheme().SuccessIcon(), opts.ExistingName, opts.NewName)

	// Delete the old profile
	if err := opts.Profiles.DeleteProfile(opts.ExistingName); err != nil {
		return fmt.Errorf("failed to delete old profile: %w", err)
	}

	// If the active profile was the profile that we just renamed, update to the
	// new name.
	if active.Name == opts.ExistingName {
		active.Name = opts.NewName
		if err := active.Write(); err != nil {
			return fmt.Errorf("failed to save active profile: %w", err)
		}

		fmt.Fprintf(opts.IO.Err(), "%s Profile %q activated.\n",
			opts.IO.ColorScheme().SuccessIcon(), opts.NewName)
	}

	return nil
}
