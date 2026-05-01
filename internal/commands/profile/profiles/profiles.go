// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package profiles implements the `tfcloud profile profiles` command group for managing tfcloud CLI profiles.
package profiles

import (
	"fmt"
	"strings"

	"github.com/muesli/reflow/indent"
	"github.com/posener/complete"
	"golang.org/x/exp/maps"

	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/ld"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
)

// NewCmdProfiles returns the `tfcloud profile profiles` command for managing tfcloud CLI profiles.
func NewCmdProfiles(ctx *cmd.Context) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "profiles",
		ShortHelp: "Manage tfcloud profiles.",
		LongHelp: heredoc.New(ctx.IO).Must(`
		The {{ template "mdCodeOrBold" "tfcloud profile profiles" }} command group manages 
		the set of named tfcloud profiles. You can create new profiles using
		{{ template "mdCodeOrBold" "tfcloud profile profiles create" }} and activate existing
		profiles using {{ template "mdCodeOrBold" "tfcloud profile profiles activate" }}.
		To run a single command against a profile other than the active profile,
		run the command with the flag {{ template "mdCodeOrBold" "--profile" }}.
		`),
	}

	cmd.AddChild(NewCmdCreate(ctx))
	cmd.AddChild(NewCmdDelete(ctx))
	cmd.AddChild(NewCmdList(ctx))
	cmd.AddChild(NewCmdActivate(ctx))
	cmd.AddChild(NewCmdRename(ctx))

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

// PredictProfiles is an argument prediction function that predicts a
// profile name. If repeated is true, multiple profiles will be predicted. This
// is useful for commands that accept lists of profiles. If predictActive is set
// to true, the active profile will be included in the prediction set.
func PredictProfiles(repeated, predictActive bool) complete.PredictFunc {
	return func(args complete.Args) []string {
		if len(args.Completed) >= 1 && !repeated {
			return nil
		}

		// Get the profile loader
		l, err := profile.NewLoader()
		if err != nil {
			return nil
		}

		// Get all the profiles that exist
		profiles, err := l.ListProfiles()
		if err != nil {
			return nil
		}

		allProfiles := make(map[string]struct{}, len(profiles))
		for _, p := range profiles {
			allProfiles[p] = struct{}{}
		}

		// Go through any previously predicted profiles and remove them
		for _, p := range args.Completed {
			delete(allProfiles, p)
		}

		// Get the active profile and delete it
		if !predictActive {
			if active, err := l.GetActiveProfile(); err == nil {
				delete(allProfiles, active.Name)
			}
		}

		return maps.Keys(allProfiles)
	}
}
