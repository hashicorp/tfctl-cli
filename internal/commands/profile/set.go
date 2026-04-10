// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mitchellh/mapstructure"

	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
)

func NewCmdSet(ctx *cmd.Context) *cmd.Command {
	opts := &SetOpts{
		Ctx:     ctx.ShutdownCtx,
		IO:      ctx.IO,
		Profile: ctx.Profile,
	}

	cmd := &cmd.Command{
		Name:      "set",
		ShortHelp: "Set a tfcloud CLI Property.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "tfcloud profile set" }} command sets the specified property in your
		active profile. A property governs the behavior of a specific aspect of the tfcloud CLI.
		This could be setting the hostname and organization to target, or configuring the default
		level of logging across commands.

		To view all currently set properties, run {{ template "mdCodeOrBold" "tfcloud profile display" }}
		or run {{ template "mdCodeOrBold" "tfcloud profile get" }} to get the value of an individual property.

		To unset properties, use {{ template "mdCodeOrBold" "tfcloud profile unset" }}.

		tfcloud CLI comes with a default profile but supports multiple. To create multiple
		configurations, use {{ template "mdCodeOrBold" "tfcloud profile profiles create" }},
		and {{ template "mdCodeOrBold" "tfcloud profile profiles activate" }} to switch between them.
		`),
		Args: cmd.PositionalArguments{
			Autocomplete: opts.Profile,
			Args: []cmd.PositionalArgument{
				{
					Name: "PROPERTY",
					Documentation: heredoc.New(ctx.IO).Must(`
					Property to be set, such as
					{{ template "mdCodeOrBold" "organization" }} and
					{{ template "mdCodeOrBold" "hostname" }}.

					Consult the Available Properties section below for a comprehensive list of properties.
					`),
				},
				{
					Name:          "VALUE",
					Documentation: "Value to be set.",
				},
			},
		},
		AdditionalDocs: []cmd.DocSection{
			availablePropertiesDoc(ctx.IO),
		},
		NoAuthRequired: true,
		RunF: func(c *cmd.Command, args []string) error {
			opts.Property = args[0]
			opts.Value = args[1]
			return setRun(opts)
		},
	}

	return cmd
}

type SetOpts struct {
	Ctx     context.Context
	IO      iostreams.IOStreams
	Profile *profile.Profile

	// Arguments
	Property string
	Value    string
}

func setRun(opts *SetOpts) error {
	// Validate we are not changing the name
	if opts.Property == "name" {
		return fmt.Errorf("to update a profile name use %s",
			opts.IO.ColorScheme().String("tfcloud profile profiles rename").Bold())
	}

	// Validate we are setting a valid property
	if err := IsValidProperty(opts.Property); err != nil {
		return err
	}

	p := opts.Profile
	d, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput:     true,
		ErrorUnused:          true,
		Result:               p,
		TagName:              "hcl",
		IgnoreUntaggedFields: true,
	})
	if err != nil {
		return err
	}

	// Build the input
	input := map[string]any{}
	cur := input
	parts := strings.Split(opts.Property, "/")
	for i, p := range parts {
		if p == "" {
			return fmt.Errorf("property name following a \"/\" is required; empty property name is not allowed")
		}

		if i == len(parts)-1 {
			cur[p] = opts.Value
			continue
		}

		newLevel := map[string]any{}
		cur[p] = newLevel
		cur = newLevel
	}

	if err := d.Decode(input); err != nil {
		return convertDecodeError(err)
	}

	if err := p.Validate(); err != nil {
		return fmt.Errorf("invalid profile: %w", err)
	}

	// Check to see if the property being set is valid
	write := true
	if opts.Property == "hostname" {
		write, err = opts.validateHostname()
	} else if opts.Property == "organization" {
		write, err = opts.validateOrg()
	}
	if err != nil {
		return err
	} else if !write {
		return nil
	}

	// Check if geography was changed and clear org/project if needed
	hostnameChanged := false
	if opts.Property == "hostname" {
		hostnameChanged = true
		// Clear organization and token to force re-initialization
		p.Organization = ""
		p.Token = ""
	}

	if err := p.Write(); err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.Err(), "%s Property %q updated\n",
		opts.IO.ColorScheme().SuccessIcon(), opts.Property)

	// Notify user about hostname changes
	if hostnameChanged {
		fmt.Fprintf(opts.IO.Err(), "\n%s Hostname changed to %q. Organization and token settings have been cleared.\n",
			opts.IO.ColorScheme().WarningLabel(), opts.Value)
		fmt.Fprintf(opts.IO.Err(), "Please run %s to reconfigure your organization and token for this hostname.\n\n",
			opts.IO.ColorScheme().String("tfcloud profile init").Bold())
	}

	return nil
}

func (o *SetOpts) validateHostname() (bool, error) {
	return true, nil
}

func (o *SetOpts) validateOrg() (bool, error) {
	return true, nil
}

// convertDecodeError converts the mapstructure decode error into a more
// contextual error.
func convertDecodeError(err error) error {
	mapErr := &mapstructure.Error{}
	if !errors.As(err, &mapErr) {
		return err
	}

	// We only expect a single error to ever occur
	if len(mapErr.Errors) > 1 {
		return err
	}

	// Parse an invalid key at the top-level
	errStr := mapErr.Errors[0]
	if strings.HasPrefix(errStr, "'' has invalid keys:") {
		parts := strings.Split(errStr, ": ")
		return fmt.Errorf("no top-level property with name %q", parts[1])
	}

	// Try to parse invalid keys within a component. This could occur if a user
	// runs "set core/bad-key value"
	var component, property string
	_, scanErr := fmt.Sscanf(strings.ReplaceAll(errStr, "'", ""), "%s has invalid keys: %s", &component, &property)
	if scanErr == nil {
		return fmt.Errorf("invalid property %q for component %q", property, component)
	}

	return errors.New(mapErr.Errors[0])
}
