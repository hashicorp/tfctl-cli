// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profiles

import (
	"fmt"

	"github.com/posener/complete"

	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/flagvalue"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

// NewCmdCreate returns the `profile profiles create` command for creating a new configuration profile.
func NewCmdCreate(ctx *cmd.Context) *cmd.Command {
	opts := &CreateOpts{
		IO: ctx.IO,
	}
	cmd := &cmd.Command{
		Name:      "create",
		ShortHelp: "Create a new configuration profile.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile profiles create" }} command creates a new named profile.

		Profile names start with a letter and may contain lower case letters a-z,
		upper case letters A-Z, digits 0-9, and underscores '_'. The maximum length for
		a profile name is 64 characters.
		`, config.Name),
		Examples: []cmd.Example{
			{
				Preamble: "To create a new profile, run:",
				Command:  fmt.Sprintf("$ %s profile profiles create my_profile", config.Name),
			},
		},
		Args: cmd.PositionalArguments{
			Args: []cmd.PositionalArgument{
				{
					Name:          "NAME",
					Documentation: "The name of the profile to create.",
				},
			},
		},
		Flags: cmd.Flags{
			Local: []*cmd.Flag{
				{
					Name:          "no-activate",
					Description:   "Disables automatic activation of the newly created profile.",
					Value:         flagvalue.Simple(false, &opts.NoActivate),
					IsBooleanFlag: true,
				},
				{
					Name:         "hostname",
					DisplayValue: "HOSTNAME",
					Description:  "HCP Terraform / Terraform Enterprise hostname.",
					Value:        flagvalue.Simple("", &opts.Hostname),
					Autocomplete: complete.PredictSet("app.eu.terraform.io"),
				},
			},
		},
		NoAuthRequired: true,
		RunF: func(_ *cmd.Command, args []string) error {
			opts.Name = args[0]
			opts.DryRun = ctx.IsDryRun()
			l, err := profile.NewLoader()
			if err != nil {
				return err
			}
			opts.Profiles = l
			return createRun(opts)
		},
	}

	return cmd
}

// CreateOpts defines the options for the `profile profiles create` command.
type CreateOpts struct {
	IO iostreams.IOStreams

	Profiles   *profile.Loader
	Name       string
	NoActivate bool
	Hostname   string
	DryRun     bool
}

func createRun(opts *CreateOpts) error {
	// Get the existing profiles
	profiles, err := opts.Profiles.ListProfiles()
	if err != nil {
		return fmt.Errorf("failed to list existing profiles: %w", err)
	}

	// Validate a profile with the given name doesn't already exist.
	for _, p := range profiles {
		if p == opts.Name {
			return fmt.Errorf("profile with name %q already exists", opts.Name)
		}
	}

	// Create the new profile
	p, err := opts.Profiles.NewProfile(opts.Name)
	if err != nil {
		return err
	}

	// Set the hostname if provided
	if opts.Hostname != "" {
		p.Hostname = opts.Hostname
	}

	if opts.DryRun {
		cs := opts.IO.ColorScheme()
		fmt.Fprintf(opts.IO.Err(), "%s would create profile %q\n", cs.DryRunLabel(), opts.Name)
		if !opts.NoActivate {
			fmt.Fprintf(opts.IO.Err(), "%s would activate profile %q\n", cs.DryRunLabel(), opts.Name)
		}
		return nil
	}

	// Save the profile
	if err := p.Write(); err != nil {
		return fmt.Errorf("failed to save new profile: %w", err)
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.Err(), "%s Profile %q created.\n", cs.SuccessIcon(), p.Name)

	if !opts.NoActivate {
		// Update the active profile.
		active, err := opts.Profiles.GetActiveProfile()
		if err != nil {
			return fmt.Errorf("failed to retrieve active profile: %w", err)
		}

		active.Name = p.Name
		if err := active.Write(); err != nil {
			return fmt.Errorf("failed to update active profile: %w", err)
		}

		fmt.Fprintf(opts.IO.Err(), "%s Profile %q activated.\n", cs.SuccessIcon(), p.Name)
	}

	fmt.Fprintln(opts.IO.Err())
	fmt.Fprintln(opts.IO.Err(), heredoc.New(opts.IO).Mustf(`
		To initialize the newly created profile, run:

		  {{ Bold "$ %s profile init" }}
		`, config.Name))
	fmt.Fprintln(opts.IO.Err())

	return nil
}
