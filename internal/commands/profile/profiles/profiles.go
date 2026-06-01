// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package profiles implements the `profile profiles` command group for managing configuration profiles.
package profiles

import (
	"github.com/posener/complete"
	"golang.org/x/exp/maps"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdProfiles returns the `profile profiles` command for managing configuration profiles.
func NewCmdProfiles(ctx *cmd.Context) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "profiles",
		ShortHelp: "Manage configuration profiles.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile profiles" }} command group manages 
		the set of named %s profiles. You can create new profiles using
		{{ template "mdCodeOrBold" "%s profile profiles create" }} and activate existing
		profiles using {{ template "mdCodeOrBold" "%s profile profiles activate" }}.
		To run a single command against a profile other than the active profile,
		run the command with the flag {{ template "mdCodeOrBold" "--profile" }}.
		`, version.Name, version.Name, version.Name, version.Name),
	}

	cmd.AddChild(NewCmdCreate(ctx))
	cmd.AddChild(NewCmdDelete(ctx))
	cmd.AddChild(NewCmdList(ctx))
	cmd.AddChild(NewCmdActivate(ctx))
	cmd.AddChild(NewCmdRename(ctx))

	return cmd
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
