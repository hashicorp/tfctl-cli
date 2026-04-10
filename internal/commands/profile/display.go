// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package profile

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/tfcloud/internal/pkg/cmd"
	"github.com/hashicorp/tfcloud/internal/pkg/format"
	"github.com/hashicorp/tfcloud/internal/pkg/heredoc"
	"github.com/hashicorp/tfcloud/internal/pkg/iostreams"
	"github.com/hashicorp/tfcloud/internal/pkg/profile"
)

func NewCmdDisplay(ctx *cmd.Context) *cmd.Command {
	cmd := &cmd.Command{
		Name:      "display",
		ShortHelp: "Display the active profile.",
		LongHelp: heredoc.New(ctx.IO).Mustf(`
		The {{ template "mdCodeOrBold" "tfcloud profile display" }} command displays the active profile.
		`),
		RunF: func(c *cmd.Command, args []string) error {
			return displayRun(&DisplayOpts{
				IO:      ctx.IO,
				Profile: ctx.Profile,
				Format:  ctx.Output.GetFormat(),
			})
		},
		NoAuthRequired: true,
	}

	return cmd
}

type DisplayOpts struct {
	IO      iostreams.IOStreams
	Profile *profile.Profile
	Format  format.Format
}

func displayRun(opts *DisplayOpts) error {
	if opts.Format == format.JSON {
		data, err := json.MarshalIndent(opts.Profile, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to JSON encode profile: %w", err)
		}

		fmt.Fprintln(opts.IO.Out(), string(data))
	} else {
		fmt.Fprintln(opts.IO.Out(), opts.Profile.String())
	}

	return nil
}
