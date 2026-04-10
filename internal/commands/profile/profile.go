// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package profile implements the `tfcloud profile` command group for managing tfcloud CLI profiles
package profile

import (
	"fmt"
	"strings"

	"github.com/muesli/reflow/indent"
	"golang.org/x/exp/maps"

	"github.com/hashicorp/tfcloud/internal/commands/profile/profiles"
	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/ld"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
)

// NewCmdProfile returns the `tfcloud profile` command for managing tfcloud CLI profiles.
func NewCmdProfile(ctx *cmd.Context) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "profile",
		ShortHelp: "View and edit tfcloud CLI properties.",
		LongHelp: heredoc.New(ctx.IO).Must(`
		The {{ template "mdCodeOrBold" "tfcloud profile" }} command group lets you initialize,
		set, view and unset properties used by the tfcloud CLI.

		A profile is a collection of properties/configuration values that inform the behavior
		of {{ template "mdCodeOrBold" "tfcloud" }} CLI. You can create additional profiles
		using {{ template "mdCodeOrBold" "tfcloud profile profiles create" }}.

		To switch between profiles, use {{ template "mdCodeOrBold" "tfcloud profile profiles activate" }}.

		{{ template "mdCodeOrBold" "tfcloud" }} has several global flags that have matching profile properties.
		Examples are the {{ template "mdCodeOrBold" "verbosity" }} and
		{{ template "mdCodeOrBold" "organization" }} properties and their respective flags
		{{ template "mdCodeOrBold" "--verbose" }} and {{ template "mdCodeOrBold" "--organization" }}.
		The difference between properties and flags is that flags apply only on the invoked command,
		while properties are persistent across all invocations. Thus profiles allow you to conviently
		maintain the same settings across command executions and multiple profiles allow you to easily
		switch between different projects and settings.

		To run a command using a profile other than the active profile, pass the
		{{ template "mdCodeOrBold" "--profile" }} flag to the command.
		`),
	}

	cmd.AddChild(NewCmdDisplay(ctx))
	cmd.AddChild(NewCmdSet(ctx))
	cmd.AddChild(NewCmdUnset(ctx))
	cmd.AddChild(NewCmdGet(ctx))
	cmd.AddChild(profiles.NewCmdProfiles(ctx))
	return cmd
}

// IsValidProperty returns an error if the given property is invalid.
func IsValidProperty(property string) error {
	valid := profile.PropertyNames()
	if _, ok := valid[property]; ok {
		return nil
	}

	if suggestions := ld.Suggestions(property, maps.Keys(valid), 3, true); len(suggestions) != 0 {
		return fmt.Errorf("property with name %q does not exist; did you mean to type one of the following properties: \n\n%s",
			property, indent.String(strings.Join(suggestions, "\n"), 2))
	}

	return fmt.Errorf("property with name %q does not exist", property)
}
