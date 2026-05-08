// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"errors"
	"fmt"

	"github.com/MakeNowJust/heredoc/v2"

	"github.com/hashicorp/tfctl-cli/internal/config"
)

// RequireOrganization requires that the profile has a set organization.
func RequireOrganization(ctx *Context) error {
	if ctx.Profile.Organization != "" {
		return nil
	}

	cs := ctx.IO.ColorScheme()
	help := heredoc.Docf(`%v

	Please run %v to interactively set the Organization, or run:

	%v`,
		cs.String("Organization must be configured before running the command.").Color(cs.Orange()),
		cs.String(fmt.Sprintf("%s config init", config.Name)).Bold(),
		cs.String(fmt.Sprintf("$ %s config set organization <organization>", config.Name)).Bold(),
	)

	return errors.New(help)
}

// RequireOrg requires that the profile has a set organization.
func RequireOrg(ctx *Context) error {
	if ctx.Profile.Organization != "" {
		return nil
	}

	cs := ctx.IO.ColorScheme()
	help := heredoc.Docf(`%v

	Please run %s to interactively set the Organization, or run:

	%v`,
		cs.String("Organization must be configured before running the command.").Color(cs.Orange()),
		cs.String(fmt.Sprintf("%s config init", config.Name)).Bold(),
		cs.String(fmt.Sprintf("$ %s config set organization <organization>", config.Name)).Bold(),
	)

	return errors.New(help)
}
