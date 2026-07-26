// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

// Package versioncmd provides the hidden version command for the tfctl CLI,
// which is invoked using `--version`, `-v`, or `version`
package versioncmd

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/hashicorp/tfctl-cli/internal/pkg/checkpoint"
	"github.com/hashicorp/tfctl-cli/internal/pkg/cmd"
	"github.com/hashicorp/tfctl-cli/internal/pkg/heredoc"
	"github.com/hashicorp/tfctl-cli/internal/pkg/iostreams"
	"github.com/hashicorp/tfctl-cli/internal/pkg/logging"
	"github.com/hashicorp/tfctl-cli/skills"
	"github.com/hashicorp/tfctl-cli/version"
)

//go:embed logo.txt
var logo string

// VersionOpts contains the options for running the banner command.
type VersionOpts struct {
	IO              iostreams.IOStreams
	TokenConfigured bool
}

// NewCmdVersion creates the hidden version command.
func NewCmdVersion(inv *cmd.Invocation) *cmd.Command {
	c := &cmd.Command{
		Hidden:         true,
		Name:           "version",
		ShortHelp:      "Shows the current version.",
		LongHelp:       heredoc.New(inv.IO).Mustf(`Shows the current version, checks for newer CLI versions and outdated skill installations.`),
		NoAuthRequired: true,
		RunF: func(_ *cmd.Command, _ []string) error {
			runVersion(inv.ShutdownCtx, &VersionOpts{
				IO:              inv.IO,
				TokenConfigured: inv.Profile != nil && inv.Profile.GetToken() != "",
			})
			return nil
		},
	}
	return c
}

// runDetectOutdatedSkill checks for any existing skills that are outdated and prints a message
// suggesting reinstallation.
func runDetectOutdatedSkill(ctx context.Context, io iostreams.IOStreams) {
	logger := logging.FromContext(ctx)
	if s := skills.DetectAnyExistingSkill(); s != nil {
		if skillVer, ok := s.KnownVersion(); ok && skillVer.Version != version.Version {
			logger.Debug("Detected known skill", "version", skillVer.Version, "checksum", skillVer.Checksum)

			if skillVer.Checksum != skills.EmbeddedChecksum() {
				fmt.Fprintln(io.ErrUnessential(), heredoc.New(io).Mustf(`Existing skill {{ template "mdCodeOrBold" "%s" }} was created by version %s, which differs from the current version %s. Consider running {{ template "mdCodeOrBold" "%s" }} to re-install the skill to match the current version.`, s.Path, skillVer.Version, version.Version, s.ReinstallCommand()))
			}
		}
	}
}

// runDetectOutdatedVersion checks if the current CLI version is outdated and prints relevant messages.
func runDetectOutdatedVersion(_ context.Context, io iostreams.IOStreams) {
	cs := io.ColorScheme()
	versionInfo := checkpoint.WaitForVersionCheck()

	if versionInfo != nil {
		if versionInfo.Outdated {
			fmt.Fprintf(io.ErrUnessential(), "A new version of %s is available: %s\n", version.Name, cs.String(fmt.Sprintf("v%s", versionInfo.Latest)).Color(cs.Purple()).Bold())
		} else {
			fmt.Fprintln(io.ErrUnessential())
			fmt.Fprintln(io.ErrUnessential(), heredoc.New(io).Mustf(`Release notes for this version are available at
			{{ template "mdCodeOrBold" "https://github.com/hashicorp/tfctl-cli/blob/%s/CHANGELOG.md" }}`, version.Version))

			fmt.Fprintln(io.ErrUnessential())
		}

		if len(versionInfo.Alerts) > 0 {
			fmt.Fprintln(io.ErrUnessential(), "")
			fmt.Fprintf(io.ErrUnessential(), "%s: %s\n", cs.WarningLabel(), "There are alerts regarding your current version.")
			for _, alert := range versionInfo.Alerts {
				fmt.Fprintln(io.ErrUnessential(), heredoc.New(io, heredoc.WithNoWrap()).Mustf(" - %s", alert))
			}
		}
	}
}

// runVersion displays the banner with the logo and version information.
func runVersion(ctx context.Context, opts *VersionOpts) {
	// Implementation for displaying the version information/banner goes here.
	io := opts.IO
	cs := io.ColorScheme()

	if io.ColorEnabled() && io.IsOutputTTY() {
		// Prepends two spaces before every line of the logo and after the final line
		fmt.Fprintf(io.ErrUnessential(), "  %s", strings.Join(strings.Split(logo, "\n"), "\n  "))
		fmt.Fprintf(io.Err(), "%s\n", cs.String(version.Version).Color(cs.Purple()).Bold())
		fmt.Fprintln(io.ErrUnessential(), "")
	} else {
		fmt.Fprintln(io.Err(), version.Version)
	}

	if !opts.TokenConfigured {
		fmt.Fprintln(io.Err(), heredoc.New(io).Mustf(`Get started by running {{ template "mdCodeOrBold" "%s auth login" }}
to authenticate with your user account or run {{ template "mdCodeOrBold" "%s --help" }} for usage
information.
`, version.Name, version.Name))
	} else {
		runDetectOutdatedSkill(ctx, io)
	}

	runDetectOutdatedVersion(ctx, io)
}
