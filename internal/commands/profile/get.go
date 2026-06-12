// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdGet returns the `profile get` command for getting a CLI configuration property.
func NewCmdGet(inv *cmd.Invocation) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "get",
		ShortHelp: fmt.Sprintf("Get a %s CLI configuration property.", version.Name),
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile get" }} command gets the specified property in your active profile.

		To view all currently set properties, run {{ template "mdCodeOrBold" "%s profile display" }}.
		`, version.Name, version.Name),
		Args: cmd.PositionalArguments{
			Autocomplete: inv.Profile,
			Args: []cmd.PositionalArgument{
				{
					Name: "PROPERTY",
					Documentation: heredoc.New(inv.IO).Must(`
					Property to be get, such as
					{{ template "mdCodeOrBold" "organization" }} and
					{{ template "mdCodeOrBold" "hostname" }}.

					Consult the Available Properties section below for a comprehensive list of properties.
					`),
				},
			},
		},
		AdditionalDocs: []cmd.DocSection{
			availablePropertiesDoc(inv.IO),
		},
		NoAuthRequired: true,
		RunF: func(_ *cmd.Command, args []string) error {
			opts := &GetOpts{
				IO:      inv.IO,
				Output:  inv.Output,
				Profile: inv.Profile,
			}

			opts.Property = args[0]

			return getRun(inv.ShutdownCtx, opts)
		},
	}

	return cmd
}

// GetOpts defines the options for the `profile get` command.
type GetOpts struct {
	IO      iostreams.IOStreams
	Profile *profile.Profile
	Output  *format.Outputter

	Property string
}

func getRun(_ context.Context, opts *GetOpts) error {
	if err := IsValidProperty(opts.Property); err != nil {
		return err
	}

	// Decode the existing profile into a map
	var data map[string]any
	dec, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
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
	var value any
	for i, p := range parts {
		// This is the final property
		if i == len(parts)-1 {
			if _, ok := level[p]; !ok {
				return fmt.Errorf("property %q is not set", opts.Property)
			}

			value = level[p]
			break
		}

		// Retrieve the component
		nested, ok := level[p]
		if !ok {
			return fmt.Errorf("property %q is not set", opts.Property)
		}

		// Check if the retrieved element is a nested object
		sub, ok := nested.(map[string]any)
		if !ok {
			return fmt.Errorf("property %q is not set", opts.Property)
		}

		level = sub
	}

	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return fmt.Errorf("property %q is not set", opts.Property)
		}
	}

	value = reflect.Indirect(v).Interface()

	if opts.Output.GetFormat().IsJSONOrAgent() {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to JSON encode property value: %w", err)
		}

		fmt.Fprintln(opts.IO.Out(), string(data))
		return nil
	}

	fmt.Fprintf(opts.IO.Out(), "%v\n", value)
	return nil
}
