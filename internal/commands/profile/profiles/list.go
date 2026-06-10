// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profiles

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdList returns the `profile profiles list` command for listing configuration profiles.
func NewCmdList(inv *cmd.Invocation) *cmd.Command {
	opts := &ListOpts{
		IO:     inv.IO,
		Output: inv.Output,
	}
	cmd := &cmd.Command{
		Name:      "list",
		ShortHelp: "List existing configuration profiles.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile profiles list" }} command lists existing configuration profiles.
		`, version.Name),
		Examples: []cmd.Example{
			{
				Preamble: "To list existing profiles, run:",
				Command:  fmt.Sprintf("$ %s profile profiles list", version.Name),
			},
		},
		NoAuthRequired: true,
		RunF: func(_ *cmd.Command, _ []string) error {
			l, err := profile.NewLoader()
			if err != nil {
				return err
			}
			opts.Profiles = l
			return listRun(inv.ShutdownCtx, opts)
		},
	}

	return cmd
}

// ListOpts defines the options for the `profile profiles list` command.
type ListOpts struct {
	IO       iostreams.IOStreams
	Output   *format.Outputter
	Profiles *profile.Loader
}

func listRun(_ context.Context, opts *ListOpts) error {
	profileNames, err := opts.Profiles.ListProfiles()
	if err != nil {
		return fmt.Errorf("failed to list profiles: %w", err)
	}

	profiles := make([]*profile.Profile, len(profileNames))
	for i, n := range profileNames {
		p, err := opts.Profiles.LoadProfile(n)
		if err != nil {
			return fmt.Errorf("failed to load profile %q: %w", n, err)
		}

		profiles[i] = p
	}

	// Sort the profiles based on name
	slices.SortFunc(profiles, func(p1, p2 *profile.Profile) int {
		return strings.Compare(p1.Name, p2.Name)
	})

	// Get the active profile
	active, err := opts.Profiles.GetActiveProfile()
	if err != nil {
		return fmt.Errorf("failed to get active profile: %w", err)
	}

	d := &profileDisplayer{
		profiles:      profiles,
		activeProfile: active.Name,
	}

	return opts.Output.Display(d)
}

type profileDisplayer struct {
	profiles      []*profile.Profile
	activeProfile string
}

func (p *profileDisplayer) DefaultFormat() format.Format { return format.Table }
func (p *profileDisplayer) Payload() any                 { return p.profiles }

func (p *profileDisplayer) FieldTemplates() []format.Field {
	return []format.Field{
		{
			Name:        "Name",
			ValueFormat: "{{ .Name }}",
		},
		{
			Name:        "Hostname",
			ValueFormat: "{{ .Hostname }}",
		},
		{
			Name:        "Active",
			ValueFormat: fmt.Sprintf("{{ eq ( .Name ) %q }}", p.activeProfile),
		},
		{
			Name:        "Organization",
			ValueFormat: "{{ .Organization }}",
		},
	}
}
