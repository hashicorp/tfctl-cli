// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"fmt"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
	"github.com/hashicorp/tfctl-cli/version"
)

// NewCmdDisplay returns the `profile display` command for displaying the active profile.
func NewCmdDisplay(inv *cmd.Invocation) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "display",
		ShortHelp: "Display the active profile.",
		LongHelp: heredoc.New(inv.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile display" }} command displays the active profile.
		`, version.Name),
		RunF: func(_ *cmd.Command, _ []string) error {
			profileNoToken := inv.Profile
			profileNoToken.Token = ""

			return displayRun(&DisplayOpts{
				IO:      inv.IO,
				Output:  inv.Output,
				Profile: profileNoToken,
			})
		},
		NoAuthRequired: true,
	}

	return cmd
}

// DisplayOpts defines the options for the `profile display` command.
type DisplayOpts struct {
	IO      iostreams.IOStreams
	Profile *profile.Profile
	Output  *format.Outputter
}

func displayRun(opts *DisplayOpts) error {
	d := &displayDisplayer{
		profile: opts.Profile,
	}

	if opts.Output.GetFormat() != format.Unset {
		return opts.Output.Display(d)
	}

	fmt.Fprint(opts.IO.Out(), opts.Profile.String())
	fmt.Fprintln(opts.IO.Out())
	return nil
}

type displayDisplayer struct {
	profile *profile.Profile
}

func (p *displayDisplayer) DefaultFormat() format.Format { return format.Pretty }
func (p *displayDisplayer) Payload() any                 { return p.profile }

func (p *displayDisplayer) FieldTemplates() []format.Field {
	return []format.Field{
		{
			Name:        "Name",
			ValueFormat: "{{ .Name }}",
		},
		{
			Name:        "Organization",
			ValueFormat: "{{ .Organization }}",
		},
		{
			Name:        "Hostname",
			ValueFormat: "{{ .Hostname }}",
		},
	}
}
