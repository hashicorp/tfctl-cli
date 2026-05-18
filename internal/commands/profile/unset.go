// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/mitchellh/mapstructure"

	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

// NewCmdUnset returns the `profile unset` command for unsetting a profile configuration property.
func NewCmdUnset(ctx *cmd.Context) *cmd.Command {
	opts := &UnsetOpts{
		Ctx:     ctx.ShutdownCtx,
		IO:      ctx.IO,
		Profile: ctx.Profile,
	}

	cmd := &cmd.Command{
		Name:      "unset",
		ShortHelp: "Unset a profile configuration property.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile unset" }} command unsets the specified property in your active profile.

		To view all currently set properties, run {{ template "mdCodeOrBold" "%s profile display" }}.
		`, config.Name, config.Name),
		Args: cmd.PositionalArguments{
			Autocomplete: opts.Profile,
			Args: []cmd.PositionalArgument{
				{
					Name: "PROPERTY",
					Documentation: heredoc.New(ctx.IO).Must(`
					Property to be unset, such as
					{{ template "mdCodeOrBold" "organization" }} and
					{{ template "mdCodeOrBold" "hostname" }}.

					Consult the Available Properties section below for a comprehensive list of properties.
					`),
				},
			},
		},
		AdditionalDocs: []cmd.DocSection{
			availablePropertiesDoc(ctx.IO),
		},
		NoAuthRequired: true,
		RunF: func(c *cmd.Command, args []string) error {
			opts.Property = args[0]
			opts.Logger = c.Logger(ctx)
			l, err := profile.NewLoader()
			if err != nil {
				return err
			}
			opts.Profiles = l
			opts.DryRun = ctx.IsDryRun()

			return unsetRun(opts)
		},
	}

	return cmd
}

// UnsetOpts defines the options for the `profile unset` command.
type UnsetOpts struct {
	Ctx     context.Context
	IO      iostreams.IOStreams
	Profile *profile.Profile
	Logger  hclog.Logger

	Property string
	Profiles *profile.Loader
	DryRun   bool
}

func unsetRun(opts *UnsetOpts) error {
	// Validate we are not changing the name
	if opts.Property == "name" {
		return fmt.Errorf("to update a profile name use %s",
			opts.IO.ColorScheme().String(fmt.Sprintf("%s profile profiles rename", config.Name)).Bold())
	}

	if err := IsValidProperty(opts.Property); err != nil {
		return err
	}

	opts.Logger.Debug("unsetting property", "property", opts.Property, "profile", opts.Profile.Name)

	// Decode the existing profile into a map
	var data map[string]any
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput:     true,
		ErrorUnused:          true,
		Result:               &data,
		TagName:              "hcl",
		IgnoreUntaggedFields: true,
	})
	if err != nil {
		return err
	}

	if err := dec.Decode(opts.Profile); err != nil {
		return err
	}

	// Delete the key from the map
	parts := strings.Split(opts.Property, "/")
	level := data
	didDelete := false
	for i, p := range parts {
		// This is the final property
		if i == len(parts)-1 {
			if _, ok := level[p]; !ok {
				break
			}

			delete(level, p)
			didDelete = true
			break
		}

		// Retrieve the component
		nested, ok := level[p]
		if !ok {
			break
		}

		// Check if the retrieved element is a nested object
		sub, ok := nested.(map[string]any)
		if !ok {
			break
		}

		level = sub
	}

	if didDelete {
		p, err := opts.Profiles.NewProfile(opts.Profile.Name)
		if err != nil {
			return err
		}

		dec2, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			WeaklyTypedInput:     true,
			ErrorUnused:          true,
			Result:               p,
			TagName:              "hcl",
			IgnoreUntaggedFields: true,
		})
		if err != nil {
			return err
		}

		if err := dec2.Decode(data); err != nil {
			return convertDecodeError(err)
		}

		if err := p.Validate(); err != nil {
			return fmt.Errorf("invalid profile: %w", err)
		}

		if opts.DryRun {
			fmt.Fprintf(opts.IO.Err(), "%s would unset profile property %q\n", opts.IO.ColorScheme().DryRunLabel(), opts.Property)
			return nil
		}

		if err := p.Write(); err != nil {
			return err
		}
	}

	cs := opts.IO.ColorScheme()
	fmt.Fprintf(opts.IO.Err(), "%s Property %q unset\n", cs.SuccessIcon(), opts.Property)
	return nil
}
