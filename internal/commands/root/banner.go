// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package root

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/skills"
	"github.com/hashicorp/tfctl-cli/version"
)

//go:embed logo.txt
var Logo string

type BannerOpts struct {
	IO              iostreams.IOStreams
	TokenConfigured bool
}

// NewCmdBanner creates the hidden banner command.
func NewCmdBanner(inv *cmd.Invocation) *cmd.Command {
	c := &cmd.Command{
		Hidden:         true,
		Name:           "banner",
		ShortHelp:      "",
		LongHelp:       "A hidden command for displaying the banner.",
		NoAuthRequired: true,
		RunF: func(c *cmd.Command, args []string) error {
			RunBanner(inv.ShutdownCtx, &BannerOpts{
				IO:              inv.IO,
				TokenConfigured: inv.Profile != nil && inv.Profile.GetToken() != "",
			})
			return nil
		},
	}
	return c
}

// RunBanner displays the banner with the logo and version information.
func RunBanner(ctx context.Context, opts *BannerOpts) {
	// Implementation for displaying the banner goes here.

	io := opts.IO

	if io.ColorEnabled() && io.IsOutputTTY() {
		cs := io.ColorScheme()
		// Prepends two spaces before every line of the logo and after the final line
		fmt.Fprintf(io.ErrUnessential(), "  %s", strings.Join(strings.Split(Logo, "\n"), "\n  "))
		fmt.Fprintf(io.ErrUnessential(), "%s\n", cs.String(version.Version).Color(cs.Purple()).Bold())
		fmt.Fprintln(io.ErrUnessential(), "")
	} else {
		fmt.Fprintln(io.ErrUnessential(), version.Version)
	}

	if !opts.TokenConfigured {
		fmt.Fprintln(io.Err(), heredoc.New(io).Mustf(`Get started by running {{ template "mdCodeOrBold" "%s auth login" }}
to authenticate with your user account or run {{ template "mdCodeOrBold" "%s --help" }} for usage
information.
`, version.Name, version.Name))
	}

	fmt.Fprintln(io.Err(), heredoc.New(io).Mustf(`Release notes for this version are available at
{{ template "mdCodeOrBold" "https://github.com/hashicorp/tfctl-cli/blob/%s/CHANGELOG.md" }}`, version.Version))

	fmt.Fprintln(io.Err(), "")
}
