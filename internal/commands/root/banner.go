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
var logo string

// BannerOpts contains the options for running the banner command.
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
		RunF: func(_ *cmd.Command, _ []string) error {
			RunBanner(inv.ShutdownCtx, &BannerOpts{
				IO:              inv.IO,
				TokenConfigured: inv.Profile != nil && inv.Profile.GetToken() != "",
			})
			return nil
		},
	}
	return c
}

// RunDetectOutdatedSkill checks for any existing skills that are outdated and prints a helpful message
// about reinstalling it.
func RunDetectOutdatedSkill(io iostreams.IOStreams) {
	if s := skills.DetectAnyExistingSkill(); s != nil {
		if skillVer, ok := s.KnownVersion(); ok && skillVer != version.Version {
			fmt.Fprintln(io.Err(), heredoc.New(io).Mustf(`Existing skill {{ template "mdCodeOrBold" "%s" }} was created by version %s, which differs from the current version %s. Consider running {{ template "mdCodeOrBold" "%s" }} to re-install the skill to match the current version.`, s.Path, skillVer, version.Version, s.ReinstallCommand()))
		}
	}
}

// RunBanner displays the banner with the logo and version information.
func RunBanner(_ context.Context, opts *BannerOpts) {
	// Implementation for displaying the banner goes here.

	io := opts.IO

	if io.ColorEnabled() && io.IsOutputTTY() {
		cs := io.ColorScheme()
		// Prepends two spaces before every line of the logo and after the final line
		fmt.Fprintf(io.ErrUnessential(), "  %s", strings.Join(strings.Split(logo, "\n"), "\n  "))
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
	} else {
		RunDetectOutdatedSkill(io)
	}

	fmt.Fprintln(io.Err())
	fmt.Fprintln(io.Err(), heredoc.New(io).Mustf(`Release notes for this version are available at
{{ template "mdCodeOrBold" "https://github.com/hashicorp/tfctl-cli/blob/%s/CHANGELOG.md" }}`, version.Version))

	fmt.Fprintln(io.Err(), "")
}
