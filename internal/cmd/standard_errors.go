// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package cmd

import (
	"errors"

	"github.com/MakeNowJust/heredoc/v2"
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
		cs.String("tfcloud config init").Bold(),
		cs.String("$ tfcloud config set organization <organization>").Bold(),
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
		cs.String("tfcloud config init").Bold(),
		cs.String("$ tfcloud config set organization <organization>").Bold(),
	)

	return errors.New(help)
}
