// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"fmt"

	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/tfctl-cli/internal/config"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/format"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/profile"
)

// NewCmdDisplay returns the `profile display` command for displaying the active profile.
func NewCmdDisplay(ctx *cmd.Context) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "display",
		ShortHelp: "Display the active profile.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "%s profile display" }} command displays the active profile.
		`, config.Name),
		RunF: func(c *cmd.Command, _ []string) error {
			profileNoToken := ctx.Profile
			profileNoToken.Token = ""

			return displayRun(&DisplayOpts{
				IO:      ctx.IO,
				Output:  ctx.Output,
				Profile: profileNoToken,
				Logger:  c.Logger(ctx),
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
	Logger  hclog.Logger
}

func displayRun(opts *DisplayOpts) error {
	d := &displayDisplayer{
		profile: opts.Profile,
	}

	if opts.Output.GetFormat() != format.Unset {
		return opts.Output.Display(d)
	}

	fmt.Fprint(opts.IO.Out(), opts.Profile.String())
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
